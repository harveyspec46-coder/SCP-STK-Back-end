package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scp-stk/hub/internal/ilovepdf"
	"github.com/scp-stk/hub/internal/model"
	"github.com/scp-stk/hub/internal/storage"
)

// ════════════════════════════════════════════════════════════════════════════
// E-Signatures — wired to the real iLovePDF Signature REST API.
//
// Flow on Create: fetch the already-uploaded source PDF -> start/sign ->
// upload -> create-signature (emails every signer) -> persist tokens.
// Flow after that is driven entirely by iLovePDF's webhook (see
// HandleSignerEvent / HandleCompletion below), not polling.
// ════════════════════════════════════════════════════════════════════════════

type ESignRepo struct {
	db       *pgxpool.Pool
	ilovepdf *ilovepdf.Client
	storage  *storage.Client
}

func NewESignRepo(db *pgxpool.Pool, ilovepdfClient *ilovepdf.Client, storageClient *storage.Client) *ESignRepo {
	return &ESignRepo{db: db, ilovepdf: ilovepdfClient, storage: storageClient}
}

func (r *ESignRepo) List(ctx context.Context) ([]model.ESignDocument, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, name, type, pages, clauses, status, created_by, created_at,
		        source_file_url, signed_file_url, ilovepdf_server, ilovepdf_task,
		        token_requester, signature_uuid, ilovepdf_status, completed_at
		 FROM esign_documents ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list esign documents: %w", err)
	}
	defer rows.Close()

	var docs []model.ESignDocument
	for rows.Next() {
		var d model.ESignDocument
		if err := rows.Scan(&d.ID, &d.Name, &d.Type, &d.Pages, &d.Clauses,
			&d.Status, &d.CreatedBy, &d.CreatedAt,
			&d.SourceFileURL, &d.SignedFileURL, &d.ILovePDFServer, &d.ILovePDFTask,
			&d.TokenRequester, &d.SignatureUUID, &d.ILovePDFStatus, &d.CompletedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}

	// One extra query per doc is fine at this volume — same tradeoff noted
	// in the original mock implementation; revisit with a JOIN if the org
	// ever has hundreds of pending documents.
	for i := range docs {
		signers, err := r.signersForDoc(ctx, docs[i].ID)
		if err != nil {
			return nil, err
		}
		docs[i].Signers = signers
	}
	return docs, nil
}

func (r *ESignRepo) signersForDoc(ctx context.Context, docID string) ([]model.ESignSigner, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, document_id, name, email, role, user_id, signed, signature_data,
		        signed_at, ilovepdf_status, ilovepdf_token_requester
		 FROM esign_signers WHERE document_id = $1 ORDER BY id`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ESignSigner
	for rows.Next() {
		var s model.ESignSigner
		if err := rows.Scan(&s.ID, &s.DocumentID, &s.Name, &s.Email, &s.Role, &s.UserID,
			&s.Signed, &s.SignatureData, &s.SignedAt, &s.ILovePDFStatus, &s.ILovePDFToken); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// GetByTokenRequester looks up a document by its iLovePDF requester token —
// used by the webhook handler to match an incoming event back to our row,
// since the webhook payload has no idea about our internal document ID.
func (r *ESignRepo) GetByTokenRequester(ctx context.Context, tokenRequester string) (*model.ESignDocument, error) {
	var d model.ESignDocument
	err := r.db.QueryRow(ctx,
		`SELECT id, name, type, pages, clauses, status, created_by, created_at,
		        source_file_url, signed_file_url, ilovepdf_server, ilovepdf_task,
		        token_requester, signature_uuid, ilovepdf_status, completed_at
		 FROM esign_documents WHERE token_requester = $1`, tokenRequester).
		Scan(&d.ID, &d.Name, &d.Type, &d.Pages, &d.Clauses,
			&d.Status, &d.CreatedBy, &d.CreatedAt,
			&d.SourceFileURL, &d.SignedFileURL, &d.ILovePDFServer, &d.ILovePDFTask,
			&d.TokenRequester, &d.SignatureUUID, &d.ILovePDFStatus, &d.CompletedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("document not found for token_requester")
		}
		return nil, fmt.Errorf("get document by token_requester: %w", err)
	}
	return &d, nil
}

// defaultElements is applied when a signer request doesn't specify custom
// field placement — one signature field and one date field, bottom of the
// last page. The frontend's future drag-and-drop UI can override this by
// sending explicit Elements per signer.
func defaultElements() []model.ESignElement {
	return []model.ESignElement{
		{Type: "signature", Pages: "-1", Position: "bottom left", Size: 40},
		{Type: "date", Pages: "-1", Position: "bottom right", Content: "m/d/Y", Size: 12},
	}
}

func toILovePDFElements(els []model.ESignElement) []ilovepdf.Element {
	out := make([]ilovepdf.Element, 0, len(els))
	for _, e := range els {
		out = append(out, ilovepdf.Element{
			Type:     e.Type,
			Page:     e.Pages,
			Position: e.Position,
			Content:  e.Content,
			Size:     e.Size,
		})
	}
	return out
}

// filenameFromURL grabs a reasonable filename off the end of a Storage URL
// for display purposes; falls back to a generic name if parsing fails.
func filenameFromURL(url string) string {
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return "document.pdf"
	}
	name := parts[len(parts)-1]
	if name == "" || !strings.Contains(name, ".") {
		return "document.pdf"
	}
	return name
}

// Create saves the document + signers directly to the database. Signing
// happens entirely in-app (drag a saved signature onto a pre-placed field —
// see esign_fields.go), so there is no external iLovePDF call here; this
// method used to run the full iLovePDF start/sign -> upload -> create-
// signature flow, which was dropped when the project moved off it.
func (r *ESignRepo) Create(ctx context.Context, req model.CreateESignDocumentRequest, createdBy string) (*model.ESignDocument, error) {
	if req.SourceFileURL == "" {
		return nil, fmt.Errorf("source_file_url is required")
	}
	if len(req.Signers) == 0 {
		return nil, fmt.Errorf("at least one signer is required")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	id := uuid.New().String()
	var d model.ESignDocument
	err = tx.QueryRow(ctx,
		`INSERT INTO esign_documents (id, name, type, pages, clauses, status, created_by, source_file_url)
		 VALUES ($1,$2,$3,1,$4,'pending',$5,$6)
		 RETURNING id, name, type, pages, clauses, status, created_by, created_at,
		           source_file_url, signed_file_url, ilovepdf_server, ilovepdf_task,
		           token_requester, signature_uuid, ilovepdf_status, completed_at`,
		id, req.Name, req.Type, req.Clauses, createdBy, req.SourceFileURL).
		Scan(&d.ID, &d.Name, &d.Type, &d.Pages, &d.Clauses, &d.Status, &d.CreatedBy, &d.CreatedAt,
			&d.SourceFileURL, &d.SignedFileURL, &d.ILovePDFServer, &d.ILovePDFTask,
			&d.TokenRequester, &d.SignatureUUID, &d.ILovePDFStatus, &d.CompletedAt)
	if err != nil {
		return nil, fmt.Errorf("insert esign document: %w", err)
	}

	for _, s := range req.Signers {
		role := s.Role
		if role == "" {
			role = "external"
		}
		sid := uuid.New().String()
		var userID interface{}
		if s.UserID != "" {
			userID = s.UserID
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO esign_signers (id, document_id, name, email, role, user_id, signed)
			 VALUES ($1,$2,$3,$4,$5,$6,false)`,
			sid, id, s.Name, s.Email, role, userID); err != nil {
			return nil, fmt.Errorf("create signer %q: %w", s.Email, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	d.Signers, _ = r.signersForDoc(ctx, id)
	return &d, nil
}

// HandleSignerEvent updates one signer's status from a webhook event
// (signature.signer.completed, or any other per-signer status change).
// When the event marks the signer as "signed", signed/signed_at are set too.
func (r *ESignRepo) HandleSignerEvent(ctx context.Context, signerTokenRequester, status string) error {
	signed := status == "signed"
	var err error
	if signed {
		_, err = r.db.Exec(ctx,
			`UPDATE esign_signers SET ilovepdf_status = $1, signed = true, signed_at = NOW()
			 WHERE ilovepdf_token_requester = $2`, status, signerTokenRequester)
	} else {
		_, err = r.db.Exec(ctx,
			`UPDATE esign_signers SET ilovepdf_status = $1 WHERE ilovepdf_token_requester = $2`,
			status, signerTokenRequester)
	}
	if err != nil {
		return fmt.Errorf("update signer status: %w", err)
	}
	return nil
}

// MarkSignerComplete flips one signer's signed flag once all of their
// esign_fields are filled, then checks whether every signer on the document
// is now done — if so, flips the document itself to complete.
func (r *ESignRepo) MarkSignerComplete(ctx context.Context, documentID, signerID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE esign_signers SET signed = true, signed_at = NOW() WHERE id = $1`,
		signerID); err != nil {
		return fmt.Errorf("mark signer signed: %w", err)
	}

	var remaining int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM esign_signers WHERE document_id = $1 AND signed = false`,
		documentID).Scan(&remaining); err != nil {
		return fmt.Errorf("count remaining signers: %w", err)
	}

	if remaining == 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE esign_documents SET status = 'complete', completed_at = NOW() WHERE id = $1`,
			documentID); err != nil {
			return fmt.Errorf("mark document complete: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// UpdateDocumentStatus mirrors iLovePDF's own status (sent/declined/expired/
// void/etc.) onto our row without touching the simplified pending/complete
// status column — that one only flips via HandleCompletion.
func (r *ESignRepo) UpdateDocumentStatus(ctx context.Context, tokenRequester, ilovepdfStatus string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE esign_documents SET ilovepdf_status = $1 WHERE token_requester = $2`,
		ilovepdfStatus, tokenRequester)
	if err != nil {
		return fmt.Errorf("update document status: %w", err)
	}
	return nil
}

// HandleCompletion runs when the signature.completed webhook fires: it
// downloads the fully-signed PDF from iLovePDF, re-uploads it to Supabase
// Storage, and flips the document to complete.
func (r *ESignRepo) HandleCompletion(ctx context.Context, doc *model.ESignDocument) error {
	if doc.ILovePDFServer == nil || doc.TokenRequester == nil {
		return fmt.Errorf("document missing ilovepdf server/token, cannot download signed pdf")
	}

	signedBytes, err := r.ilovepdf.DownloadSigned(ctx, *doc.ILovePDFServer, *doc.TokenRequester)
	if err != nil {
		return fmt.Errorf("download signed pdf: %w", err)
	}

	path := fmt.Sprintf("signed/%s.pdf", doc.ID)
	publicURL, err := r.storage.Upload(ctx, path, signedBytes, "application/pdf")
	if err != nil {
		return fmt.Errorf("store signed pdf: %w", err)
	}

	_, err = r.db.Exec(ctx,
		`UPDATE esign_documents
		 SET status = 'complete', ilovepdf_status = 'completed',
		     signed_file_url = $1, completed_at = $2
		 WHERE id = $3`,
		publicURL, time.Now().UTC(), doc.ID)
	if err != nil {
		return fmt.Errorf("mark document complete: %w", err)
	}
	return nil
}
