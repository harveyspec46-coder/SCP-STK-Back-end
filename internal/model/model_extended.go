package model

import "time"

// ════════════════════════════════════════════════════════════════════════════
// This file extends model.go with everything added after the initial CRM/
// Tasks/Finance/Grants/Voting build: admin auto-recognition, participants
// (vs. paying CRM clients), MOUs, resources, workshops, shift scheduling,
// the append-only audit log, and in-app e-signatures.
// ════════════════════════════════════════════════════════════════════════════

// ─── Admin allowlist ────────────────────────────────────────────────────────
// Any email in this table is automatically granted the "admin" role (and an
// ADM-XXX display_id) the moment they sign up, via the handle_new_user()
// Postgres trigger. See migrations/002_extend_schema.sql.

type AdminAllowlistEntry struct {
	Email     string    `json:"email"`
	Note      string    `json:"note,omitempty"`
	AddedBy   *string   `json:"added_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type AddAllowlistRequest struct {
	Email string `json:"email"`
	Note  string `json:"note"`
}

// ─── Participants & Volunteers ────────────────────────────────────────────
// Distinct from crm_clients (paying workforce-services customers).
// Participants are people who have gone through SCP-STK programs;
// volunteers are people willing to help with org tasks and programs.

type ParticipantType string
type IntakeMode string

const (
	ParticipantTypeParticipant ParticipantType = "participant"
	ParticipantTypeVolunteer   ParticipantType = "volunteer"

	IntakeDigital  IntakeMode = "digital"
	IntakePhysical IntakeMode = "physical"
	IntakeLink     IntakeMode = "link"
)

type Participant struct {
	ID            string          `json:"id"`
	DisplayID     string          `json:"display_id"` // PAR-001 | VOL-001, auto-assigned on insert
	Type          ParticipantType `json:"type"`
	FullName      string          `json:"full_name"`
	Phone         string          `json:"phone,omitempty"`
	ProgramID     *string         `json:"program_id,omitempty"`
	ProgramName   string          `json:"program_name,omitempty"`
	Stage         string          `json:"stage"`
	AssignedStaff *string         `json:"assigned_staff,omitempty"`
	HousingStatus string          `json:"housing_status,omitempty"`
	Language      string          `json:"language,omitempty"`
	IntakeMode    IntakeMode      `json:"intake_mode"`
	Notes         string          `json:"notes,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type CreateParticipantRequest struct {
	Type          ParticipantType `json:"type"`
	FullName      string          `json:"full_name"`
	Phone         string          `json:"phone"`
	ProgramID     *string         `json:"program_id"`
	HousingStatus string          `json:"housing_status"`
	Language      string          `json:"language"`
	IntakeMode    IntakeMode      `json:"intake_mode"`
	AssignedStaff *string         `json:"assigned_staff"`
	Notes         string          `json:"notes"`
}

type AdvanceParticipantStageRequest struct {
	Stage string `json:"stage"`
}

// ─── MOUs & Contracts ──────────────────────────────────────────────────────

type MOU struct {
	ID         string     `json:"id"`
	Partner    string     `json:"partner"`
	Type       string     `json:"type"`
	SignedOn   *time.Time `json:"signed_on,omitempty"`
	ExpiresOn  time.Time  `json:"expires_on"`
	AssignedTo *string    `json:"assigned_to,omitempty"`
	ValueScope string     `json:"value_scope,omitempty"`
	Status     string     `json:"status"` // active | expiring | expired
	CreatedAt  time.Time  `json:"created_at"`
}

type CreateMOURequest struct {
	Partner    string     `json:"partner"`
	Type       string     `json:"type"`
	SignedOn   *time.Time `json:"signed_on"`
	ExpiresOn  time.Time  `json:"expires_on"`
	AssignedTo *string    `json:"assigned_to"`
	ValueScope string     `json:"value_scope"`
}

// ─── Resources (documents + training videos) ─────────────────────────────

type Resource struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // doc | video
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	URL         string    `json:"url,omitempty"`
	Audience    string    `json:"audience"` // all | staff | board
	UploadedBy  string    `json:"uploaded_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateResourceRequest struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Audience    string `json:"audience"`
}

// ─── Workshops (virtual sessions library) ─────────────────────────────────

type Workshop struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Facilitator   string     `json:"facilitator,omitempty"`
	ScheduledAt   *time.Time `json:"scheduled_at,omitempty"`
	Platform      string     `json:"platform,omitempty"`
	MeetingLink   string     `json:"meeting_link,omitempty"`
	Description   string     `json:"description,omitempty"`
	Audience      string     `json:"audience"`
	RecordingURL  string     `json:"recording_url,omitempty"`
	Status        string     `json:"status"` // upcoming | completed
	AttendeeCount int        `json:"attendee_count"`
	CreatedAt     time.Time  `json:"created_at"`
}

type CreateWorkshopRequest struct {
	Title       string     `json:"title"`
	Facilitator string     `json:"facilitator"`
	ScheduledAt *time.Time `json:"scheduled_at"`
	Platform    string     `json:"platform"`
	MeetingLink string     `json:"meeting_link"`
	Description string     `json:"description"`
	Audience    string     `json:"audience"`
}

type AddRecordingRequest struct {
	RecordingURL string `json:"recording_url"`
}

// ─── Shift schedule ────────────────────────────────────────────────────────

type Shift struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name,omitempty"`
	JobLabel  string    `json:"job_label"`
	DayOfWeek string    `json:"day_of_week"` // Mon..Sun
	WeekStart time.Time `json:"week_start"`
	SlotHour  string    `json:"slot_hour"` // "9am" etc — matches frontend hour labels
	CreatedAt time.Time `json:"created_at"`
}

type CreateShiftRequest struct {
	UserID    string `json:"user_id"`
	JobLabel  string `json:"job_label"`
	DayOfWeek string `json:"day_of_week"`
	SlotHour  string `json:"slot_hour"`
}

type MoveShiftRequest struct {
	DayOfWeek string `json:"day_of_week"`
	SlotHour  string `json:"slot_hour"`
}

// ─── Audit log (append-only) ───────────────────────────────────────────────

type AuditEntry struct {
	ID        int64     `json:"id"`
	ActorID   *string   `json:"actor_id,omitempty"`
	ActorName string    `json:"actor_name,omitempty"`
	Action    string    `json:"action"`
	Module    string    `json:"module"`
	Detail    string    `json:"detail"`
	IPAddress string    `json:"ip_address,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateAuditEntryRequest struct {
	Action string `json:"action"`
	Module string `json:"module"`
	Detail string `json:"detail"`
}

// ─── E-Signatures ───────────────────────────────────────────────────────────
// Visible to admin + manager only — never surfaced to staff (enforced both
// by RLS in migrations/002 and by the BoardOnly middleware on these routes).

type ESignDocument struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Type      string        `json:"type"` // participant | mou | staff | grant
	Pages     int           `json:"pages"`
	Clauses   []string      `json:"clauses"`
	Status    string        `json:"status"` // pending | complete
	CreatedBy string        `json:"created_by,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	Signers   []ESignSigner `json:"signers,omitempty"`
}

type ESignSigner struct {
	ID            string     `json:"id"`
	DocumentID    string     `json:"document_id"`
	Name          string     `json:"name"`
	Role          string     `json:"role"` // admin | manager | staff | participant | external
	UserID        *string    `json:"user_id,omitempty"`
	Signed        bool       `json:"signed"`
	SignatureData *string    `json:"signature_data,omitempty"` // base64 PNG data URL
	SignedAt      *time.Time `json:"signed_at,omitempty"`
}

type CreateESignDocumentRequest struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Clauses     []string `json:"clauses"`
	SignerNames []string `json:"signer_names"` // free-text names or "UID · Name"
}

type SignDocumentRequest struct {
	SignatureData string `json:"signature_data"`
}
