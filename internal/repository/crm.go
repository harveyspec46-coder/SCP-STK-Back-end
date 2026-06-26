package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scp-stk/hub/internal/model"
)

type CRMRepo struct {
	db *pgxpool.Pool
}

func NewCRMRepo(db *pgxpool.Pool) *CRMRepo {
	return &CRMRepo{db: db}
}

// ─── Clients ──────────────────────────────────────────────────────────────────

func (r *CRMRepo) ListClients(ctx context.Context, search string) ([]model.Client, error) {
	query := `
		SELECT c.id, c.full_name, c.phone, c.email, c.address, c.notes, c.created_at,
		       COUNT(j.id) FILTER (WHERE j.stage NOT IN ('completed','invoiced')) AS open_jobs
		FROM crm_clients c
		LEFT JOIN crm_jobs j ON j.client_id = c.id
		WHERE ($1 = '' OR c.full_name ILIKE '%' || $1 || '%' OR c.phone ILIKE '%' || $1 || '%')
		GROUP BY c.id
		ORDER BY c.created_at DESC`

	rows, err := r.db.Query(ctx, query, search)
	if err != nil {
		return nil, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()

	var clients []model.Client
	for rows.Next() {
		var c model.Client
		if err := rows.Scan(&c.ID, &c.FullName, &c.Phone, &c.Email,
			&c.Address, &c.Notes, &c.CreatedAt, &c.OpenJobs); err != nil {
			return nil, err
		}
		clients = append(clients, c)
	}
	return clients, nil
}

func (r *CRMRepo) GetClient(ctx context.Context, id string) (*model.Client, error) {
	query := `SELECT id, full_name, phone, email, address, notes, created_at FROM crm_clients WHERE id = $1`
	var c model.Client
	err := r.db.QueryRow(ctx, query, id).Scan(
		&c.ID, &c.FullName, &c.Phone, &c.Email, &c.Address, &c.Notes, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get client: %w", err)
	}
	return &c, nil
}

func (r *CRMRepo) CreateClient(ctx context.Context, req model.CreateClientRequest) (*model.Client, error) {
	id := uuid.New().String()
	query := `
		INSERT INTO crm_clients (id, full_name, phone, email, address, notes)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, full_name, phone, email, address, notes, created_at`
	var c model.Client
	err := r.db.QueryRow(ctx, query, id, req.FullName, req.Phone, req.Email, req.Address, req.Notes).
		Scan(&c.ID, &c.FullName, &c.Phone, &c.Email, &c.Address, &c.Notes, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	return &c, nil
}

func (r *CRMRepo) UpdateClient(ctx context.Context, id string, req model.CreateClientRequest) (*model.Client, error) {
	query := `
		UPDATE crm_clients SET full_name=$1, phone=$2, email=$3, address=$4, notes=$5
		WHERE id=$6
		RETURNING id, full_name, phone, email, address, notes, created_at`
	var c model.Client
	err := r.db.QueryRow(ctx, query, req.FullName, req.Phone, req.Email, req.Address, req.Notes, id).
		Scan(&c.ID, &c.FullName, &c.Phone, &c.Email, &c.Address, &c.Notes, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("update client: %w", err)
	}
	return &c, nil
}

func (r *CRMRepo) DeleteClient(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM crm_clients WHERE id=$1`, id)
	return err
}

// ─── Jobs ─────────────────────────────────────────────────────────────────────

func (r *CRMRepo) ListJobs(ctx context.Context, stage, serviceType, officeFilter string) ([]model.Job, error) {
	query := `
		SELECT j.id, j.client_id, j.service_type, j.stage, j.address, j.description,
		       j.tools_used, j.scheduled_at, j.arrived_at, j.completed_at, j.price, j.notes, j.created_at,
		       c.full_name, c.phone, c.address
		FROM crm_jobs j
		JOIN crm_clients c ON c.id = j.client_id
		WHERE ($1 = '' OR j.stage = $1)
		  AND ($2 = '' OR j.service_type = $2)
		ORDER BY j.scheduled_at ASC NULLS LAST, j.created_at DESC`

	rows, err := r.db.Query(ctx, query, stage, serviceType)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []model.Job
	for rows.Next() {
		var j model.Job
		j.Client = &model.Client{}
		if err := rows.Scan(
			&j.ID, &j.ClientID, &j.ServiceType, &j.Stage,
			&j.Address, &j.Description, &j.ToolsUsed,
			&j.ScheduledAt, &j.ArrivedAt, &j.CompletedAt,
			&j.Price, &j.Notes, &j.CreatedAt,
			&j.Client.FullName, &j.Client.Phone, &j.Client.Address,
		); err != nil {
			return nil, err
		}
		j.Client.ID = j.ClientID
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (r *CRMRepo) GetJob(ctx context.Context, id string) (*model.Job, error) {
	query := `
		SELECT j.id, j.client_id, j.service_type, j.stage, j.address, j.description,
		       j.tools_used, j.scheduled_at, j.arrived_at, j.completed_at, j.price, j.notes, j.created_at,
		       c.id, c.full_name, c.phone, c.email, c.address
		FROM crm_jobs j
		JOIN crm_clients c ON c.id = j.client_id
		WHERE j.id = $1`

	var j model.Job
	j.Client = &model.Client{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&j.ID, &j.ClientID, &j.ServiceType, &j.Stage,
		&j.Address, &j.Description, &j.ToolsUsed,
		&j.ScheduledAt, &j.ArrivedAt, &j.CompletedAt,
		&j.Price, &j.Notes, &j.CreatedAt,
		&j.Client.ID, &j.Client.FullName, &j.Client.Phone, &j.Client.Email, &j.Client.Address,
	)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}

	// Load assignments
	asgns, err := r.ListAssignments(ctx, id)
	if err == nil {
		j.Assignments = asgns
	}

	// Load tasks
	tasks, err := r.ListJobTasks(ctx, id)
	if err == nil {
		j.Tasks = tasks
	}

	return &j, nil
}

func (r *CRMRepo) CreateJob(ctx context.Context, req model.CreateJobRequest) (*model.Job, error) {
	id := uuid.New().String()
	query := `
		INSERT INTO crm_jobs (id, client_id, service_type, stage, address, description, tools_used, scheduled_at, price, notes)
		VALUES ($1,$2,$3,'job_scheduled',$4,$5,$6,$7,$8,$9)
		RETURNING id, client_id, service_type, stage, address, description, tools_used, scheduled_at, arrived_at, completed_at, price, notes, created_at`
	var j model.Job
	err := r.db.QueryRow(ctx, query,
		id, req.ClientID, req.ServiceType, req.Address, req.Description,
		req.ToolsUsed, req.ScheduledAt, req.Price, req.Notes,
	).Scan(&j.ID, &j.ClientID, &j.ServiceType, &j.Stage,
		&j.Address, &j.Description, &j.ToolsUsed,
		&j.ScheduledAt, &j.ArrivedAt, &j.CompletedAt,
		&j.Price, &j.Notes, &j.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	return &j, nil
}

func (r *CRMRepo) UpdateJob(ctx context.Context, id string, req model.UpdateJobRequest) (*model.Job, error) {
	query := `
		UPDATE crm_jobs SET
			service_type = COALESCE(NULLIF($1,''), service_type),
			address      = COALESCE(NULLIF($2,''), address),
			description  = COALESCE(NULLIF($3,''), description),
			tools_used   = CASE WHEN $4::text[] IS NOT NULL THEN $4 ELSE tools_used END,
			scheduled_at = COALESCE($5, scheduled_at),
			price        = CASE WHEN $6 > 0 THEN $6 ELSE price END,
			notes        = COALESCE(NULLIF($7,''), notes)
		WHERE id = $8
		RETURNING id, client_id, service_type, stage, address, description, tools_used, scheduled_at, arrived_at, completed_at, price, notes, created_at`
	var j model.Job
	err := r.db.QueryRow(ctx, query,
		req.ServiceType, req.Address, req.Description,
		req.ToolsUsed, req.ScheduledAt, req.Price, req.Notes, id,
	).Scan(&j.ID, &j.ClientID, &j.ServiceType, &j.Stage,
		&j.Address, &j.Description, &j.ToolsUsed,
		&j.ScheduledAt, &j.ArrivedAt, &j.CompletedAt,
		&j.Price, &j.Notes, &j.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("update job: %w", err)
	}
	return &j, nil
}

// AdvanceStage moves a job through the funnel and records timestamps.
func (r *CRMRepo) AdvanceStage(ctx context.Context, jobID string, req model.AdvanceStageRequest) (*model.Job, error) {
	// Set the relevant timestamp column based on new stage
	var query string
	switch req.Stage {
	case model.StageArrived:
		query = `UPDATE crm_jobs SET stage=$1, arrived_at=COALESCE($2, NOW()) WHERE id=$3`
	case model.StageCompleted:
		query = `UPDATE crm_jobs SET stage=$1, completed_at=COALESCE($2, NOW()) WHERE id=$3`
	default:
		query = `UPDATE crm_jobs SET stage=$1 WHERE id=$3`
	}

	if _, err := r.db.Exec(ctx, query, req.Stage, req.Timestamp, jobID); err != nil {
		return nil, fmt.Errorf("advance stage: %w", err)
	}
	return r.GetJob(ctx, jobID)
}

// ─── Assignments ──────────────────────────────────────────────────────────────

func (r *CRMRepo) ListAssignments(ctx context.Context, jobID string) ([]model.JobAssignment, error) {
	query := `
		SELECT a.id, a.job_id, a.user_id, a.role_on_job, a.assigned_at,
		       u.full_name, u.role, u.office, u.phone
		FROM crm_job_assignments a
		JOIN users u ON u.id = a.user_id
		WHERE a.job_id = $1
		ORDER BY a.assigned_at ASC`

	rows, err := r.db.Query(ctx, query, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []model.JobAssignment
	for rows.Next() {
		var a model.JobAssignment
		a.User = &model.User{}
		if err := rows.Scan(&a.ID, &a.JobID, &a.UserID, &a.RoleOnJob, &a.AssignedAt,
			&a.User.FullName, &a.User.Role, &a.User.Office, &a.User.Phone); err != nil {
			return nil, err
		}
		a.User.ID = a.UserID
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// AssignStaff adds a staff member to a job. A staff member can have multiple
// active job assignments simultaneously — this is by design for the workforce model.
func (r *CRMRepo) AssignStaff(ctx context.Context, jobID string, req model.AssignStaffRequest) (*model.JobAssignment, error) {
	id := uuid.New().String()
	query := `
		INSERT INTO crm_job_assignments (id, job_id, user_id, role_on_job)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (job_id, user_id) DO UPDATE SET role_on_job = EXCLUDED.role_on_job
		RETURNING id, job_id, user_id, role_on_job, assigned_at`
	var a model.JobAssignment
	err := r.db.QueryRow(ctx, query, id, jobID, req.UserID, req.RoleOnJob).
		Scan(&a.ID, &a.JobID, &a.UserID, &a.RoleOnJob, &a.AssignedAt)
	if err != nil {
		return nil, fmt.Errorf("assign staff: %w", err)
	}
	return &a, nil
}

func (r *CRMRepo) RemoveAssignment(ctx context.Context, jobID, userID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM crm_job_assignments WHERE job_id=$1 AND user_id=$2`, jobID, userID)
	return err
}

// StaffWorkload returns all active jobs for a given staff member.
// This supports the "assign staff while they have concurrent jobs" requirement.
func (r *CRMRepo) StaffWorkload(ctx context.Context, userID string) ([]model.Job, error) {
	query := `
		SELECT j.id, j.client_id, j.service_type, j.stage, j.address,
		       j.description, j.tools_used, j.scheduled_at, j.arrived_at,
		       j.completed_at, j.price, j.notes, j.created_at
		FROM crm_jobs j
		JOIN crm_job_assignments a ON a.job_id = j.id
		WHERE a.user_id = $1
		  AND j.stage NOT IN ('completed','invoiced')
		ORDER BY j.scheduled_at ASC NULLS LAST`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []model.Job
	for rows.Next() {
		var j model.Job
		if err := rows.Scan(&j.ID, &j.ClientID, &j.ServiceType, &j.Stage,
			&j.Address, &j.Description, &j.ToolsUsed,
			&j.ScheduledAt, &j.ArrivedAt, &j.CompletedAt,
			&j.Price, &j.Notes, &j.CreatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// ─── Job tasks ────────────────────────────────────────────────────────────────

func (r *CRMRepo) ListJobTasks(ctx context.Context, jobID string) ([]model.Task, error) {
	query := `
		SELECT t.id, t.job_id, t.assigned_to, t.title, t.description, t.status, t.due_at, t.created_by, t.created_at,
		       u.full_name
		FROM tasks t
		JOIN users u ON u.id = t.assigned_to
		WHERE t.job_id = $1
		ORDER BY t.created_at ASC`

	rows, err := r.db.Query(ctx, query, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []model.Task
	for rows.Next() {
		var t model.Task
		t.Assignee = &model.User{}
		if err := rows.Scan(&t.ID, &t.JobID, &t.AssignedTo,
			&t.Title, &t.Description, &t.Status,
			&t.DueAt, &t.CreatedBy, &t.CreatedAt,
			&t.Assignee.FullName); err != nil {
			return nil, err
		}
		t.Assignee.ID = t.AssignedTo
		tasks = append(tasks, t)
	}
	return tasks, nil
}
