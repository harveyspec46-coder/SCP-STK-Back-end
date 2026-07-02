package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scp-stk/hub/internal/auth"
	"github.com/scp-stk/hub/internal/model"
	"github.com/scp-stk/hub/internal/repository"
)

// ─── Tasks handler ────────────────────────────────────────────────────────────

type TaskHandler struct {
	repo  *repository.TaskRepo
	notif *repository.NotificationRepo
}

func NewTaskHandler(repo *repository.TaskRepo, notif *repository.NotificationRepo) *TaskHandler {
	return &TaskHandler{repo: repo, notif: notif}
}

// GET /api/tasks — returns user's own tasks; admins/managers see all
func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	var tasks []model.Task
	var err error
	if user.Role == "admin" || user.Role == "manager" {
		tasks, err = h.repo.ListAll(r.Context())
	} else {
		tasks, err = h.repo.ListForUser(r.Context(), user.ID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: tasks, Total: len(tasks)})
}

// POST /api/tasks — admin only (enforced by router); assignee must be admin/manager
func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTaskRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user := auth.GetUser(r.Context())
	task, err := h.repo.Create(r.Context(), req, user.ID)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidAssignee) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	if req.AssignedTo != user.ID {
		assignerName, _ := h.repo.GetFullName(r.Context(), user.ID)
		_ = h.notif.Create(r.Context(), req.AssignedTo, "task_assigned",
			assignerName+" has assigned you a task: "+req.Title, &task.ID)
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: task})
}

// PATCH /api/tasks/:id/status — strict two-step handoff:
//   open -> in_progress   only the assignee may do this
//   in_progress -> done   only the original assigner (creator) may do this
// Any other transition or actor is rejected.
func (h *TaskHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Status model.TaskStatus `json:"status"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user := auth.GetUser(r.Context())
	task, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	switch {
	case task.Status == model.TaskOpen && body.Status == model.TaskInProgress:
		if user.ID != task.AssignedTo {
			writeError(w, http.StatusForbidden, "only the assignee can start this task")
			return
		}
	case task.Status == model.TaskInProgress && body.Status == model.TaskDone:
		if user.ID != task.CreatedBy {
			writeError(w, http.StatusForbidden, "only the assigner can mark this task complete")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "invalid status transition")
		return
	}

	if err := h.repo.UpdateStatus(r.Context(), id, body.Status); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update task")
		return
	}

	if body.Status == model.TaskInProgress {
		actorName, _ := h.repo.GetFullName(r.Context(), user.ID)
		_ = h.notif.Create(r.Context(), task.CreatedBy, "task_started",
			actorName+" has started working on: "+task.Title, &id)
	} else if body.Status == model.TaskDone {
		actorName, _ := h.repo.GetFullName(r.Context(), user.ID)
		_ = h.notif.Create(r.Context(), task.AssignedTo, "task_completed",
			actorName+" has marked your task as done: "+task.Title, &id)
	}

	writeJSON(w, http.StatusOK, model.Response{Message: "status updated"})
}

// POST /api/tasks/:id/notify-ready — assignee signals work is done and
// notifies the assigner in real time, WITHOUT changing the status. Only the
// assigner can actually move the task to done (via UpdateStatus).
func (h *TaskHandler) NotifyReady(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user := auth.GetUser(r.Context())
	task, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if user.ID != task.AssignedTo {
		writeError(w, http.StatusForbidden, "only the assignee can notify the assigner")
		return
	}
	if task.Status != model.TaskInProgress {
		writeError(w, http.StatusBadRequest, "task must be in progress to notify")
		return
	}
	if err := h.repo.SetReadyForReview(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update task")
		return
	}
	assigneeName, _ := h.repo.GetFullName(r.Context(), user.ID)
	_ = h.notif.Create(r.Context(), task.CreatedBy, "task_ready_for_review",
		assigneeName+" has completed their assigned task: "+task.Title, &id)
	writeJSON(w, http.StatusOK, model.Response{Message: "assigner notified"})
}

// POST /api/tasks/:id/attachments — either the assigner or the assignee can
// attach one or more files, at any point. The client uploads to Supabase
// Storage first and posts the resulting URL here.
func (h *TaskHandler) AddAttachment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		FileURL  string `json:"file_url"`
		FileName string `json:"file_name"`
	}
	if err := decode(r, &body); err != nil || body.FileURL == "" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user := auth.GetUser(r.Context())
	task, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if user.ID != task.AssignedTo && user.ID != task.CreatedBy {
		writeError(w, http.StatusForbidden, "only the assigner or assignee can attach files to this task")
		return
	}
	att, err := h.repo.AddAttachment(r.Context(), id, user.ID, body.FileURL, body.FileName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to attach file")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: att})
}

// DELETE /api/tasks/:id
func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.repo.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete task")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "task deleted"})
}

// GET /api/tasks/productivity?user_id=&period=2026-04
func (h *TaskHandler) Productivity(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	period := r.URL.Query().Get("period")
	if period == "" {
		period = time.Now().Format("2006-01")
	}
	user := auth.GetUser(r.Context())
	if userID == "" {
		userID = user.ID
	}
	score, completed, total, err := h.repo.ProductivityScore(r.Context(), userID, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to calculate productivity")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: map[string]interface{}{
		"user_id":   userID,
		"period":    period,
		"score":     fmt.Sprintf("%.1f%%", score),
		"completed": completed,
		"total":     total,
	}})
}

// ─── Finance + Hours handler ──────────────────────────────────────────────────

type FinanceHandler struct {
	repo *repository.FinanceRepo
}

func NewFinanceHandler(repo *repository.FinanceRepo) *FinanceHandler {
	return &FinanceHandler{repo: repo}
}

// GET /api/hours
func (h *FinanceHandler) ListHours(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	entries, err := h.repo.ListHours(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list hours")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: entries, Total: len(entries)})
}

// POST /api/hours
func (h *FinanceHandler) LogHours(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	var req model.LogHoursRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Hours <= 0 {
		writeError(w, http.StatusBadRequest, "hours must be greater than 0")
		return
	}
	entry, err := h.repo.LogHours(r.Context(), user.ID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to log hours")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: entry})
}

// GET /api/payroll?period=2026-04
func (h *FinanceHandler) GetPayroll(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	period := r.URL.Query().Get("period")
	if period == "" {
		period = time.Now().Format("2006-01")
	}
	// Staff see only their own payroll; admin/manager can query any user
	userID := user.ID
	if (user.Role == "admin" || user.Role == "manager") && r.URL.Query().Get("user_id") != "" {
		userID = r.URL.Query().Get("user_id")
	}
	entry, err := h.repo.GetPayroll(r.Context(), userID, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get payroll")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: entry})
}

// PATCH /api/payroll/:uid/adjust — admin/manager only
func (h *FinanceHandler) AdjustPay(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	period := r.URL.Query().Get("period")
	if period == "" {
		period = time.Now().Format("2006-01")
	}
	var req model.AdjustPayRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.repo.AdjustPay(r.Context(), uid, period, req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to adjust pay")
		return
	}
	// Return updated payroll
	entry, _ := h.repo.GetPayroll(r.Context(), uid, period)
	writeJSON(w, http.StatusOK, model.Response{Data: entry, Message: "adjustment applied"})
}

// GET /api/finance/summary?period=2026-04
func (h *FinanceHandler) Summary(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = time.Now().Format("2006-01")
	}
	summary, err := h.repo.GetSummary(r.Context(), period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get summary")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: summary})
}

// ─── Grants handler ───────────────────────────────────────────────────────────

type GrantHandler struct {
	repo  *repository.GrantRepo
	notif *repository.NotificationRepo
}

func NewGrantHandler(repo *repository.GrantRepo, notif *repository.NotificationRepo) *GrantHandler {
	return &GrantHandler{repo: repo, notif: notif}
}

func (h *GrantHandler) List(w http.ResponseWriter, r *http.Request) {
	grants, err := h.repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list grants")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: grants, Total: len(grants)})
}

func (h *GrantHandler) Create(w http.ResponseWriter, r *http.Request) {
	var g model.Grant
	if err := decode(r, &g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if g.Stage == "" {
		g.Stage = model.GrantIncoming
	}
	grant, err := h.repo.Create(r.Context(), g)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create grant")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: grant})
}

func (h *GrantHandler) AdvanceStage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Stage model.GrantStage `json:"stage"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.repo.AdvanceStage(r.Context(), id, body.Stage); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to advance stage")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "grant stage updated to " + string(body.Stage)})
}

func (h *GrantHandler) Assign(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.repo.Assign(r.Context(), id, body.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign grant")
		return
	}
	_ = h.notif.Create(r.Context(), body.UserID, "task_assigned",
		"You have been assigned to prepare a grant application", &id)
	writeJSON(w, http.StatusOK, model.Response{Message: "grant assigned"})
}

// ─── Voting handler ───────────────────────────────────────────────────────────

type VotingHandler struct {
	repo  *repository.VotingRepo
	notif *repository.NotificationRepo
}

func NewVotingHandler(repo *repository.VotingRepo, notif *repository.NotificationRepo) *VotingHandler {
	return &VotingHandler{repo: repo, notif: notif}
}

func (h *VotingHandler) List(w http.ResponseWriter, r *http.Request) {
	resolutions, err := h.repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list resolutions")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: resolutions, Total: len(resolutions)})
}

func (h *VotingHandler) Create(w http.ResponseWriter, r *http.Request) {
	var res model.Resolution
	if err := decode(r, &res); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user := auth.GetUser(r.Context())
	res.ProposedBy = user.ID
	if res.OpensAt.IsZero() {
		res.OpensAt = time.Now()
	}
	created, err := h.repo.Create(r.Context(), res)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create resolution")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: created})
}

func (h *VotingHandler) Voters(w http.ResponseWriter, r *http.Request) {
	resID := chi.URLParam(r, "id")
	voters, err := h.repo.VotersForResolution(r.Context(), resID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load voters")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: voters, Total: len(voters)})
}

func (h *VotingHandler) CastVote(w http.ResponseWriter, r *http.Request) {
	resID := chi.URLParam(r, "id")
	var req model.CastVoteRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user := auth.GetUser(r.Context())
	if err := h.repo.CastVote(r.Context(), resID, user.ID, req.Choice); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "vote cast: " + string(req.Choice)})
}

// ─── Notifications handler ────────────────────────────────────────────────────

type NotificationHandler struct {
	repo *repository.NotificationRepo
}

func NewNotificationHandler(repo *repository.NotificationRepo) *NotificationHandler {
	return &NotificationHandler{repo: repo}
}

func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	notifs, err := h.repo.ListForUser(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: notifs, Total: len(notifs)})
}

func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.repo.MarkRead(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark read")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "marked as read"})
}
