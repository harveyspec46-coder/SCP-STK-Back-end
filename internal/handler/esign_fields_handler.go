package handler

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/scp-stk/hub/internal/auth"
	"github.com/scp-stk/hub/internal/model"
	"github.com/scp-stk/hub/internal/repository"
)

type ESignFieldsHandler struct {
	fields  *repository.ESignFieldsRepo
	signers *repository.ESignRepo
	audit   *repository.AuditRepo
}

func NewESignFieldsHandler(fields *repository.ESignFieldsRepo, signers *repository.ESignRepo, audit *repository.AuditRepo) *ESignFieldsHandler {
	return &ESignFieldsHandler{fields: fields, signers: signers, audit: audit}
}

// GET /api/esign/documents/{id}/fields
func (h *ESignFieldsHandler) List(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")
	fields, err := h.fields.ListForDocument(r.Context(), docID)
	if err != nil {
		log.Printf("esign fields list failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list fields")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: fields, Total: len(fields)})
}

// POST /api/esign/documents/{id}/fields
// Body: { fields: [{ signer_id, type, page, x, y, width, height }] }
// Replaces all existing fields for the document — called once when the
// admin finalizes placement while preparing it.
func (h *ESignFieldsHandler) Create(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")
	var req struct {
		Fields []repository.FieldInput `json:"fields"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Fields) == 0 {
		writeError(w, http.StatusBadRequest, "at least one field is required")
		return
	}
	for _, f := range req.Fields {
		if f.SignerID == "" || (f.Type != "signature" && f.Type != "date") {
			writeError(w, http.StatusBadRequest, "each field needs a signer_id and type of signature or date")
			return
		}
	}

	saved, err := h.fields.CreateBatch(r.Context(), docID, req.Fields)
	if err != nil {
		log.Printf("esign fields create failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save fields")
		return
	}

	user := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), user.ID, "esign_fields_placed", "E-Signatures",
		"Placed signature fields on document "+docID, r.RemoteAddr)
	writeJSON(w, http.StatusCreated, model.Response{Data: saved, Total: len(saved)})
}

// POST /api/esign/documents/{id}/signers/{signerId}/finalize
// Idempotent: checks whether every field belonging to this signer on this
// document is filled, and if so marks the signer (and possibly the whole
// document) complete. Safe to call even if already complete, or if fields
// were filled in an earlier session — this is the only place that
// guarantees completion status gets corrected regardless of how the fields
// ended up filled.
func (h *ESignFieldsHandler) Finalize(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")
	signerID := chi.URLParam(r, "signerId")

	allDone, err := h.fields.AllFilledForSigner(r.Context(), docID, signerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check field completion")
		return
	}
	if !allDone {
		writeJSON(w, http.StatusOK, model.Response{Message: "not all fields filled yet", Data: map[string]bool{"complete": false}})
		return
	}

	if err := h.signers.MarkSignerComplete(r.Context(), docID, signerID); err != nil {
		log.Printf("esign fields finalize: failed to mark signer complete: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to finalize signer")
		return
	}

	writeJSON(w, http.StatusOK, model.Response{Message: "signer finalized", Data: map[string]bool{"complete": true}})
}

// PATCH /api/esign/fields/{id}
// Body: { signer_id, value }
// value is either the signer's saved signature image (data URL) or the
// Pacific-Time date string, depending on the field's type.
func (h *ESignFieldsHandler) Fill(w http.ResponseWriter, r *http.Request) {
	fieldID := chi.URLParam(r, "id")
	var req struct {
		SignerID string `json:"signer_id"`
		Value    string `json:"value"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SignerID == "" || req.Value == "" {
		writeError(w, http.StatusBadRequest, "signer_id and value are required")
		return
	}

	field, err := h.fields.Fill(r.Context(), fieldID, req.SignerID, req.Value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fill field")
		return
	}

	allDone, err := h.fields.AllFilledForSigner(r.Context(), field.DocumentID, req.SignerID)
	if err == nil && allDone {
		if err := h.signers.MarkSignerComplete(r.Context(), field.DocumentID, req.SignerID); err != nil {
			log.Printf("esign fields: failed to mark signer complete: %v", err)
		}
	}

	user := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), user.ID, "esign_field_filled", "E-Signatures",
		"Filled a "+field.Type+" field", r.RemoteAddr)
	writeJSON(w, http.StatusOK, model.Response{Data: field})
}
