package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/scp-stk/hub/internal/model"
)

type CommitteeDocumentRepo struct {
	db *pgxpool.Pool
}

func NewCommitteeDocumentRepo(db *pgxpool.Pool) *CommitteeDocumentRepo {
	return &CommitteeDocumentRepo{db: db}
}

// IsMember checks whether userID is a member of committeeID -- used to
// gate document uploads to committee members only.
func (r *CommitteeDocumentRepo) IsMember(ctx context.Context, committeeID, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM committee_members WHERE committee_id = $1 AND user_id = $2)`,
		committeeID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check membership: %w", err)
	}
	return exists, nil
}

func (r *CommitteeDocumentRepo) List(ctx context.Context, committeeID string) ([]model.CommitteeDocument, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, committee_id, name, url, file_type, uploaded_by, uploaded_at
		 FROM committee_documents WHERE committee_id = $1 ORDER BY uploaded_at DESC`,
		committeeID)
	if err != nil {
		return nil, fmt.Errorf("list committee documents: %w", err)
	}
	defer rows.Close()

	var out []model.CommitteeDocument
	for rows.Next() {
		var d model.CommitteeDocument
		var fileType *string
		if err := rows.Scan(&d.ID, &d.CommitteeID, &d.Name, &d.URL, &fileType, &d.UploadedBy, &d.UploadedAt); err != nil {
			return nil, err
		}
		if fileType != nil {
			d.FileType = *fileType
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *CommitteeDocumentRepo) Add(ctx context.Context, committeeID string, req model.AddCommitteeDocumentRequest, uploadedBy string) (*model.CommitteeDocument, error) {
	id := uuid.New().String()
	var fileType *string
	if req.FileType != "" {
		fileType = &req.FileType
	}
	var d model.CommitteeDocument
	err := r.db.QueryRow(ctx,
		`INSERT INTO committee_documents (id, committee_id, name, url, file_type, uploaded_by)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, committee_id, name, url, file_type, uploaded_by, uploaded_at`,
		id, committeeID, req.Name, req.URL, fileType, uploadedBy).
		Scan(&d.ID, &d.CommitteeID, &d.Name, &d.URL, &fileType, &d.UploadedBy, &d.UploadedAt)
	if err != nil {
		return nil, fmt.Errorf("insert committee document: %w", err)
	}
	if fileType != nil {
		d.FileType = *fileType
	}
	return &d, nil
}

var ErrNotDocumentUploaderOrAdmin = fmt.Errorf("only the uploader or an admin can delete this document")

// Delete removes a document, but only if requestedBy uploaded it OR
// requesterIsAdmin is true.
func (r *CommitteeDocumentRepo) Delete(ctx context.Context, documentID, requestedBy string, requesterIsAdmin bool) error {
	var uploadedBy string
	err := r.db.QueryRow(ctx,
		`SELECT uploaded_by FROM committee_documents WHERE id = $1`, documentID).Scan(&uploadedBy)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("document not found")
		}
		return fmt.Errorf("lookup document uploader: %w", err)
	}
	if uploadedBy != requestedBy && !requesterIsAdmin {
		return ErrNotDocumentUploaderOrAdmin
	}

	if _, err := r.db.Exec(ctx, `DELETE FROM committee_documents WHERE id = $1`, documentID); err != nil {
		return fmt.Errorf("delete committee document: %w", err)
	}
	return nil
}
