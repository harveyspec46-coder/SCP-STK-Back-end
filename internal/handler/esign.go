package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/scp-stk/hub/internal/auth"
	"github.com/scp-stk/hub/internal/ilovepdf"
	"github.com/scp-stk/hub/internal/model"
	"github.com/scp-stk/hub/internal/repository"
)

// ════════════════════════════════════════════════════════════════════════════
// E-Signatures — board only (admin + manager) for the document routes;
// the webhook route is public (iLovePDF calls it directly, with no
// Supabase JWT) and is registered separately, outside the /api auth group.
// ════════════════════════════════════════════════════════════════════════════

type ESignHandler struct {
	repo    *repository.ESignRepo
	sigRepo *repository.SignatureRepo
	audit   *repository.AuditRepo
}

func NewESignHandler(repo *repository.ESignRepo, sigRepo *repository.SignatureRepo, audit *repository.AuditRepo) *ESignHandler {
	return &ESignHandler{repo: repo, sigRepo: sigRepo, audit: audit}
}

// GET /api/esign/documents
func (h *ESignHandler) List(w http.ResponseWriter, r *http.Request) {
	docs, err := h.repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list documents")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: docs, Total: len(docs)})
}

// POST /api/esign/documents
// Body: { name, type, clauses, source_file_url, signers: [{name, email, role?, elements?}] }
// The PDF at source_file_url must already be uploaded to Supabase Storage by
// the frontend (same pattern as task attachments). This single call runs
// the full iLovePDF flow — start/sign, upload, create-signature — which
// emails every signer immediately, so there's no separate "send" step.
func (h *ESignHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateESignDocumentRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.SourceFileURL == "" {
		writeError(w, http.StatusBadRequest, "source_file_url is required")
		return
	}
	if len(req.Signers) == 0 {
		writeError(w, http.StatusBadRequest, "at least one signer is required")
		return
	}
	for _, s := range req.Signers {
		if s.Name == "" || s.Email == "" {
			writeError(w, http.StatusBadRequest, "every signer needs a name and email")
			return
		}
	}

	user := auth.GetUser(r.Context())
	doc, err := h.repo.Create(r.Context(), req, user.ID)
	if err != nil {
		log.Printf("esign create failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create signature request")
		return
	}

	_ = h.audit.Record(r.Context(), user.ID, "esign_request_created", "E-Signatures",
		"Created signature request: "+doc.Name, r.RemoteAddr)
	writeJSON(w, http.StatusCreated, model.Response{Data: doc})
}

// GET /api/esign/my-signature
// Returns the caller's saved signature, or data: null if not set up yet.
func (h *ESignHandler) GetMySignature(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	sig, err := h.sigRepo.GetMine(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load signature")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: sig})
}

// POST /api/esign/my-signature
// Body: { full_name, font_style, signature_image }
// Creates or overwrites the caller's saved signature.
func (h *ESignHandler) SaveMySignature(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FullName       string `json:"full_name"`
		FontStyle      string `json:"font_style"`
		SignatureImage string `json:"signature_image"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FullName == "" || req.FontStyle == "" || req.SignatureImage == "" {
		writeError(w, http.StatusBadRequest, "full_name, font_style, and signature_image are required")
		return
	}

	user := auth.GetUser(r.Context())
	sig, err := h.sigRepo.SaveMine(r.Context(), user.ID, req.FullName, req.FontStyle, req.SignatureImage)
	if err != nil {
		log.Printf("save my-signature failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save signature")
		return
	}

	_ = h.audit.Record(r.Context(), user.ID, "esign_signature_saved", "E-Signatures",
		"Set up personal signature", r.RemoteAddr)
	writeJSON(w, http.StatusOK, model.Response{Data: sig})
}

// POST /webhooks/ilovepdf — PUBLIC, no Supabase auth. Must be registered as
// the webhook URL in the iloveapi.com dashboard (Webhooks section) for any
// of this to fire; iLovePDF only lets you configure webhook URLs there, not
// via the API.
//
// Always returns 200 quickly — iLovePDF doesn't meaningfully retry or care
// about the response body, and returning an error here just risks it
// interpreting delivery as failed for events we don't even act on.
func (h *ESignHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read webhook body")
		return
	}

	var payload ilovepdf.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("esign webhook: failed to decode payload: %v", err)
		writeJSON(w, http.StatusOK, model.Response{Message: "ignored"})
		return
	}

	switch payload.Event {
	case ilovepdf.EventSignerCompleted:
		if payload.Data.Signer != nil {
			if err := h.repo.HandleSignerEvent(r.Context(), payload.Data.Signer.TokenRequester, payload.Data.Signer.Status); err != nil {
				log.Printf("esign webhook: failed to update signer status: %v", err)
			}
		}

	case ilovepdf.EventSignatureCompleted:
		if payload.Data.Signature != nil {
			doc, err := h.repo.GetByTokenRequester(r.Context(), payload.Data.Signature.TokenRequester)
			if err != nil {
				log.Printf("esign webhook: signature.completed for unknown document: %v", err)
				break
			}
			if err := h.repo.HandleCompletion(r.Context(), doc); err != nil {
				log.Printf("esign webhook: failed to handle completion: %v", err)
			}
		}

	case ilovepdf.EventSignatureDeclined, ilovepdf.EventSignatureExpired, ilovepdf.EventSignatureVoided:
		if payload.Data.Signature != nil {
			status := payload.Data.Signature.Status
			if err := h.repo.UpdateDocumentStatus(r.Context(), payload.Data.Signature.TokenRequester, status); err != nil {
				log.Printf("esign webhook: failed to update document status: %v", err)
			}
		}

	default:
		// signature.created, signature.sent, task.completed, etc. — no
		// action needed for those in this app today.
	}

	writeJSON(w, http.StatusOK, model.Response{Message: "ok"})
}
