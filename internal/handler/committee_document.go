package handler

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/scp-stk/hub/internal/auth"
	"github.com/scp-stk/hub/internal/model"
	"github.com/scp-stk/hub/internal/repository"
)

type CommitteeDocumentHandler struct {
	repo  *repository.CommitteeDocumentRepo
	audit *repository.AuditRepo
}

func NewCommitteeDocumentHandler(repo *repository.CommitteeDocumentRepo, audit *repository.AuditRepo) *CommitteeDocumentHandler {
	return &CommitteeDocumentHandler{repo: repo, audit: audit}
}

// GET /api/committees/{id}/documents -- any board member (route group
// already requires manager role minimum)
func (h *CommitteeDocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	committeeID := chi.URLParam(r, "id")
	docs, err := h.repo.List(r.Context(), committeeID)
	if err != nil {
		log.Printf("committee documents list failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list documents")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: docs, Total: len(docs)})
}

// POST /api/committees/{id}/documents -- committee members only
// Body: { name, url, file_type? }
// The file itself must already be uploaded to Supabase Storage
// (committee-documents bucket) by the frontend; url is that Storage URL.
func (h *CommitteeDocumentHandler) Add(w http.ResponseWriter, r *http.Request) {
	committeeID := chi.URLParam(r, "id")
	var req model.AddCommitteeDocumentRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}

	user := auth.GetUser(r.Context())

	isMember, err := h.repo.IsMember(r.Context(), committeeID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify membership")
		return
	}
	if !isMember && user.Role != "admin" {
		writeError(w, http.StatusForbidden, "only committee members can add documents")
		return
	}

	doc, err := h.repo.Add(r.Context(), committeeID, req, user.ID)
	if err != nil {
		log.Printf("committee document add failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to add document")
		return
	}

	_ = h.audit.Record(r.Context(), user.ID, "committee_document_added", "Committees",
		"Added document to committee "+committeeID+": "+doc.Name, r.RemoteAddr)
	writeJSON(w, http.StatusCreated, model.Response{Data: doc})
}

// DELETE /api/committees/{id}/documents/{docId} -- uploader or admin only
func (h *CommitteeDocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docId")
	user := auth.GetUser(r.Context())

	err := h.repo.Delete(r.Context(), docID, user.ID, user.Role == "admin")
	if err != nil {
		if err == repository.ErrNotDocumentUploaderOrAdmin {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		log.Printf("committee document delete failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete document")
		return
	}

	_ = h.audit.Record(r.Context(), user.ID, "committee_document_deleted", "Committees",
		"Deleted a committee document", r.RemoteAddr)
	writeJSON(w, http.StatusOK, model.Response{Message: "document deleted"})
}
