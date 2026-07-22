package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/scp-stk/hub/internal/model"
)

type CommitteeRepo struct {
	db *pgxpool.Pool
}

func NewCommitteeRepo(db *pgxpool.Pool) *CommitteeRepo {
	return &CommitteeRepo{db: db}
}

// List returns every committee with its members joined in (name looked up
// from auth.users for display, avoiding a separate round trip per member).
func (r *CommitteeRepo) List(ctx context.Context) ([]model.Committee, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, name, description, created_by, created_at
		 FROM committees ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list committees: %w", err)
	}
	defer rows.Close()

	var committees []model.Committee
	for rows.Next() {
		var c model.Committee
		var desc *string
		if err := rows.Scan(&c.ID, &c.Name, &desc, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, err
		}
		if desc != nil {
			c.Description = *desc
		}
		committees = append(committees, c)
	}

	for i := range committees {
		members, err := r.membersFor(ctx, committees[i].ID)
		if err != nil {
			return nil, err
		}
		committees[i].Members = members
	}
	return committees, nil
}

func (r *CommitteeRepo) membersFor(ctx context.Context, committeeID string) ([]model.CommitteeMember, error) {
	rows, err := r.db.Query(ctx,
		`SELECT cm.id, cm.committee_id, cm.user_id, cm.role, cm.added_at,
		        COALESCE(u.full_name, '') AS full_name
		 FROM committee_members cm
		 LEFT JOIN users u ON u.id = cm.user_id
		 WHERE cm.committee_id = $1
		 ORDER BY CASE cm.role WHEN 'lead' THEN 0 WHEN 'co-lead' THEN 1 ELSE 2 END, cm.added_at`,
		committeeID)
	if err != nil {
		return nil, fmt.Errorf("list committee members: %w", err)
	}
	defer rows.Close()

	var out []model.CommitteeMember
	for rows.Next() {
		var m model.CommitteeMember
		if err := rows.Scan(&m.ID, &m.CommitteeID, &m.UserID, &m.Role, &m.AddedAt, &m.FullName); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// Create makes a new committee and adds any initial members (as "member"
// role — promote to lead/co-lead afterward via UpdateMemberRole).
func (r *CommitteeRepo) Create(ctx context.Context, req model.CreateCommitteeRequest, createdBy string) (*model.Committee, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	id := uuid.New().String()
	var c model.Committee
	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO committees (id, name, description, created_by)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id, name, description, created_by, created_at`,
		id, req.Name, desc, createdBy).
		Scan(&c.ID, &c.Name, &desc, &c.CreatedBy, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert committee: %w", err)
	}
	if desc != nil {
		c.Description = *desc
	}

	for _, userID := range req.MemberIDs {
		mid := uuid.New().String()
		if _, err := tx.Exec(ctx,
			`INSERT INTO committee_members (id, committee_id, user_id, role)
			 VALUES ($1,$2,$3,'member')
			 ON CONFLICT (committee_id, user_id) DO NOTHING`,
			mid, id, userID); err != nil {
			return nil, fmt.Errorf("add initial member %q: %w", userID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	c.Members, _ = r.membersFor(ctx, id)
	return &c, nil
}

// AddMember adds a user to a committee with the given role (defaults to
// "member"). Idempotent-ish: if they're already a member, this errors with
// a constraint violation rather than silently updating their role — use
// UpdateMemberRole for role changes instead.
func (r *CommitteeRepo) AddMember(ctx context.Context, committeeID, userID, role string) (*model.CommitteeMember, error) {
	if role == "" {
		role = "member"
	}
	id := uuid.New().String()
	var m model.CommitteeMember
	err := r.db.QueryRow(ctx,
		`INSERT INTO committee_members (id, committee_id, user_id, role)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id, committee_id, user_id, role, added_at`,
		id, committeeID, userID, role).
		Scan(&m.ID, &m.CommitteeID, &m.UserID, &m.Role, &m.AddedAt)
	if err != nil {
		return nil, fmt.Errorf("add member: %w", err)
	}
	return &m, nil
}

func (r *CommitteeRepo) UpdateMemberRole(ctx context.Context, committeeID, memberID, role string) (*model.CommitteeMember, error) {
	var m model.CommitteeMember
	err := r.db.QueryRow(ctx,
		`UPDATE committee_members SET role = $1
		 WHERE id = $2 AND committee_id = $3
		 RETURNING id, committee_id, user_id, role, added_at`,
		role, memberID, committeeID).
		Scan(&m.ID, &m.CommitteeID, &m.UserID, &m.Role, &m.AddedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("member not found on this committee")
		}
		return nil, fmt.Errorf("update member role: %w", err)
	}
	return &m, nil
}

func (r *CommitteeRepo) RemoveMember(ctx context.Context, committeeID, memberID string) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM committee_members WHERE id = $1 AND committee_id = $2`,
		memberID, committeeID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("member not found on this committee")
	}
	return nil
}

// Delete removes a committee (cascades to its members via FK).
func (r *CommitteeRepo) Delete(ctx context.Context, committeeID string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM committees WHERE id = $1`, committeeID)
	if err != nil {
		return fmt.Errorf("delete committee: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("committee not found")
	}
	return nil
}
