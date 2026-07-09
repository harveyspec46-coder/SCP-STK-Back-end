package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scp-stk/hub/internal/model"
)

// ─── Tasks ────────────────────────────────────────────────────────────────────

// ErrInvalidAssignee is returned when a task is assigned to someone who is
// not an admin or manager — board tasks are admin/manager only.
var ErrInvalidAssignee = errors.New("tasks can only be assigned to admins or managers")

type TaskRepo struct {
	db *pgxpool.Pool
}

func NewTaskRepo(db *pgxpool.Pool) *TaskRepo {
	return &TaskRepo{db: db}
}

func (r *TaskRepo) ListForUser(ctx context.Context, userID string) ([]model.Task, error) {
	query := `
		SELECT t.id, t.job_id, t.assigned_to, t.title, t.description,
		       t.status, t.priority, t.ready_for_review, t.due_at, t.created_by, t.created_at,
		       u.full_name, u.role
		FROM tasks t
		JOIN users u ON u.id = t.assigned_to
		WHERE t.assigned_to = $1
		ORDER BY
			CASE t.status WHEN 'open' THEN 1 WHEN 'in_progress' THEN 2 ELSE 3 END,
			t.due_at ASC NULLS LAST`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	return r.attachAttachments(ctx, tasks)
}

func (r *TaskRepo) ListAll(ctx context.Context) ([]model.Task, error) {
	query := `
		SELECT t.id, t.job_id, t.assigned_to, t.title, t.description,
		       t.status, t.priority, t.ready_for_review, t.due_at, t.created_by, t.created_at,
		       u.full_name, u.role
		FROM tasks t
		JOIN users u ON u.id = t.assigned_to
		ORDER BY t.due_at ASC NULLS LAST`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	return r.attachAttachments(ctx, tasks)
}

func scanTasks(rows interface {
	Next() bool
	Scan(...any) error
}) ([]model.Task, error) {
	var tasks []model.Task
	for rows.Next() {
		var t model.Task
		t.Assignee = &model.User{}
		if err := rows.Scan(&t.ID, &t.JobID, &t.AssignedTo, &t.Title,
			&t.Description, &t.Status, &t.Priority, &t.ReadyForReview, &t.DueAt, &t.CreatedBy, &t.CreatedAt,
			&t.Assignee.FullName, &t.Assignee.Role); err != nil {
			return nil, err
		}
		t.Assignee.ID = t.AssignedTo
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// attachAttachments batch-loads every file attached to the given tasks and
// fills in each task's Attachments field. One query regardless of task count.
func (r *TaskRepo) attachAttachments(ctx context.Context, tasks []model.Task) ([]model.Task, error) {
	if len(tasks) == 0 {
		return tasks, nil
	}
	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	byTask, err := r.AttachmentsForTasks(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].Attachments = byTask[tasks[i].ID]
	}
	return tasks, nil
}

// AttachmentsForTasks batch-loads files for a set of task ids, grouped by task.
func (r *TaskRepo) AttachmentsForTasks(ctx context.Context, taskIDs []string) (map[string][]model.TaskAttachment, error) {
	out := map[string][]model.TaskAttachment{}
	if len(taskIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, task_id, uploaded_by, file_url, file_name, created_at
		FROM task_attachments
		WHERE task_id = ANY($1)
		ORDER BY created_at ASC`, taskIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a model.TaskAttachment
		if err := rows.Scan(&a.ID, &a.TaskID, &a.UploadedBy, &a.FileURL, &a.FileName, &a.CreatedAt); err != nil {
			return nil, err
		}
		out[a.TaskID] = append(out[a.TaskID], a)
	}
	return out, nil
}

// AddAttachment records a file already uploaded to Storage (client uploads
// first, then posts the resulting URL here). Either the assigner or the
// assignee may add attachments — that check happens in the handler.
func (r *TaskRepo) AddAttachment(ctx context.Context, taskID, uploadedBy, fileURL, fileName string) (*model.TaskAttachment, error) {
	id := uuid.New().String()
	var a model.TaskAttachment
	err := r.db.QueryRow(ctx, `
		INSERT INTO task_attachments (id, task_id, uploaded_by, file_url, file_name)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, task_id, uploaded_by, file_url, file_name, created_at`,
		id, taskID, uploadedBy, fileURL, fileName).
		Scan(&a.ID, &a.TaskID, &a.UploadedBy, &a.FileURL, &a.FileName, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add attachment: %w", err)
	}
	return &a, nil
}

// GetByID fetches a single task — used by the handler to check who's allowed
// to move it to the next status before calling UpdateStatus.
func (r *TaskRepo) GetByID(ctx context.Context, id string) (*model.Task, error) {
	var t model.Task
	err := r.db.QueryRow(ctx, `
		SELECT id, job_id, assigned_to, title, description, status, priority,
		       ready_for_review, due_at, created_by, created_at
		FROM tasks WHERE id=$1`, id).
		Scan(&t.ID, &t.JobID, &t.AssignedTo, &t.Title, &t.Description, &t.Status,
			&t.Priority, &t.ReadyForReview, &t.DueAt, &t.CreatedBy, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TaskRepo) Create(ctx context.Context, req model.CreateTaskRequest, createdBy string) (*model.Task, error) {
	var assigneeRole string
	if err := r.db.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, req.AssignedTo).
		Scan(&assigneeRole); err != nil {
		return nil, fmt.Errorf("assignee not found: %w", err)
	}
	if assigneeRole != "admin" && assigneeRole != "manager" {
		return nil, ErrInvalidAssignee
	}
	priority := req.Priority
	if priority == "" {
		priority = "normal"
	}
	id := uuid.New().String()
	query := `
		INSERT INTO tasks (id, job_id, assigned_to, title, description, status, priority, due_at, created_by)
		VALUES ($1,$2,$3,$4,$5,'open',$6,$7,$8)
		RETURNING id, job_id, assigned_to, title, description, status, priority, ready_for_review, due_at, created_by, created_at`
	var t model.Task
	err := r.db.QueryRow(ctx, query, id, req.JobID, req.AssignedTo,
		req.Title, req.Description, priority, req.DueAt, createdBy).
		Scan(&t.ID, &t.JobID, &t.AssignedTo, &t.Title,
			&t.Description, &t.Status, &t.Priority, &t.ReadyForReview, &t.DueAt, &t.CreatedBy, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return &t, nil
}

func (r *TaskRepo) UpdateStatus(ctx context.Context, id string, status model.TaskStatus) error {
	_, err := r.db.Exec(ctx, `UPDATE tasks SET status=$1, ready_for_review=false WHERE id=$2`, status, id)
	return err
}

// SetReadyForReview flags a task as ready without changing its status —
// used when the assignee wants to notify the assigner without completing it.
func (r *TaskRepo) SetReadyForReview(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE tasks SET ready_for_review=true WHERE id=$1`, id)
	return err
}

func (r *TaskRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM tasks WHERE id=$1`, id)
	return err
}

func (r *TaskRepo) GetFullName(ctx context.Context, id string) (string, error) {
	var name string
	err := r.db.QueryRow(ctx, `SELECT full_name FROM users WHERE id=$1`, id).Scan(&name)
	return name, err
}
// ProductivityScore returns completed/total ratio for a user in a given period.
func (r *TaskRepo) ProductivityScore(ctx context.Context, userID, period string) (float64, int, int, error) {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE status='done')   AS completed,
			COUNT(*)                                  AS total
		FROM tasks
		WHERE assigned_to = $1
		  AND date_trunc('month', created_at) = date_trunc('month', $2::date)`
	var completed, total int
	if err := r.db.QueryRow(ctx, query, userID, period+"-01").Scan(&completed, &total); err != nil {
		return 0, 0, 0, err
	}
	score := 0.0
	if total > 0 {
		score = float64(completed) / float64(total) * 100
	}
	return score, completed, total, nil
}

// ─── Hours + Payroll ──────────────────────────────────────────────────────────

type FinanceRepo struct {
	db *pgxpool.Pool
}

func NewFinanceRepo(db *pgxpool.Pool) *FinanceRepo {
	return &FinanceRepo{db: db}
}

// CurrentPayPeriod returns the current custom pay cycle: the 29th of a month
// through the 28th of the next month (org policy). end is exclusive.
// CurrentMonthPeriod returns the current calendar month (1st through the
// last day, end exclusive) — org policy: monthly, not a custom cycle.
func CurrentMonthPeriod() (start, end time.Time, label string) {
	now := time.Now()
	start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end = start.AddDate(0, 1, 0)
	label = start.Format("January 2006")
	return start, end, label
}

// GetPayroll returns a staff member's payroll for the current month. Hours
// and rate are entered manually by the admin (SetPayrollManual) — there is
// no automatic calculation from jobs.
func (r *FinanceRepo) GetPayroll(ctx context.Context, userID string) (*model.PayrollEntry, error) {
	_, _, label := CurrentMonthPeriod()

	query := `
		SELECT
			u.id, u.full_name, u.role,
			COALESCE(pa.hours, 0) AS hours,
			COALESCE(pa.rate, u.hourly_rate, 22.50) AS rate,
			COALESCE(pa.adjustment, 0) AS adjustment,
			COALESCE(pa.paid, false) AS paid,
			pa.paid_amount, pa.paid_at
		FROM users u
		LEFT JOIN payroll_adjustments pa ON pa.user_id = u.id AND pa.period = $2
		WHERE u.id = $1`

	var p model.PayrollEntry
	var user model.User
	p.User = &user
	var paidAmount *float64
	err := r.db.QueryRow(ctx, query, userID, label).Scan(
		&user.ID, &user.FullName, &user.Role,
		&p.TotalHours, &p.HourlyRate, &p.Adjustment,
		&p.Paid, &paidAmount, &p.PaidAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get payroll: %w", err)
	}
	if paidAmount != nil {
		p.PaidAmount = *paidAmount
	}
	p.UserID = userID
	p.Period = label
	p.GrossPay = p.TotalHours * p.HourlyRate
	p.NetPay = p.GrossPay + p.Adjustment
	return &p, nil
}

// SetPayrollManual records the admin's manual hours/rate entry for a staff
// member for the current month.
func (r *FinanceRepo) SetPayrollManual(ctx context.Context, userID string, req model.SetPayrollManualRequest) error {
	_, _, label := CurrentMonthPeriod()
	query := `
		INSERT INTO payroll_adjustments (id, user_id, period, hours, rate, adjustment, reason)
		VALUES ($1,$2,$3,$4,$5,0,'')
		ON CONFLICT (user_id, period) DO UPDATE
		SET hours = EXCLUDED.hours, rate = EXCLUDED.rate`
	_, err := r.db.Exec(ctx, query, uuid.New().String(), userID, label, req.Hours, req.Rate)
	return err
}

// ListPayrollForAllStaff returns the current month's payroll for every
// staff-role user, for the Payroll tab.
func (r *FinanceRepo) ListPayrollForAllStaff(ctx context.Context) ([]model.PayrollEntry, error) {
	rows, err := r.db.Query(ctx, `SELECT id FROM users WHERE role = 'staff'`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()

	var entries []model.PayrollEntry
	for _, id := range ids {
		p, err := r.GetPayroll(ctx, id)
		if err != nil {
			continue
		}
		entries = append(entries, *p)
	}
	return entries, nil
}

func (r *FinanceRepo) AdjustPay(ctx context.Context, userID string, req model.AdjustPayRequest) error {
	_, _, label := CurrentMonthPeriod()
	query := `
		INSERT INTO payroll_adjustments (id, user_id, period, adjustment, reason)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (user_id, period) DO UPDATE
		SET adjustment = EXCLUDED.adjustment, reason = EXCLUDED.reason`
	_, err := r.db.Exec(ctx, query, uuid.New().String(), userID, label, req.Adjustment, req.Reason)
	return err
}

// LogPayment records that a staff member's current-month pay was actually
// paid out (cash, etc), once an admin confirms it.
func (r *FinanceRepo) LogPayment(ctx context.Context, userID string, amount float64) error {
	_, _, label := CurrentMonthPeriod()
	query := `
		INSERT INTO payroll_adjustments (id, user_id, period, adjustment, reason, paid, paid_amount, paid_at)
		VALUES ($1,$2,$3,0,'',true,$4,NOW())
		ON CONFLICT (user_id, period) DO UPDATE
		SET paid = true, paid_amount = EXCLUDED.paid_amount, paid_at = NOW()`
	_, err := r.db.Exec(ctx, query, uuid.New().String(), userID, label, amount)
	return err
}

// LogJobPayment records (or updates) the manually-entered payment amount +
// receipt for a completed/invoiced job.
func (r *FinanceRepo) LogJobPayment(ctx context.Context, jobID, enteredBy string, req model.LogJobPaymentRequest) (*model.JobPayment, error) {
	query := `
		INSERT INTO job_payments (id, job_id, amount, receipt_url, entered_by)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (job_id) DO UPDATE
		SET amount = EXCLUDED.amount, receipt_url = EXCLUDED.receipt_url,
		    entered_by = EXCLUDED.entered_by, entered_at = NOW()
		RETURNING id, job_id, amount, receipt_url, entered_by, entered_at`
	var p model.JobPayment
	err := r.db.QueryRow(ctx, query, uuid.New().String(), jobID, req.Amount, req.ReceiptURL, enteredBy).
		Scan(&p.ID, &p.JobID, &p.Amount, &p.ReceiptURL, &p.EnteredBy, &p.EnteredAt)
	if err != nil {
		return nil, fmt.Errorf("log job payment: %w", err)
	}
	return &p, nil
}

// ListJobRevenue returns completed/invoiced jobs with their logged payment
// (if any), for the Workforce Revenue Overview tab.
func (r *FinanceRepo) ListJobRevenue(ctx context.Context) ([]model.JobRevenueRow, error) {
	query := `
		SELECT j.id, j.service_type, j.address, c.full_name, j.stage, j.completed_at,
		       jp.amount, jp.receipt_url
		FROM crm_jobs j
		JOIN crm_clients c ON c.id = j.client_id
		LEFT JOIN job_payments jp ON jp.job_id = j.id
		WHERE j.stage IN ('completed', 'invoiced')
		ORDER BY j.completed_at DESC NULLS LAST`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.JobRevenueRow
	for rows.Next() {
		var row model.JobRevenueRow
		if err := rows.Scan(&row.JobID, &row.ServiceType, &row.Address, &row.ClientName,
			&row.Stage, &row.CompletedAt, &row.PaymentAmount, &row.ReceiptURL); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// ─── Finance ledger (other revenue / expenses, outside job work) ──────────────

func (r *FinanceRepo) ListLedger(ctx context.Context, start, end time.Time) ([]model.LedgerEntry, error) {
	query := `
		SELECT id, entry_type, description, amount, receipt_url, entry_date, entered_by, created_at
		FROM finance_ledger
		WHERE entry_date >= $1 AND entry_date < $2
		ORDER BY entry_date DESC`
	rows, err := r.db.Query(ctx, query, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.LedgerEntry
	for rows.Next() {
		var e model.LedgerEntry
		if err := rows.Scan(&e.ID, &e.EntryType, &e.Description, &e.Amount,
			&e.ReceiptURL, &e.EntryDate, &e.EnteredBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *FinanceRepo) CreateLedgerEntry(ctx context.Context, enteredBy string, req model.CreateLedgerEntryRequest) (*model.LedgerEntry, error) {
	entryDate, err := time.Parse("2006-01-02", req.EntryDate)
	if err != nil {
		entryDate = time.Now()
	}
	query := `
		INSERT INTO finance_ledger (id, entry_type, description, amount, receipt_url, entry_date, entered_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, entry_type, description, amount, receipt_url, entry_date, entered_by, created_at`
	var e model.LedgerEntry
	err = r.db.QueryRow(ctx, query, uuid.New().String(), req.EntryType, req.Description,
		req.Amount, req.ReceiptURL, entryDate, enteredBy).
		Scan(&e.ID, &e.EntryType, &e.Description, &e.Amount, &e.ReceiptURL, &e.EntryDate, &e.EnteredBy, &e.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create ledger entry: %w", err)
	}
	return &e, nil
}

func (r *FinanceRepo) DeleteLedgerEntry(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM finance_ledger WHERE id=$1`, id)
	return err
}

// GetSummary computes Overview figures for the current calendar month:
// invoiced revenue + pipeline value from manually-logged job payments,
// total jobs created, payroll due, and other ledger revenue/expenses.
func (r *FinanceRepo) GetSummary(ctx context.Context) (*model.FinanceSummary, error) {
	start, end, label := CurrentMonthPeriod()
	var s model.FinanceSummary
	s.Period = label

	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(jp.amount), 0)
		FROM crm_jobs j JOIN job_payments jp ON jp.job_id = j.id
		WHERE j.stage = 'invoiced' AND jp.entered_at >= $1 AND jp.entered_at < $2`,
		start, end).Scan(&s.InvoicedRevenue)
	if err != nil {
		return nil, fmt.Errorf("invoiced revenue: %w", err)
	}

	err = r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(jp.amount), 0)
		FROM crm_jobs j JOIN job_payments jp ON jp.job_id = j.id
		WHERE j.stage = 'completed' AND jp.entered_at >= $1 AND jp.entered_at < $2`,
		start, end).Scan(&s.PipelineValue)
	if err != nil {
		return nil, fmt.Errorf("pipeline value: %w", err)
	}

	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM crm_jobs WHERE created_at >= $1 AND created_at < $2`,
		start, end).Scan(&s.TotalJobs)
	if err != nil {
		return nil, fmt.Errorf("total jobs: %w", err)
	}

	payrolls, err := r.ListPayrollForAllStaff(ctx)
	if err == nil {
		for _, p := range payrolls {
			if !p.Paid {
				s.PayrollDue += p.NetPay
			}
		}
	}

	ledger, err := r.ListLedger(ctx, start, end)
	if err == nil {
		for _, e := range ledger {
			if e.EntryType == "revenue" {
				s.OtherRevenue += e.Amount
			} else {
				s.OtherExpenses += e.Amount
			}
		}
	}

	return &s, nil
}

// ─── Notifications ────────────────────────────────────────────────────────────

type NotificationRepo struct {
	db *pgxpool.Pool
}

func NewNotificationRepo(db *pgxpool.Pool) *NotificationRepo {
	return &NotificationRepo{db: db}
}

func (r *NotificationRepo) Create(ctx context.Context, userID, notifType, body string, refID *string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO notifications (id, user_id, type, body, ref_id) VALUES ($1,$2,$3,$4,$5)`,
		uuid.New().String(), userID, notifType, body, refID)
	return err
}

func (r *NotificationRepo) ListForUser(ctx context.Context, userID string) ([]model.Notification, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, type, body, ref_id, read, created_at
		 FROM notifications WHERE user_id=$1 ORDER BY created_at DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notifs []model.Notification
	for rows.Next() {
		var n model.Notification
		rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Body, &n.RefID, &n.Read, &n.CreatedAt)
		notifs = append(notifs, n)
	}
	return notifs, nil
}

func (r *NotificationRepo) MarkRead(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE notifications SET read=true WHERE id=$1`, id)
	return err
}

// ─── Grants ───────────────────────────────────────────────────────────────────

type GrantRepo struct {
	db *pgxpool.Pool
}

func NewGrantRepo(db *pgxpool.Pool) *GrantRepo {
	return &GrantRepo{db: db}
}

func (r *GrantRepo) List(ctx context.Context) ([]model.Grant, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, title, funder, amount, stage, priority, description, link,
		        deadline, assigned_to, office_tag, notes, created_at
		 FROM grants ORDER BY deadline ASC NULLS LAST, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var grants []model.Grant
	for rows.Next() {
		var g model.Grant
		rows.Scan(&g.ID, &g.Title, &g.Funder, &g.Amount, &g.Stage,
			&g.Priority, &g.Description, &g.Link,
			&g.Deadline, &g.AssignedTo, &g.OfficeTag, &g.Notes, &g.CreatedAt)
		grants = append(grants, g)
	}
	return grants, nil
}

func (r *GrantRepo) Create(ctx context.Context, g model.Grant) (*model.Grant, error) {
	g.ID = uuid.New().String()
	if g.Priority == "" {
		g.Priority = "normal"
	}
	query := `
		INSERT INTO grants (id, title, funder, amount, stage, priority, description, link,
		                     deadline, assigned_to, office_tag, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id, title, funder, amount, stage, priority, description, link,
		          deadline, assigned_to, office_tag, notes, created_at`
	err := r.db.QueryRow(ctx, query, g.ID, g.Title, g.Funder, g.Amount,
		g.Stage, g.Priority, g.Description, g.Link, g.Deadline, g.AssignedTo, g.OfficeTag, g.Notes).
		Scan(&g.ID, &g.Title, &g.Funder, &g.Amount, &g.Stage,
			&g.Priority, &g.Description, &g.Link,
			&g.Deadline, &g.AssignedTo, &g.OfficeTag, &g.Notes, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *GrantRepo) AdvanceStage(ctx context.Context, id string, stage model.GrantStage) error {
	_, err := r.db.Exec(ctx, `UPDATE grants SET stage=$1 WHERE id=$2`, stage, id)
	return err
}

func (r *GrantRepo) Assign(ctx context.Context, id, userID string) error {
	_, err := r.db.Exec(ctx, `UPDATE grants SET assigned_to=$1 WHERE id=$2`, userID, id)
	return err
}

// DueForDeadlineAlert returns grants in "incoming" or "in_progress" stage
// whose deadline is within the next 72 hours (including already-overdue
// ones that haven't been notified yet) and that haven't been alerted on yet.
func (r *GrantRepo) DueForDeadlineAlert(ctx context.Context) ([]model.Grant, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, title, funder, amount, stage, priority, description, link,
		       deadline, assigned_to, office_tag, notes, created_at
		FROM grants
		WHERE stage IN ('incoming', 'in_progress')
		  AND deadline_notified = false
		  AND deadline IS NOT NULL
		  AND deadline <= now() + interval '72 hours'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var grants []model.Grant
	for rows.Next() {
		var g model.Grant
		if err := rows.Scan(&g.ID, &g.Title, &g.Funder, &g.Amount, &g.Stage, &g.Priority,
			&g.Description, &g.Link, &g.Deadline, &g.AssignedTo, &g.OfficeTag, &g.Notes, &g.CreatedAt); err != nil {
			return nil, err
		}
		grants = append(grants, g)
	}
	return grants, nil
}

// MarkDeadlineNotified flags a grant so the 72-hour alert isn't sent again.
func (r *GrantRepo) MarkDeadlineNotified(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE grants SET deadline_notified=true WHERE id=$1`, id)
	return err
}

// AdminManagerUserIDs returns the IDs of every admin/manager, used as the
// audience for grant-deadline alerts.
func (r *GrantRepo) AdminManagerUserIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT id FROM users WHERE role IN ('admin', 'manager')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ─── Voting ───────────────────────────────────────────────────────────────────

type VotingRepo struct {
	db *pgxpool.Pool
}

func NewVotingRepo(db *pgxpool.Pool) *VotingRepo {
	return &VotingRepo{db: db}
}

func (r *VotingRepo) List(ctx context.Context) ([]model.Resolution, error) {
	query := `
		SELECT r.id, r.title, r.body, r.proposed_by, r.opens_at, r.closes_at, r.status, r.created_at,
		       COALESCE(r.document_url,''),
		       u.full_name,
		       COUNT(v.id) FILTER (WHERE v.choice='yes')     AS yes_count,
		       COUNT(v.id) FILTER (WHERE v.choice='no')      AS no_count,
		       COUNT(v.id) FILTER (WHERE v.choice='abstain') AS abstain_count
		FROM resolutions r
		JOIN users u ON u.id = r.proposed_by
		LEFT JOIN votes v ON v.resolution_id = r.id
		GROUP BY r.id, u.full_name
		ORDER BY r.opens_at DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []model.Resolution
	for rows.Next() {
		var resolution model.Resolution
		resolution.Proposer = &model.User{}
		rows.Scan(&resolution.ID, &resolution.Title, &resolution.Body,
			&resolution.ProposedBy, &resolution.OpensAt, &resolution.ClosesAt, &resolution.Status,
			&resolution.CreatedAt, &resolution.DocumentURL, &resolution.Proposer.FullName,
			&resolution.YesCount, &resolution.NoCount, &resolution.AbstainCount)
		resolution.Proposer.ID = resolution.ProposedBy
		res = append(res, resolution)
	}
	return res, nil
}

// VotersForResolution returns every board member (admin/manager) and whether
// they have voted on this resolution yet, with their display_id for tracking.
func (r *VotingRepo) VotersForResolution(ctx context.Context, resolutionID string) ([]model.VoterStatus, error) {
	query := `
		SELECT u.id, u.full_name, u.display_id,
		       v.id IS NOT NULL AS voted,
		       v.choice
		FROM users u
		LEFT JOIN votes v ON v.user_id = u.id AND v.resolution_id = $1
		WHERE u.role IN ('admin','manager')
		ORDER BY u.full_name`
	rows, err := r.db.Query(ctx, query, resolutionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.VoterStatus
	for rows.Next() {
		var v model.VoterStatus
		var choice *string
		if err := rows.Scan(&v.UserID, &v.FullName, &v.DisplayID, &v.Voted, &choice); err != nil {
			continue
		}
		v.Choice = choice
		out = append(out, v)
	}
	return out, nil
}

func (r *VotingRepo) Create(ctx context.Context, res model.Resolution) (*model.Resolution, error) {
	res.ID = uuid.New().String()
	query := `
		INSERT INTO resolutions (id, title, body, proposed_by, opens_at, closes_at, status, document_url)
		VALUES ($1,$2,$3,$4,$5,$6,'open',$7)
		RETURNING id, title, body, proposed_by, opens_at, closes_at, status, created_at, COALESCE(document_url,'')`
	err := r.db.QueryRow(ctx, query, res.ID, res.Title, res.Body,
		res.ProposedBy, res.OpensAt, res.ClosesAt, res.DocumentURL).
		Scan(&res.ID, &res.Title, &res.Body, &res.ProposedBy,
			&res.OpensAt, &res.ClosesAt, &res.Status, &res.CreatedAt, &res.DocumentURL)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (r *VotingRepo) CastVote(ctx context.Context, resolutionID, userID string, choice model.VoteChoice) error {
	// Check resolution is still open
	var status string
	var closesAt time.Time
	err := r.db.QueryRow(ctx,
		`SELECT status, closes_at FROM resolutions WHERE id=$1`, resolutionID).
		Scan(&status, &closesAt)
	if err != nil {
		return fmt.Errorf("resolution not found")
	}
	if status != "open" || time.Now().After(closesAt) {
		return fmt.Errorf("voting is closed for this resolution")
	}

	// Upsert vote (one per user per resolution)
	_, err = r.db.Exec(ctx,
		`INSERT INTO votes (id, resolution_id, user_id, choice)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (resolution_id, user_id) DO UPDATE SET choice=EXCLUDED.choice`,
		uuid.New().String(), resolutionID, userID, choice)
	return err
}
