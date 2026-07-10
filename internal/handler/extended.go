package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scp-stk/hub/internal/auth"
	"github.com/scp-stk/hub/internal/model"
	"github.com/scp-stk/hub/internal/repository"
)

// ════════════════════════════════════════════════════════════════════════════
// Admin — allowlist + Users & IDs panel
// ════════════════════════════════════════════════════════════════════════════

type AdminHandler struct {
	admin *repository.AdminRepo
	audit *repository.AuditRepo
}

func NewAdminHandler(admin *repository.AdminRepo, audit *repository.AuditRepo) *AdminHandler {
	return &AdminHandler{admin: admin, audit: audit}
}

// GET /api/admin/allowlist — admin only
// Shows which emails will be auto-recognized as Admin the moment they sign up.
func (h *AdminHandler) ListAllowlist(w http.ResponseWriter, r *http.Request) {
	entries, err := h.admin.ListAllowlist(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list allowlist")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: entries, Total: len(entries)})
}

// POST /api/admin/allowlist — admin only
// Add a new email (e.g. a 4th board admin) so the platform recognizes them
// as Admin automatically on their first signup — no manual ID assignment needed.
func (h *AdminHandler) AddAllowlistEntry(w http.ResponseWriter, r *http.Request) {
	var req model.AddAllowlistRequest
	if err := decode(r, &req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	user := auth.GetUser(r.Context())
	entry, err := h.admin.AddToAllowlist(r.Context(), req.Email, req.Note, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add allowlist entry")
		return
	}
	_ = h.audit.Record(r.Context(), user.ID, "admin_allowlisted", "Users & IDs",
		"Added "+req.Email+" to the admin allowlist", r.RemoteAddr)
	writeJSON(w, http.StatusCreated, model.Response{Data: entry})
}

// DELETE /api/admin/allowlist/{email} — admin only
func (h *AdminHandler) RemoveAllowlistEntry(w http.ResponseWriter, r *http.Request) {
	email := chi.URLParam(r, "email")
	if err := h.admin.RemoveFromAllowlist(r.Context(), email); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove allowlist entry")
		return
	}
	user := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), user.ID, "admin_allowlist_removed", "Users & IDs",
		"Removed "+email+" from the admin allowlist", r.RemoteAddr)
	writeJSON(w, http.StatusOK, model.Response{Message: "removed from allowlist"})
}

// GET /api/admin/users — admin only
// Powers the "Users & IDs" panel: shows every account, role, and display_id.
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.admin.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: users, Total: len(users)})
}

// PATCH /api/admin/users/{id}/display-id — admin only
// Manual fallback for managers/staff whose ID wasn't auto-assigned at signup.
func (h *AdminHandler) AssignDisplayID(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var req struct {
		DisplayID string `json:"display_id"`
	}
	if err := decode(r, &req); err != nil || req.DisplayID == "" {
		writeError(w, http.StatusBadRequest, "display_id is required")
		return
	}
	if err := h.admin.AssignDisplayID(r.Context(), userID, req.DisplayID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign display id")
		return
	}
	actor := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), actor.ID, "id_assigned", "Users & IDs",
		"Assigned "+req.DisplayID+" to user "+userID, r.RemoteAddr)
	writeJSON(w, http.StatusOK, model.Response{Message: "display_id assigned"})
}

// ════════════════════════════════════════════════════════════════════════════
// Participants & Volunteers
// ════════════════════════════════════════════════════════════════════════════

// ════════════════════════════════════════════════════════════════════════════
// Programs & Documents
// ════════════════════════════════════════════════════════════════════════════

type ProgramHandler struct {
	repo *repository.ProgramRepo
}

func NewProgramHandler(repo *repository.ProgramRepo) *ProgramHandler {
	return &ProgramHandler{repo: repo}
}

// GET /api/programs
func (h *ProgramHandler) List(w http.ResponseWriter, r *http.Request) {
	programs, err := h.repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list programs")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: programs, Total: len(programs)})
}

// POST /api/programs
func (h *ProgramHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateProgramRequest
	if err := decode(r, &req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	program, err := h.repo.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create program")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: program})
}

// GET /api/programs/:id/documents
func (h *ProgramHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	programID := chi.URLParam(r, "id")
	docs, err := h.repo.ListDocuments(r.Context(), programID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list documents")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: docs, Total: len(docs)})
}

// POST /api/programs/:id/documents
func (h *ProgramHandler) CreateDocument(w http.ResponseWriter, r *http.Request) {
	programID := chi.URLParam(r, "id")
	var req model.CreateDocumentRequest
	if err := decode(r, &req); err != nil || req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}
	user := auth.GetUser(r.Context())
	doc, err := h.repo.CreateDocument(r.Context(), programID, user.ID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add document")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: doc})
}


type ProgramLedgerHandler struct {
	repo *repository.ProgramLedgerRepo
}

func NewProgramLedgerHandler(repo *repository.ProgramLedgerRepo) *ProgramLedgerHandler {
	return &ProgramLedgerHandler{repo: repo}
}

// GET /api/programs/:id/ledger
func (h *ProgramLedgerHandler) List(w http.ResponseWriter, r *http.Request) {
	programID := chi.URLParam(r, "id")
	entries, err := h.repo.ListByProgram(r.Context(), programID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list ledger")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: entries, Total: len(entries)})
}

// POST /api/program-ledger
func (h *ProgramLedgerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateProgramLedgerRequest
	if err := decode(r, &req); err != nil || req.FullName == "" {
		writeError(w, http.StatusBadRequest, "full_name is required")
		return
	}
	user := auth.GetUser(r.Context())
	entry, err := h.repo.Create(r.Context(), user.ID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add ledger entry")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: entry})
}

type ParticipantHandler struct {
	repo  *repository.ParticipantRepo
	audit *repository.AuditRepo
}

func NewParticipantHandler(repo *repository.ParticipantRepo, audit *repository.AuditRepo) *ParticipantHandler {
	return &ParticipantHandler{repo: repo, audit: audit}
}

// GET /api/participants?type=participant|volunteer&search=
func (h *ParticipantHandler) List(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	search := r.URL.Query().Get("search")
	parts, err := h.repo.List(r.Context(), typeFilter, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list participants")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: parts, Total: len(parts)})
}

// POST /api/participants
// type = "participant" (gone through our programs) or "volunteer" (willing to
// help with org tasks/programs). display_id is auto-assigned: PAR-XXX or VOL-XXX.
func (h *ParticipantHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateParticipantRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FullName == "" {
		writeError(w, http.StatusBadRequest, "full_name is required")
		return
	}
	p, err := h.repo.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create participant")
		return
	}
	user := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), user.ID, "participant_added", "Participants",
		"Added "+string(p.Type)+" "+p.FullName+" ("+p.DisplayID+")", r.RemoteAddr)
	writeJSON(w, http.StatusCreated, model.Response{Data: p})
}

// PATCH /api/participants/{id}/stage
func (h *ParticipantHandler) AdvanceStage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req model.AdvanceParticipantStageRequest
	if err := decode(r, &req); err != nil || req.Stage == "" {
		writeError(w, http.StatusBadRequest, "stage is required")
		return
	}
	if err := h.repo.AdvanceStage(r.Context(), id, req.Stage); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to advance stage")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "stage advanced to " + req.Stage})
}

// ════════════════════════════════════════════════════════════════════════════
// MOUs & Contracts
// ════════════════════════════════════════════════════════════════════════════

type MOUHandler struct {
	repo  *repository.MOURepo
	audit *repository.AuditRepo
}

func NewMOUHandler(repo *repository.MOURepo, audit *repository.AuditRepo) *MOUHandler {
	return &MOUHandler{repo: repo, audit: audit}
}

// GET /api/mous — status (active/expiring/expired) is recalculated on every read.
func (h *MOUHandler) List(w http.ResponseWriter, r *http.Request) {
	mous, err := h.repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list MOUs")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: mous, Total: len(mous)})
}

// POST /api/mous
func (h *MOUHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateMOURequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Partner == "" || req.ExpiresOn.IsZero() {
		writeError(w, http.StatusBadRequest, "partner and expires_on are required")
		return
	}
	m, err := h.repo.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create MOU")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: m})
}

// PATCH /api/mous/{id}/renew
func (h *MOUHandler) Renew(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		NewExpiry time.Time `json:"new_expiry"`
	}
	if err := decode(r, &req); err != nil || req.NewExpiry.IsZero() {
		writeError(w, http.StatusBadRequest, "new_expiry is required")
		return
	}
	if err := h.repo.Renew(r.Context(), id, req.NewExpiry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to renew MOU")
		return
	}
	user := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), user.ID, "mou_renewed", "MOUs",
		"Renewed MOU "+id+" through "+req.NewExpiry.Format("2006-01-02"), r.RemoteAddr)
	writeJSON(w, http.StatusOK, model.Response{Message: "MOU renewed"})
}

// ════════════════════════════════════════════════════════════════════════════
// Resources (documents + training videos)
// ════════════════════════════════════════════════════════════════════════════

type ResourceHandler struct {
	repo *repository.ResourceRepo
}

func NewResourceHandler(repo *repository.ResourceRepo) *ResourceHandler {
	return &ResourceHandler{repo: repo}
}

// GET /api/resources?type=doc|video
func (h *ResourceHandler) List(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	resources, err := h.repo.List(r.Context(), typeFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list resources")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: resources, Total: len(resources)})
}

// POST /api/resources
func (h *ResourceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateResourceRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" || req.Type == "" {
		writeError(w, http.StatusBadRequest, "title and type are required")
		return
	}
	user := auth.GetUser(r.Context())
	res, err := h.repo.Create(r.Context(), req, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create resource")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: res})
}

// ════════════════════════════════════════════════════════════════════════════
// Workshops
// ════════════════════════════════════════════════════════════════════════════

type WorkshopHandler struct {
	repo *repository.WorkshopRepo
}

func NewWorkshopHandler(repo *repository.WorkshopRepo) *WorkshopHandler {
	return &WorkshopHandler{repo: repo}
}

// GET /api/workshops?status=upcoming|completed
func (h *WorkshopHandler) List(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	workshops, err := h.repo.List(r.Context(), statusFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workshops")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: workshops, Total: len(workshops)})
}

// POST /api/workshops
func (h *WorkshopHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateWorkshopRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	user := auth.GetUser(r.Context())
	ws, err := h.repo.Create(r.Context(), req, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workshop")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: ws})
}

// PATCH /api/workshops/{id}/recording
func (h *WorkshopHandler) AddRecording(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req model.AddRecordingRequest
	if err := decode(r, &req); err != nil || req.RecordingURL == "" {
		writeError(w, http.StatusBadRequest, "recording_url is required")
		return
	}
	if err := h.repo.AddRecording(r.Context(), id, req.RecordingURL); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save recording link")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "recording saved"})
}

// ════════════════════════════════════════════════════════════════════════════
// Shift schedule
// ════════════════════════════════════════════════════════════════════════════

type ShiftHandler struct {
	repo  *repository.ShiftRepo
	audit *repository.AuditRepo
}

func NewShiftHandler(repo *repository.ShiftRepo, audit *repository.AuditRepo) *ShiftHandler {
	return &ShiftHandler{repo: repo, audit: audit}
}

// GET /api/shifts?week=2026-06-22  (any date in the target week; defaults to this week)
func (h *ShiftHandler) List(w http.ResponseWriter, r *http.Request) {
	weekStart := time.Now()
	if v := r.URL.Query().Get("week"); v != "" {
		if parsed, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = parsed
		}
	}
	shifts, err := h.repo.ListForWeek(r.Context(), weekStart)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list shifts")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: shifts, Total: len(shifts)})
}

// POST /api/shifts — assign a staff member to a job slot
func (h *ShiftHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateShiftRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" || req.JobLabel == "" || req.DayOfWeek == "" || req.SlotHour == "" {
		writeError(w, http.StatusBadRequest, "user_id, job_label, day_of_week, and slot_hour are required")
		return
	}
	user := auth.GetUser(r.Context())
	shift, err := h.repo.Create(r.Context(), req, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create shift")
		return
	}
	_ = h.audit.Record(r.Context(), user.ID, "shift_assigned", "Schedule",
		"Assigned shift: "+req.JobLabel+" — "+req.DayOfWeek+" "+req.SlotHour, r.RemoteAddr)
	writeJSON(w, http.StatusCreated, model.Response{Data: shift})
}

// PATCH /api/shifts/{id}/move — drag-and-drop reschedule
func (h *ShiftHandler) Move(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req model.MoveShiftRequest
	if err := decode(r, &req); err != nil || req.DayOfWeek == "" || req.SlotHour == "" {
		writeError(w, http.StatusBadRequest, "day_of_week and slot_hour are required")
		return
	}
	if err := h.repo.Move(r.Context(), id, req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to move shift")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "shift moved"})
}

// DELETE /api/shifts/{id}
func (h *ShiftHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.repo.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete shift")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Message: "shift removed"})
}

// ════════════════════════════════════════════════════════════════════════════
// Audit log (read-only API — entries are written internally by other handlers)
// ════════════════════════════════════════════════════════════════════════════

type AuditHandler struct {
	repo *repository.AuditRepo
}

func NewAuditHandler(repo *repository.AuditRepo) *AuditHandler {
	return &AuditHandler{repo: repo}
}

// GET /api/audit-log?module=&search=&limit=200 — admin only
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	module := r.URL.Query().Get("module")
	search := r.URL.Query().Get("search")
	limit := 200
	entries, err := h.repo.List(r.Context(), module, search, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list audit log")
		return
	}
	writeJSON(w, http.StatusOK, model.Response{Data: entries, Total: len(entries)})
}

// ════════════════════════════════════════════════════════════════════════════
// E-Signatures — board only (admin + manager); never exposed to staff
// ════════════════════════════════════════════════════════════════════════════

type ESignHandler struct {
	repo  *repository.ESignRepo
	audit *repository.AuditRepo
}

func NewESignHandler(repo *repository.ESignRepo, audit *repository.AuditRepo) *ESignHandler {
	return &ESignHandler{repo: repo, audit: audit}
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
	user := auth.GetUser(r.Context())
	doc, err := h.repo.Create(r.Context(), req, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create document")
		return
	}
	writeJSON(w, http.StatusCreated, model.Response{Data: doc})
}

// POST /api/esign/signers/{signerId}/sign
// Body: { "signature_data": "data:image/png;base64,..." } from the drawn canvas.
// Flips the parent document to "complete" once every signer has signed.
func (h *ESignHandler) Sign(w http.ResponseWriter, r *http.Request) {
	signerID := chi.URLParam(r, "signerId")
	var req model.SignDocumentRequest
	if err := decode(r, &req); err != nil || req.SignatureData == "" {
		writeError(w, http.StatusBadRequest, "signature_data is required")
		return
	}
	if err := h.repo.Sign(r.Context(), signerID, req.SignatureData); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record signature")
		return
	}
	user := auth.GetUser(r.Context())
	_ = h.audit.Record(r.Context(), user.ID, "document_signed", "E-Signatures",
		"Signed document via signer "+signerID, r.RemoteAddr)
	writeJSON(w, http.StatusOK, model.Response{Message: "signature recorded"})
}
