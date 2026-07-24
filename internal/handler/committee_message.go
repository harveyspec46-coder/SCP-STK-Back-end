package handler

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/scp-stk/hub/internal/auth"
	"github.com/scp-stk/hub/internal/model"
	"github.com/scp-stk/hub/internal/repository"
)

type CommitteeMessageHandler struct {
	repo *repository.CommitteeMessageRepo
}

func NewCommitteeMessageHandler(repo *repository.CommitteeMessageRepo) *CommitteeMessageHandler {
	return &CommitteeMessageHandler{repo: repo}
}

// GET /api/committees/{id}/messages -- committee members only
func (h *CommitteeMessageHandler) List(w http.ResponseWriter, r *http.Request) {
	committeeID := chi.URLParam(r, "id")
	user := auth.GetUser(r.Context())

	isMember, err := h.repo.IsMember(r.Context(), committeeID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify membership")
		return
	}
	if !isMember && user.Role != "admin" {
		writeError(w, http.StatusForbidden, "only committee members can view messages")
		return
	}

	messages, err := h.repo.List(r.Context(), committeeID)
	if err != nil {
		log.Printf("committee messages list failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: messages, Total: len(messages)})
}

// POST /api/committees/{id}/messages -- committee members only
// Body: { body }
func (h *CommitteeMessageHandler) Send(w http.ResponseWriter, r *http.Request) {
	committeeID := chi.URLParam(r, "id")
	var req model.SendCommitteeMessageRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	user := auth.GetUser(r.Context())

	isMember, err := h.repo.IsMember(r.Context(), committeeID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify membership")
		return
	}
	if !isMember && user.Role != "admin" {
		writeError(w, http.StatusForbidden, "only committee members can send messages")
		return
	}

	msg, err := h.repo.Send(r.Context(), committeeID, user.ID, req.Body)
	if err != nil {
		log.Printf("committee message send failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: msg})
}
