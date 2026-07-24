package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/scp-stk/hub/internal/model"
)

type CommitteeMessageRepo struct {
	db *pgxpool.Pool
}

func NewCommitteeMessageRepo(db *pgxpool.Pool) *CommitteeMessageRepo {
	return &CommitteeMessageRepo{db: db}
}

// IsMember checks whether userID is a member of committeeID -- used to gate
// both reading and sending messages to committee members only.
func (r *CommitteeMessageRepo) IsMember(ctx context.Context, committeeID, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM committee_members WHERE committee_id = $1 AND user_id = $2)`,
		committeeID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check membership: %w", err)
	}
	return exists, nil
}

// List returns messages oldest-first (chat reads top-to-bottom), capped at
// 200 most recent -- plenty for a committee chat, and keeps the payload
// small since the frontend refetches the whole list on every realtime event
// rather than tracking incremental state.
func (r *CommitteeMessageRepo) List(ctx context.Context, committeeID string) ([]model.CommitteeMessage, error) {
	rows, err := r.db.Query(ctx,
		`SELECT cm.id, cm.committee_id, cm.user_id, cm.body, cm.created_at,
		        COALESCE(u.full_name, '') AS full_name
		 FROM committee_messages cm
		 LEFT JOIN users u ON u.id = cm.user_id
		 WHERE cm.committee_id = $1
		 ORDER BY cm.created_at DESC
		 LIMIT 200`,
		committeeID)
	if err != nil {
		return nil, fmt.Errorf("list committee messages: %w", err)
	}
	defer rows.Close()

	var out []model.CommitteeMessage
	for rows.Next() {
		var m model.CommitteeMessage
		if err := rows.Scan(&m.ID, &m.CommitteeID, &m.UserID, &m.Body, &m.CreatedAt, &m.FullName); err != nil {
			return nil, err
		}
		out = append(out, m)
	}

	// Reverse to oldest-first for display (query above pulls DESC + LIMIT to
	// cap at the 200 most recent, which requires DESC ordering first).
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (r *CommitteeMessageRepo) Send(ctx context.Context, committeeID, userID, body string) (*model.CommitteeMessage, error) {
	id := uuid.New().String()
	var m model.CommitteeMessage
	err := r.db.QueryRow(ctx,
		`INSERT INTO committee_messages (id, committee_id, user_id, body)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id, committee_id, user_id, body, created_at`,
		id, committeeID, userID, body).
		Scan(&m.ID, &m.CommitteeID, &m.UserID, &m.Body, &m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("send committee message: %w", err)
	}
	return &m, nil
}
