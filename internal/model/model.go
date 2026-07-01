package model

import (
	"time"
)

// ─── Enums ────────────────────────────────────────────────────────────────────

type UserRole string
type Office string
type ServiceType string
type JobStage string
type TaskStatus string
type GrantStage string
type VoteChoice string

const (
	RoleAdmin   UserRole = "admin"
	RoleManager UserRole = "manager"
	RoleStaff   UserRole = "staff"

	OfficeNorth Office = "north"
	OfficeSouth Office = "south"

	ServiceCleaning    ServiceType = "cleaning"
	ServiceLandscaping ServiceType = "landscaping"
	ServicePainting    ServiceType = "painting"
	ServiceMoving      ServiceType = "moving"
	ServiceSnow        ServiceType = "snow_removal"

	// Job funnel stages
	StageScheduled ServiceType = "job_scheduled" // reusing type for simplicity
	StageAssigned  JobStage    = "staff_assigned"
	StageArrived   JobStage    = "arrived_at_site"
	StageCompleted JobStage    = "completed"
	StageInvoiced  JobStage    = "invoiced"

	TaskOpen       TaskStatus = "open"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone       TaskStatus = "done"

	GrantApplied    GrantStage = "applied"
	GrantInProgress GrantStage = "in_progress"
	GrantIncoming   GrantStage = "incoming"
	GrantAwarded    GrantStage = "awarded"

	VoteYes     VoteChoice = "yes"
	VoteNo      VoteChoice = "no"
	VoteAbstain VoteChoice = "abstain"
)

// ─── User ─────────────────────────────────────────────────────────────────────

type User struct {
	ID       string   `json:"id"`
	FullName string   `json:"full_name"`
	Email    string   `json:"email"`
	Role     UserRole `json:"role"`
	Office   Office   `json:"office"`
	Phone    string   `json:"phone,omitempty"`
	// DisplayID is the human-facing badge shown across the app (e.g. ADM-001, MGR-003, STF-007).
	// Auto-assigned at signup for admin_allowlist matches; pending (empty) for manager/staff
	// until an Admin assigns one in the Users & IDs panel.
	DisplayID string    `json:"display_id,omitempty"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── CRM Client (service customer) ───────────────────────────────────────────

type Client struct {
	ID        string    `json:"id"`
	FullName  string    `json:"full_name"`
	Phone     string    `json:"phone"`
	Email     string    `json:"email,omitempty"`
	Address   string    `json:"address"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// Computed
	OpenJobs int `json:"open_jobs,omitempty"`
}

type CreateClientRequest struct {
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Address  string `json:"address"`
	Notes    string `json:"notes"`
}

// ─── CRM Job ──────────────────────────────────────────────────────────────────

type Job struct {
	ID          string     `json:"id"`
	ClientID    string     `json:"client_id"`
	Client      *Client    `json:"client,omitempty"`
	ServiceType string     `json:"service_type"`
	Stage       JobStage   `json:"stage"`
	Address     string     `json:"address"`
	Description string     `json:"description"`
	ToolsUsed   []string   `json:"tools_used"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	ArrivedAt   *time.Time `json:"arrived_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Price       float64    `json:"price"`
	Notes       string     `json:"notes,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	// Computed relations
	Assignments []JobAssignment `json:"assignments,omitempty"`
	Tasks       []Task          `json:"tasks,omitempty"`
}

type CreateJobRequest struct {
	ClientID    string     `json:"client_id"`
	ServiceType string     `json:"service_type"`
	Address     string     `json:"address"`
	Description string     `json:"description"`
	ToolsUsed   []string   `json:"tools_used"`
	ScheduledAt *time.Time `json:"scheduled_at"`
	Price       float64    `json:"price"`
	Notes       string     `json:"notes"`
}

type UpdateJobRequest struct {
	ServiceType string     `json:"service_type,omitempty"`
	Address     string     `json:"address,omitempty"`
	Description string     `json:"description,omitempty"`
	ToolsUsed   []string   `json:"tools_used,omitempty"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	Price       float64    `json:"price,omitempty"`
	Notes       string     `json:"notes,omitempty"`
}

type AdvanceStageRequest struct {
	Stage     JobStage   `json:"stage"`
	Timestamp *time.Time `json:"timestamp,omitempty"` // optional override
}

// ─── Job Assignment ───────────────────────────────────────────────────────────

type JobAssignment struct {
	ID         string    `json:"id"`
	JobID      string    `json:"job_id"`
	UserID     string    `json:"user_id"`
	User       *User     `json:"user,omitempty"`
	RoleOnJob  string    `json:"role_on_job"` // lead | support
	AssignedAt time.Time `json:"assigned_at"`
}

type AssignStaffRequest struct {
	UserID    string `json:"user_id"`
	RoleOnJob string `json:"role_on_job"` // lead | support
}

// ─── Task ─────────────────────────────────────────────────────────────────────

type Task struct {
	ID             string           `json:"id"`
	JobID          *string          `json:"job_id,omitempty"`
	AssignedTo     string           `json:"assigned_to"`
	Assignee       *User            `json:"assignee,omitempty"`
	Title          string           `json:"title"`
	Description    string           `json:"description,omitempty"`
	Status         TaskStatus       `json:"status"`
	Priority       string           `json:"priority"`
	ReadyForReview bool             `json:"ready_for_review"`
	Attachments    []TaskAttachment `json:"attachments,omitempty"`
	DueAt          *time.Time       `json:"due_at,omitempty"`
	CreatedBy      string           `json:"created_by"`
	CreatedAt      time.Time        `json:"created_at"`
}

type CreateTaskRequest struct {
	JobID       *string    `json:"job_id"`
	AssignedTo  string     `json:"assigned_to"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    string     `json:"priority"`
	DueAt       *time.Time `json:"due_at"`
}

// TaskAttachment is a single file attached to a task — either side (assigner
// or assignee) can add one or more, at any point in the task's life.
type TaskAttachment struct {
	ID         string    `json:"id"`
	TaskID     string    `json:"task_id"`
	UploadedBy string    `json:"uploaded_by"`
	FileURL    string    `json:"file_url"`
	FileName   string    `json:"file_name"`
	CreatedAt  time.Time `json:"created_at"`
}

// ─── Program ──────────────────────────────────────────────────────────────────

type Program struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Sub          string     `json:"sub"`
	Office       string     `json:"office"` // north | south | both
	Active       bool       `json:"active"`
	Participants int        `json:"participants,omitempty"`
	Documents    []Document `json:"documents,omitempty"`
}

type Document struct {
	ID         string    `json:"id"`
	ProgramID  string    `json:"program_id"`
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	FileType   string    `json:"file_type"`
	UploadedBy string    `json:"uploaded_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// ─── Grant ────────────────────────────────────────────────────────────────────

type Grant struct {
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	Funder string     `json:"funder"`
	Amount float64    `json:"amount"`
	Stage  GrantStage `json:"stage"`
	// Priority: "normal" | "high" | "urgent" — drives the colored strip on the kanban card.
	Priority    string     `json:"priority,omitempty"`
	Description string     `json:"description,omitempty"`
	Link        string     `json:"link,omitempty"`
	Deadline    *time.Time `json:"deadline,omitempty"`
	AssignedTo  *string    `json:"assigned_to,omitempty"`
	Assignee    *User      `json:"assignee,omitempty"`
	OfficeTag   string     `json:"office_tag"`
	Notes       string     `json:"notes,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ─── Hours + Finance ──────────────────────────────────────────────────────────

type HoursEntry struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	JobID     *string   `json:"job_id,omitempty"`
	Hours     float64   `json:"hours"`
	Date      time.Time `json:"date"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type LogHoursRequest struct {
	JobID *string   `json:"job_id"`
	Hours float64   `json:"hours"`
	Date  time.Time `json:"date"`
	Notes string    `json:"notes"`
}

type PayrollEntry struct {
	UserID     string  `json:"user_id"`
	User       *User   `json:"user,omitempty"`
	Period     string  `json:"period"` // e.g. "2026-04"
	TotalHours float64 `json:"total_hours"`
	HourlyRate float64 `json:"hourly_rate"`
	GrossPay   float64 `json:"gross_pay"`
	Adjustment float64 `json:"adjustment"`
	NetPay     float64 `json:"net_pay"`
	Approved   bool    `json:"approved"`
}

type AdjustPayRequest struct {
	Adjustment float64 `json:"adjustment"`
	Reason     string  `json:"reason"`
}

type FinanceSummary struct {
	Period        string  `json:"period"`
	TotalRevenue  float64 `json:"total_revenue"`
	TotalExpenses float64 `json:"total_expenses"`
	TotalPayouts  float64 `json:"total_payouts"`
	NetBalance    float64 `json:"net_balance"`
}

// ─── Voting ───────────────────────────────────────────────────────────────────

type Resolution struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Body         string      `json:"body"`
	ProposedBy   string      `json:"proposed_by"`
	Proposer     *User       `json:"proposer,omitempty"`
	OpensAt      time.Time   `json:"opens_at"`
	ClosesAt     time.Time   `json:"closes_at"`
	Status       string      `json:"status"` // open | closed | passed | failed
	DocumentURL  string      `json:"document_url,omitempty"`
	YesCount     int         `json:"yes_count"`
	NoCount      int         `json:"no_count"`
	AbstainCount int         `json:"abstain_count"`
	MyVote       *VoteChoice `json:"my_vote,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
}

type VoterStatus struct {
	UserID    string  `json:"user_id"`
	FullName  string  `json:"full_name"`
	DisplayID *string `json:"display_id"`
	Voted     bool    `json:"voted"`
	Choice    *string `json:"choice,omitempty"`
}

type CastVoteRequest struct {
	Choice VoteChoice `json:"choice"`
}

// ─── Notification ─────────────────────────────────────────────────────────────

type Notification struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Type      string    `json:"type"` // task_assigned | vote_opened | job_stage | mou_expiring | hours_milestone
	Body      string    `json:"body"`
	RefID     *string   `json:"ref_id,omitempty"` // linked job/task/grant ID
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── API response wrapper ─────────────────────────────────────────────────────

type Response struct {
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
	Total   int         `json:"total,omitempty"`
}
