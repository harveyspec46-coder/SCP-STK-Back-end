package handler

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/scp-stk/hub/internal/auth"
	"github.com/scp-stk/hub/internal/model"
	"github.com/scp-stk/hub/internal/repository"
)

type CommitteeHandler struct {
	repo  *repository.CommitteeRepo
	audit *repository.AuditRepo
}

func NewCommitteeHandler(repo *repository.CommitteeRepo, audit *repository.AuditRepo) *CommitteeHandler {
	return &CommitteeHandler{repo: repo, audit: audit}
}

// GET /api/committees — board (admin + manager)
func (h *CommitteeHandler) List(w http.ResponseWriter, r *http.Request) {
	committees, err := h.repo.List(r.Context())
	if err != nil {
		log.Printf("committees list failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list committees")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: committees, Total: len(committees)})
}

// POST /api/committees — admin only
func (h *CommitteeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateCommitteeRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	user := auth.GetUser(r.Context())
	committee, err := h.repo.Create(r.Context(), req, user.ID)
	if err != nil {
		log.Printf("committee create failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create committee")
		return
	}

	_ = h.audit.Record(r.Context(), user.ID, "committee_created", "Committees",
		"Created committee: "+committee.Name, r.RemoteAddr)
	writeJSON(w, http.StatusCreated, model.Response{Data: committee})
}

// DELETE /api/committees/{id} — admin only
func (h *CommitteeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	committeeID := chi.URLParam(r, "id")
	if err := h.repo.Delete(r.Context(), committeeID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete committee")
		return
	}

	user := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), user.ID, "committee_deleted", "Committees",
		"Deleted committee "+committeeID, r.RemoteAddr)
	writeJSON(w, http.StatusOK, model.Response{Message: "committee deleted"})
}

// POST /api/committees/{id}/members — admin only
// Body: { user_id, role? }
func (h *CommitteeHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	committeeID := chi.URLParam(r, "id")
	var req model.AddCommitteeMemberRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if req.Role != "" && req.Role != "lead" && req.Role != "co-lead" && req.Role != "member" {
		writeError(w, http.StatusBadRequest, "role must be lead, co-lead, or member")
		return
	}

	member, err := h.repo.AddMember(r.Context(), committeeID, req.UserID, req.Role)
	if err != nil {
		log.Printf("committee add member failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to add member (they may already be on this committee)")
		return
	}

	user := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), user.ID, "committee_member_added", "Committees",
		"Added member to committee "+committeeID, r.RemoteAddr)
	writeJSON(w, http.StatusCreated, model.Response{Data: member})
}

// PATCH /api/committees/{id}/members/{memberId} — admin only
// Body: { role }
func (h *CommitteeHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	committeeID := chi.URLParam(r, "id")
	memberID := chi.URLParam(r, "memberId")
	var req model.UpdateCommitteeMemberRoleRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Role != "lead" && req.Role != "co-lead" && req.Role != "member" {
		writeError(w, http.StatusBadRequest, "role must be lead, co-lead, or member")
		return
	}

	member, err := h.repo.UpdateMemberRole(r.Context(), committeeID, memberID, req.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	user := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), user.ID, "committee_member_role_updated", "Committees",
		"Updated a member's role on committee "+committeeID, r.RemoteAddr)
	writeJSON(w, http.StatusOK, model.Response{Data: member})
}

// DELETE /api/committees/{id}/members/{memberId} — admin only
func (h *CommitteeHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	committeeID := chi.URLParam(r, "id")
	memberID := chi.URLParam(r, "memberId")

	if err := h.repo.RemoveMember(r.Context(), committeeID, memberID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	user := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), user.ID, "committee_member_removed", "Committees",
		"Removed a member from committee "+committeeID, r.RemoteAddr)
	writeJSON(w, http.StatusOK, model.Response{Message: "member removed"})
}
