package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scp-stk/hub/internal/auth"
	"github.com/scp-stk/hub/internal/model"
	"github.com/scp-stk/hub/internal/repository"
)

type CRMHandler struct {
	crm   *repository.CRMRepo
	task  *repository.TaskRepo
	notif *repository.NotificationRepo
}

func NewCRMHandler(crm *repository.CRMRepo, task *repository.TaskRepo, notif *repository.NotificationRepo) *CRMHandler {
	return &CRMHandler{crm: crm, task: task, notif: notif}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, model.Response{Error: msg})
}

func decode(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ─── Client handlers ──────────────────────────────────────────────────────────

// GET /api/crm/clients?search=&office=
func (h *CRMHandler) ListClients(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	clients, err := h.crm.ListClients(r.Context(), search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list clients")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: clients, Total: len(clients)})
}

// POST /api/crm/clients
func (h *CRMHandler) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req model.CreateClientRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FullName == "" || req.Phone == "" {
		writeError(w, http.StatusBadRequest, "full_name and phone are required")
		return
	}
	client, err := h.crm.CreateClient(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create client")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: client})
}

// GET /api/crm/clients/:id
func (h *CRMHandler) GetClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	client, err := h.crm.GetClient(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "client not found")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: client})
}

// PUT /api/crm/clients/:id
func (h *CRMHandler) UpdateClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req model.CreateClientRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	client, err := h.crm.UpdateClient(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update client")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: client})
}

// DELETE /api/crm/clients/:id — admin only
func (h *CRMHandler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.crm.DeleteClient(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete client")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "client deleted"})
}

// ─── Job handlers ─────────────────────────────────────────────────────────────

// GET /api/crm/jobs?stage=&service_type=
// Returns jobs grouped by funnel stage for Kanban board
func (h *CRMHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	stage := r.URL.Query().Get("stage")
	serviceType := r.URL.Query().Get("service_type")
	office := r.URL.Query().Get("office")

	jobs, err := h.crm.ListJobs(r.Context(), stage, serviceType, office)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	// Group by stage for Kanban view
	type KanbanBoard struct {
		Scheduled []model.Job `json:"job_scheduled"`
		Assigned  []model.Job `json:"staff_assigned"`
		Arrived   []model.Job `json:"arrived_at_site"`
		Completed []model.Job `json:"completed"`
		Invoiced  []model.Job `json:"invoiced"`
	}
	board := KanbanBoard{}
	for _, j := range jobs {
		switch j.Stage {
		case "job_scheduled":
			board.Scheduled = append(board.Scheduled, j)
		case "staff_assigned":
			board.Assigned = append(board.Assigned, j)
		case "arrived_at_site":
			board.Arrived = append(board.Arrived, j)
		case "completed":
			board.Completed = append(board.Completed, j)
		case "invoiced":
			board.Invoiced = append(board.Invoiced, j)
		}
	}
	writeJSON(w, http.StatusOK, model.Response{Data: board, Total: len(jobs)})
}

// POST /api/crm/jobs
func (h *CRMHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req model.CreateJobRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ClientID == "" || req.ServiceType == "" || req.Address == "" {
		writeError(w, http.StatusBadRequest, "client_id, service_type, and address are required")
		return
	}
	job, err := h.crm.CreateJob(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create job")
		return
	}

	// Assign staff at creation time (new simplified flow: admin creates AND
	// assigns in one step). Each assignment notifies that staff member.
	for _, uid := range req.AssignedTo {
		if uid == "" {
			continue
		}
		_, _ = h.crm.AssignStaff(r.Context(), job.ID, model.AssignStaffRequest{
			UserID:    uid,
			RoleOnJob: "support",
		})
	}
	if len(req.AssignedTo) > 0 {
		updated, err := h.crm.AdvanceStage(r.Context(), job.ID, model.AdvanceStageRequest{Stage: model.JobStage("staff_assigned")})
		if err == nil {
			job = updated
		}
	}

	writeJSON(w, http.StatusCreated, model.Response{Data: job})
}

// GET /api/crm/jobs/:id
func (h *CRMHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.crm.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: job})
}

// PUT /api/crm/jobs/:id
// DELETE /api/crm/jobs/:id — admin only
func (h *CRMHandler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.crm.DeleteJob(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete job")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "job deleted"})
}

func (h *CRMHandler) UpdateJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req model.UpdateJobRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	job, err := h.crm.UpdateJob(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update job")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: job})
}

// PATCH /api/crm/jobs/:id/stage
// Advances the job through the funnel. Records timestamps automatically.
func (h *CRMHandler) AdvanceStage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req model.AdvanceStageRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Admins may only move a job from completed to invoiced — every earlier
	// transition belongs to the assigned staff member via their own
	// checkin/complete endpoints. This is enforced here, not just hidden in
	// the UI, since this route is reachable directly.
	existing, err := h.crm.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if !(existing.Stage == "completed" && req.Stage == "invoiced") {
		writeError(w, http.StatusForbidden, "admins can only move a completed job to invoiced")
		return
	}
	job, err := h.crm.AdvanceStage(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to advance stage")
		return
	}

	// Fire notification to all assigned staff when stage changes
	user := auth.GetUser(r.Context())
	for _, asgn := range job.Assignments {
		if asgn.UserID != user.ID {
			_ = h.notif.Create(r.Context(), asgn.UserID, "job_stage",
				"Job "+job.ServiceType+" at "+job.Address+" moved to "+string(req.Stage), &id)
		}
	}

	writeJSON(w, http.StatusOK, model.Response{Data: job})
}

// POST /api/crm/jobs/:id/assign
// Assigns a staff member to a job. Staff can have multiple concurrent job assignments.
func (h *CRMHandler) AssignStaff(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	var req model.AssignStaffRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if req.RoleOnJob == "" {
		req.RoleOnJob = "support"
	}

	// Check current workload — informational only, we do NOT block assignment
	// because staff CAN work multiple jobs simultaneously
	workload, _ := h.crm.StaffWorkload(r.Context(), req.UserID)
	activeJobs := len(workload)

	assignment, err := h.crm.AssignStaff(r.Context(), jobID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign staff")
		return
	}
	if req.ScheduledAt != nil {
		_ = h.crm.SetJobSchedule(r.Context(), jobID, *req.ScheduledAt)
	}
	// Notify the assigned staff member
	job, _ := h.crm.GetJob(r.Context(), jobID)
	if job != nil {
		body := "You've been assigned to a " + job.ServiceType + " job at " + job.Address
		if job.ScheduledAt != nil {
			body += " on " + job.ScheduledAt.Format("Jan 2, 2006 at 3:04 PM")
		}
		if activeJobs > 0 {
			body += " (you have " + itoa(activeJobs) + " other active job(s))"
		}
		_ = h.notif.Create(r.Context(), req.UserID, "task_assigned", body, &jobID)
	}

	writeJSON(w, http.StatusCreated, model.Response{
		Data:    assignment,
		Message: "staff assigned — current workload: " + itoa(activeJobs) + " other active jobs",
	})
}

// PATCH /api/crm/jobs/:id/reschedule — moves an already-scheduled job to a
// new time without touching its staff assignments (used for drag-to-reschedule
// in the Shift & Schedule calendar).
func (h *CRMHandler) RescheduleJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	var body struct {
		ScheduledAt time.Time `json:"scheduled_at"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.crm.SetJobSchedule(r.Context(), jobID, body.ScheduledAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reschedule job")
		return
	}
	job, _ := h.crm.GetJob(r.Context(), jobID)
	writeJSON(w, http.StatusOK, model.Response{Data: job, Message: "job rescheduled"})
}

// DELETE /api/crm/jobs/:id/assign/:uid
func (h *CRMHandler) RemoveAssignment(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "uid")
	if err := h.crm.RemoveAssignment(r.Context(), jobID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove assignment")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "assignment removed"})
}

// POST /api/crm/jobs/:id/tasks
func (h *CRMHandler) CreateJobTask(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	var req model.CreateTaskRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.JobID = &jobID
	user := auth.GetUser(r.Context())

	task, err := h.task.Create(r.Context(), req, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	// Notify assignee
	if req.AssignedTo != user.ID {
		_ = h.notif.Create(r.Context(), req.AssignedTo, "task_assigned",
			"New task for job: "+req.Title, &jobID)
	}

	writeJSON(w, http.StatusCreated, model.Response{Data: task})
}

// ─── Staff-facing "My Jobs" endpoints ──────────────────────────────────────
// Unlike the /crm/* routes above, these are open to ANY authenticated user
// (staff included) but only ever expose/act on jobs that user is personally
// assigned to — never the full client list or other staff's jobs.

// GET /api/my-jobs
func (h *CRMHandler) MyJobs(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	jobs, err := h.crm.MyJobs(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load your jobs")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: jobs, Total: len(jobs)})
}

// PATCH /api/my-jobs/:id/checkin — staff marks that they've arrived on site.
func (h *CRMHandler) CheckIn(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	user := auth.GetUser(r.Context())

	assigned, err := h.crm.IsAssignedToJob(r.Context(), jobID, user.ID)
	if err != nil || !assigned {
		writeError(w, http.StatusForbidden, "you are not assigned to this job")
		return
	}

	job, err := h.crm.AdvanceStage(r.Context(), jobID, model.AdvanceStageRequest{Stage: model.StageArrived})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check in")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: job, Message: "checked in"})
}

// PATCH /api/my-jobs/:id/complete — staff marks the job done; notifies admins.
func (h *CRMHandler) CompleteMyJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	user := auth.GetUser(r.Context())

	assigned, err := h.crm.IsAssignedToJob(r.Context(), jobID, user.ID)
	if err != nil || !assigned {
		writeError(w, http.StatusForbidden, "you are not assigned to this job")
		return
	}

	var body struct {
		ActualHours float64 `json:"actual_hours"`
		Note        string  `json:"note"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	notesText := "Actual hours: " + ftoa(body.ActualHours)
	if body.Note != "" {
		notesText += ". " + body.Note
	}

	job, err := h.crm.CompleteJob(r.Context(), jobID, notesText)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete job")
		return
	}

	adminIDs, _ := h.crm.AdminUserIDs(r.Context())
	body2 := user.DisplayID + " completed the " + job.ServiceType + " job at " + job.Address
	for _, aid := range adminIDs {
		_ = h.notif.Create(r.Context(), aid, "job_completed", body2, &jobID)
	}

	writeJSON(w, http.StatusOK, model.Response{Data: job, Message: "job marked complete"})
}

// GET /api/crm/staff/:uid/workload
func (h *CRMHandler) StaffWorkload(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "uid")
	jobs, err := h.crm.StaffWorkload(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get workload")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: jobs, Total: len(jobs)})
}

// ftoa formats a float with 1 decimal place.
func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'f', 1, 64)
}

// itoa is a simple int-to-string helper to avoid importing strconv everywhere.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
