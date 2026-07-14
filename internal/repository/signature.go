package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ════════════════════════════════════════════════════════════════════════════
// User-level saved signatures — set up once per admin/manager, then reused
// on every document they sign. Separate from esign_documents/esign_signers,
// which track per-document state.
// ════════════════════════════════════════════════════════════════════════════

type UserSignature struct {
	UserID         string `json:"user_id"`
	FullName       string `json:"full_name"`
	FontStyle      string `json:"font_style"`
	SignatureImage string `json:"signature_image"`
}

type SignatureRepo struct {
	db *pgxpool.Pool
}

func NewSignatureRepo(db *pgxpool.Pool) *SignatureRepo {
	return &SignatureRepo{db: db}
}

// GetMine returns the caller's saved signature, or nil if they haven't set
// one up yet (not an error — the frontend uses this to decide whether to
// show the "set up your signature" screen).
func (r *SignatureRepo) GetMine(ctx context.Context, userID string) (*UserSignature, error) {
	var s UserSignature
	err := r.db.QueryRow(ctx,
		`SELECT user_id, full_name, font_style, signature_image
		 FROM user_signatures WHERE user_id = $1`, userID).
		Scan(&s.UserID, &s.FullName, &s.FontStyle, &s.SignatureImage)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user signature: %w", err)
	}
	return &s, nil
}

// SaveMine creates or overwrites the caller's saved signature — matches the
// "set it up once" design; there's intentionally no history of prior styles.
func (r *SignatureRepo) SaveMine(ctx context.Context, userID, fullName, fontStyle, signatureImage string) (*UserSignature, error) {
	var s UserSignature
	err := r.db.QueryRow(ctx,
		`INSERT INTO user_signatures (user_id, full_name, font_style, signature_image, updated_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (user_id) DO UPDATE
		   SET full_name = $2, font_style = $3, signature_image = $4, updated_at = NOW()
		 RETURNING user_id, full_name, font_style, signature_image`,
		userID, fullName, fontStyle, signatureImage).
		Scan(&s.UserID, &s.FullName, &s.FontStyle, &s.SignatureImage)
	if err != nil {
		return nil, fmt.Errorf("save user signature: %w", err)
	}
	return &s, nil
}
