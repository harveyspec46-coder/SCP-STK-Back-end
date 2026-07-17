package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ════════════════════════════════════════════════════════════════════════════
// Signature/date field markers placed on a document by the admin while
// preparing it. Signers later fill in only the marker(s) assigned to them —
// they don't choose position, just drag their saved signature/date into the
// pre-designated spot.
// ════════════════════════════════════════════════════════════════════════════

type ESignField struct {
	ID          string  `json:"id"`
	DocumentID  string  `json:"document_id"`
	SignerID    string  `json:"signer_id"`
	Type        string  `json:"type"` // "signature" | "date"
	Page        int     `json:"page"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	Filled      bool    `json:"filled"`
	FilledValue *string `json:"filled_value"`
	FilledAt    *time.Time `json:"filled_at"`
}

type FieldInput struct {
	SignerID string  `json:"signer_id"`
	Type     string  `json:"type"`
	Page     int     `json:"page"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
}

type ESignFieldsRepo struct {
	db *pgxpool.Pool
}

func NewESignFieldsRepo(db *pgxpool.Pool) *ESignFieldsRepo {
	return &ESignFieldsRepo{db: db}
}

// CreateBatch replaces all fields for a document with the given set — used
// when the admin finalizes placement while preparing the document. Wrapped
// in a transaction so a partial write never leaves a document half-marked.
func (r *ESignFieldsRepo) CreateBatch(ctx context.Context, documentID string, fields []FieldInput) ([]ESignField, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM esign_fields WHERE document_id = $1`, documentID); err != nil {
		return nil, fmt.Errorf("clear existing fields: %w", err)
	}

	out := make([]ESignField, 0, len(fields))
	for _, f := range fields {
		id := uuid.New().String()
		var saved ESignField
		err := tx.QueryRow(ctx,
			`INSERT INTO esign_fields (id, document_id, signer_id, type, page, x, y, width, height)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			 RETURNING id, document_id, signer_id, type, page, x, y, width, height, filled, filled_value, filled_at`,
			id, documentID, f.SignerID, f.Type, f.Page, f.X, f.Y, f.Width, f.Height).
			Scan(&saved.ID, &saved.DocumentID, &saved.SignerID, &saved.Type, &saved.Page,
				&saved.X, &saved.Y, &saved.Width, &saved.Height,
				&saved.Filled, &saved.FilledValue, &saved.FilledAt)
		if err != nil {
			return nil, fmt.Errorf("insert field: %w", err)
		}
		out = append(out, saved)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return out, nil
}

// ListForDocument returns every field on a document — used by both the
// admin's placement view and (filtered client-side, or via ListForSigner)
// the signer's fill-in view.
func (r *ESignFieldsRepo) ListForDocument(ctx context.Context, documentID string) ([]ESignField, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, document_id, signer_id, type, page, x, y, width, height, filled, filled_value, filled_at
		 FROM esign_fields WHERE document_id = $1 ORDER BY page, id`, documentID)
	if err != nil {
		return nil, fmt.Errorf("list fields: %w", err)
	}
	defer rows.Close()

	var out []ESignField
	for rows.Next() {
		var f ESignField
		if err := rows.Scan(&f.ID, &f.DocumentID, &f.SignerID, &f.Type, &f.Page,
			&f.X, &f.Y, &f.Width, &f.Height, &f.Filled, &f.FilledValue, &f.FilledAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// Fill marks one field as completed by its signer, storing either the saved
// signature image (data URL) or the Pacific-Time date string.
func (r *ESignFieldsRepo) Fill(ctx context.Context, fieldID, signerID, value string) (*ESignField, error) {
	var f ESignField
	err := r.db.QueryRow(ctx,
		`UPDATE esign_fields SET filled = true, filled_value = $1, filled_at = NOW()
		 WHERE id = $2 AND signer_id = $3
		 RETURNING id, document_id, signer_id, type, page, x, y, width, height, filled, filled_value, filled_at`,
		value, fieldID, signerID).
		Scan(&f.ID, &f.DocumentID, &f.SignerID, &f.Type, &f.Page,
			&f.X, &f.Y, &f.Width, &f.Height, &f.Filled, &f.FilledValue, &f.FilledAt)
	if err != nil {
		return nil, fmt.Errorf("fill field: %w", err)
	}
	return &f, nil
}

// AllFilledForSigner reports whether every field belonging to a signer on a
// document is filled — used to decide when to flip esign_signers.signed.
func (r *ESignFieldsRepo) AllFilledForSigner(ctx context.Context, documentID, signerID string) (bool, error) {
	var remaining int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM esign_fields
		 WHERE document_id = $1 AND signer_id = $2 AND filled = false`,
		documentID, signerID).Scan(&remaining)
	if err != nil {
		return false, fmt.Errorf("count unfilled fields: %w", err)
	}
	return remaining == 0, nil
}
