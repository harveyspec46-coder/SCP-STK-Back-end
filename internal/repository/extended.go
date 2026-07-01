package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scp-stk/hub/internal/model"
)

// ════════════════════════════════════════════════════════════════════════════
// Admin allowlist
// ════════════════════════════════════════════════════════════════════════════

type AdminRepo struct {
	db *pgxpool.Pool
}

func NewAdminRepo(db *pgxpool.Pool) *AdminRepo {
	return &AdminRepo{db: db}
}

// ListAllowlist returns every pre-approved admin email. Shown in the
// Users & IDs panel so existing admins can see who will be auto-recognized.
func (r *AdminRepo) ListAllowlist(ctx context.Context) ([]model.AdminAllowlistEntry, error) {
	rows, err := r.db.Query(ctx,
		`SELECT email, COALESCE(note,''), added_by, created_at FROM admin_allowlist ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list allowlist: %w", err)
	}
	defer rows.Close()
	var out []model.AdminAllowlistEntry
	for rows.Next() {
		var e model.AdminAllowlistEntry
		if err := rows.Scan(&e.Email, &e.Note, &e.AddedBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// AddToAllowlist registers a new email for automatic admin recognition.
// The role assignment itself happens via the Postgres trigger on signup —
// this just adds the email so the trigger will match it.
func (r *AdminRepo) AddToAllowlist(ctx context.Context, email, note, addedBy string) (*model.AdminAllowlistEntry, error) {
	query := `
		INSERT INTO admin_allowlist (email, note, added_by)
		VALUES (LOWER($1), $2, $3)
		ON CONFLICT (email) DO UPDATE SET note = EXCLUDED.note
		RETURNING email, COALESCE(note,''), added_by, created_at`
	var e model.AdminAllowlistEntry
	if err := r.db.QueryRow(ctx, query, email, note, addedBy).
		Scan(&e.Email, &e.Note, &e.AddedBy, &e.CreatedAt); err != nil {
		return nil, fmt.Errorf("add allowlist entry: %w", err)
	}
	return &e, nil
}

func (r *AdminRepo) RemoveFromAllowlist(ctx context.Context, email string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM admin_allowlist WHERE LOWER(email) = LOWER($1)`, email)
	return err
}

// ListUsers returns every account with role/display_id, used by the
// Users & IDs panel (replaces the frontend's local-only mock state).
func (r *AdminRepo) ListUsers(ctx context.Context) ([]model.User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT u.id, u.full_name, u.role, u.office, COALESCE(u.phone,''),
				COALESCE(u.display_id,''), u.active, u.created_at,
				COALESCE(a.email,'')
			 FROM users u
			 LEFT JOIN auth.users a ON a.id = u.id
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.FullName, &u.Role, &u.Office, &u.Phone,
			&u.DisplayID, &u.Active, &u.CreatedAt, &u.Email); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// AssignDisplayID is the manual fallback for managers/staff whose ID wasn't
// auto-assigned (only allowlisted admins get one automatically at signup).
func (r *AdminRepo) AssignDisplayID(ctx context.Context, userID, displayID string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET display_id = $1 WHERE id = $2`, displayID, userID)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
// Participants & Volunteers
// ════════════════════════════════════════════════════════════════════════════

type ParticipantRepo struct {
	db *pgxpool.Pool
}

func NewParticipantRepo(db *pgxpool.Pool) *ParticipantRepo {
	return &ParticipantRepo{db: db}
}

func (r *ParticipantRepo) List(ctx context.Context, typeFilter, search string) ([]model.Participant, error) {
	query := `
		SELECT p.id, p.display_id, p.type, p.full_name, COALESCE(p.phone,''),
		       p.program_id, COALESCE(pr.name,''), p.stage, p.assigned_staff,
		       COALESCE(p.housing_status,''), COALESCE(p.language,'English'),
		       p.intake_mode, COALESCE(p.notes,''), p.created_at
		FROM participants p
		LEFT JOIN programs pr ON pr.id = p.program_id
		WHERE ($1 = '' OR p.type = $1::participant_type)
		  AND ($2 = '' OR p.full_name ILIKE '%' || $2 || '%')
		ORDER BY p.created_at DESC`
	rows, err := r.db.Query(ctx, query, typeFilter, search)
	if err != nil {
		return nil, fmt.Errorf("list participants: %w", err)
	}
	defer rows.Close()
	var out []model.Participant
	for rows.Next() {
		var p model.Participant
		if err := rows.Scan(&p.ID, &p.DisplayID, &p.Type, &p.FullName, &p.Phone,
			&p.ProgramID, &p.ProgramName, &p.Stage, &p.AssignedStaff,
			&p.HousingStatus, &p.Language, &p.IntakeMode, &p.Notes, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *ParticipantRepo) Create(ctx context.Context, req model.CreateParticipantRequest) (*model.Participant, error) {
	id := uuid.New().String()
	pType := req.Type
	if pType == "" {
		pType = model.ParticipantTypeParticipant
	}
	mode := req.IntakeMode
	if mode == "" {
		mode = model.IntakeDigital
	}
	lang := req.Language
	if lang == "" {
		lang = "English"
	}
	query := `
		INSERT INTO participants
			(id, type, full_name, phone, program_id, stage, assigned_staff,
			 housing_status, language, intake_mode, notes)
		VALUES ($1,$2,$3,$4,$5,'Intake',$6,$7,$8,$9,$10)
		RETURNING id, display_id, type, full_name, COALESCE(phone,''), program_id,
		          stage, assigned_staff, COALESCE(housing_status,''),
		          COALESCE(language,'English'), intake_mode, COALESCE(notes,''), created_at`
	var p model.Participant
	err := r.db.QueryRow(ctx, query, id, pType, req.FullName, req.Phone, req.ProgramID,
		req.AssignedStaff, req.HousingStatus, lang, mode, req.Notes).
		Scan(&p.ID, &p.DisplayID, &p.Type, &p.FullName, &p.Phone, &p.ProgramID,
			&p.Stage, &p.AssignedStaff, &p.HousingStatus, &p.Language, &p.IntakeMode,
			&p.Notes, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create participant: %w", err)
	}
	return &p, nil
}

func (r *ParticipantRepo) AdvanceStage(ctx context.Context, id, stage string) error {
	_, err := r.db.Exec(ctx, `UPDATE participants SET stage = $1 WHERE id = $2`, stage, id)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
// MOUs & Contracts
// ════════════════════════════════════════════════════════════════════════════

type MOURepo struct {
	db *pgxpool.Pool
}

func NewMOURepo(db *pgxpool.Pool) *MOURepo {
	return &MOURepo{db: db}
}

// List also calls refresh_mou_status() first so expiring/expired badges are
// always accurate without needing a separate cron job wired up yet.
func (r *MOURepo) List(ctx context.Context) ([]model.MOU, error) {
	if _, err := r.db.Exec(ctx, `SELECT refresh_mou_status()`); err != nil {
		return nil, fmt.Errorf("refresh mou status: %w", err)
	}
	rows, err := r.db.Query(ctx,
		`SELECT id, partner, type, signed_on, expires_on, assigned_to,
		        COALESCE(value_scope,''), status, created_at
		 FROM mous ORDER BY expires_on ASC`)
	if err != nil {
		return nil, fmt.Errorf("list mous: %w", err)
	}
	defer rows.Close()
	var out []model.MOU
	for rows.Next() {
		var m model.MOU
		if err := rows.Scan(&m.ID, &m.Partner, &m.Type, &m.SignedOn, &m.ExpiresOn,
			&m.AssignedTo, &m.ValueScope, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *MOURepo) Create(ctx context.Context, req model.CreateMOURequest) (*model.MOU, error) {
	id := uuid.New().String()
	query := `
		INSERT INTO mous (id, partner, type, signed_on, expires_on, assigned_to, value_scope, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'active')
		RETURNING id, partner, type, signed_on, expires_on, assigned_to, COALESCE(value_scope,''), status, created_at`
	var m model.MOU
	err := r.db.QueryRow(ctx, query, id, req.Partner, req.Type, req.SignedOn,
		req.ExpiresOn, req.AssignedTo, req.ValueScope).
		Scan(&m.ID, &m.Partner, &m.Type, &m.SignedOn, &m.ExpiresOn,
			&m.AssignedTo, &m.ValueScope, &m.Status, &m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create mou: %w", err)
	}
	return &m, nil
}

func (r *MOURepo) Renew(ctx context.Context, id string, newExpiry time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE mous SET status = 'active', expires_on = $1 WHERE id = $2`, newExpiry, id)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
// Resources (documents + training videos)
// ════════════════════════════════════════════════════════════════════════════

type ResourceRepo struct {
	db *pgxpool.Pool
}

func NewResourceRepo(db *pgxpool.Pool) *ResourceRepo {
	return &ResourceRepo{db: db}
}

func (r *ResourceRepo) List(ctx context.Context, typeFilter string) ([]model.Resource, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, type, title, COALESCE(description,''), COALESCE(url,''),
		        audience, COALESCE(uploaded_by::text,''), created_at
		 FROM resources
		 WHERE ($1 = '' OR type = $1::resource_type)
		 ORDER BY created_at DESC`, typeFilter)
	if err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}
	defer rows.Close()
	var out []model.Resource
	for rows.Next() {
		var res model.Resource
		if err := rows.Scan(&res.ID, &res.Type, &res.Title, &res.Description,
			&res.URL, &res.Audience, &res.UploadedBy, &res.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, nil
}

func (r *ResourceRepo) Create(ctx context.Context, req model.CreateResourceRequest, uploadedBy string) (*model.Resource, error) {
	id := uuid.New().String()
	audience := req.Audience
	if audience == "" {
		audience = "all"
	}
	query := `
		INSERT INTO resources (id, type, title, description, url, audience, uploaded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, type, title, COALESCE(description,''), COALESCE(url,''), audience,
		          COALESCE(uploaded_by::text,''), created_at`
	var res model.Resource
	err := r.db.QueryRow(ctx, query, id, req.Type, req.Title, req.Description,
		req.URL, audience, uploadedBy).
		Scan(&res.ID, &res.Type, &res.Title, &res.Description, &res.URL,
			&res.Audience, &res.UploadedBy, &res.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}
	return &res, nil
}

// ════════════════════════════════════════════════════════════════════════════
// Workshops
// ════════════════════════════════════════════════════════════════════════════

type WorkshopRepo struct {
	db *pgxpool.Pool
}

func NewWorkshopRepo(db *pgxpool.Pool) *WorkshopRepo {
	return &WorkshopRepo{db: db}
}

func (r *WorkshopRepo) List(ctx context.Context, statusFilter string) ([]model.Workshop, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, title, COALESCE(facilitator,''), scheduled_at, COALESCE(platform,''),
		        COALESCE(meeting_link,''), COALESCE(description,''), audience,
		        COALESCE(recording_url,''), status, attendee_count, created_at
		 FROM workshops
		 WHERE ($1 = '' OR status = $1::workshop_status)
		 ORDER BY scheduled_at DESC NULLS LAST`, statusFilter)
	if err != nil {
		return nil, fmt.Errorf("list workshops: %w", err)
	}
	defer rows.Close()
	var out []model.Workshop
	for rows.Next() {
		var w model.Workshop
		if err := rows.Scan(&w.ID, &w.Title, &w.Facilitator, &w.ScheduledAt, &w.Platform,
			&w.MeetingLink, &w.Description, &w.Audience, &w.RecordingURL,
			&w.Status, &w.AttendeeCount, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

func (r *WorkshopRepo) Create(ctx context.Context, req model.CreateWorkshopRequest, createdBy string) (*model.Workshop, error) {
	id := uuid.New().String()
	audience := req.Audience
	if audience == "" {
		audience = "all"
	}
	query := `
		INSERT INTO workshops (id, title, facilitator, scheduled_at, platform, meeting_link,
		                        description, audience, status, attendee_count, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'upcoming',0,$9)
		RETURNING id, title, COALESCE(facilitator,''), scheduled_at, COALESCE(platform,''),
		          COALESCE(meeting_link,''), COALESCE(description,''), audience,
		          COALESCE(recording_url,''), status, attendee_count, created_at`
	var w model.Workshop
	err := r.db.QueryRow(ctx, query, id, req.Title, req.Facilitator, req.ScheduledAt,
		req.Platform, req.MeetingLink, req.Description, audience, createdBy).
		Scan(&w.ID, &w.Title, &w.Facilitator, &w.ScheduledAt, &w.Platform,
			&w.MeetingLink, &w.Description, &w.Audience, &w.RecordingURL,
			&w.Status, &w.AttendeeCount, &w.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create workshop: %w", err)
	}
	return &w, nil
}

func (r *WorkshopRepo) AddRecording(ctx context.Context, id, url string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE workshops SET recording_url = $1, status = 'completed' WHERE id = $2`, url, id)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
// Shift schedule
// ════════════════════════════════════════════════════════════════════════════

type ShiftRepo struct {
	db *pgxpool.Pool
}

func NewShiftRepo(db *pgxpool.Pool) *ShiftRepo {
	return &ShiftRepo{db: db}
}

// ListForWeek returns every shift for the week containing the given date
// (defaults to the current week if weekStart is zero).
func (r *ShiftRepo) ListForWeek(ctx context.Context, weekStart time.Time) ([]model.Shift, error) {
	query := `
		SELECT s.id, s.user_id, u.full_name, s.job_label, s.day_of_week, s.week_start, s.slot_hour, s.created_at
		FROM shifts s
		JOIN users u ON u.id = s.user_id
		WHERE s.week_start = date_trunc('week', $1::date)::date
		ORDER BY s.day_of_week, s.slot_hour`
	rows, err := r.db.Query(ctx, query, weekStart)
	if err != nil {
		return nil, fmt.Errorf("list shifts: %w", err)
	}
	defer rows.Close()
	var out []model.Shift
	for rows.Next() {
		var s model.Shift
		if err := rows.Scan(&s.ID, &s.UserID, &s.UserName, &s.JobLabel,
			&s.DayOfWeek, &s.WeekStart, &s.SlotHour, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *ShiftRepo) Create(ctx context.Context, req model.CreateShiftRequest, createdBy string) (*model.Shift, error) {
	id := uuid.New().String()
	query := `
		INSERT INTO shifts (id, user_id, job_label, day_of_week, slot_hour, created_by)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, user_id, job_label, day_of_week, week_start, slot_hour, created_at`
	var s model.Shift
	err := r.db.QueryRow(ctx, query, id, req.UserID, req.JobLabel, req.DayOfWeek, req.SlotHour, createdBy).
		Scan(&s.ID, &s.UserID, &s.JobLabel, &s.DayOfWeek, &s.WeekStart, &s.SlotHour, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create shift: %w", err)
	}
	return &s, nil
}

func (r *ShiftRepo) Move(ctx context.Context, id string, req model.MoveShiftRequest) error {
	_, err := r.db.Exec(ctx,
		`UPDATE shifts SET day_of_week = $1, slot_hour = $2 WHERE id = $3`,
		req.DayOfWeek, req.SlotHour, id)
	return err
}

func (r *ShiftRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM shifts WHERE id = $1`, id)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
// Audit log (append-only — no Update/Delete methods exist on purpose)
// ════════════════════════════════════════════════════════════════════════════

type AuditRepo struct {
	db *pgxpool.Pool
}

func NewAuditRepo(db *pgxpool.Pool) *AuditRepo {
	return &AuditRepo{db: db}
}

// Record writes one append-only entry. Call this from any handler whose
// action should be traceable for grant audits / board accountability.
func (r *AuditRepo) Record(ctx context.Context, actorID, action, module, detail, ip string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO audit_log (actor_id, action, module, detail, ip_address) VALUES ($1,$2,$3,$4,$5)`,
		actorID, action, module, detail, ip)
	return err
}

func (r *AuditRepo) List(ctx context.Context, moduleFilter, search string, limit int) ([]model.AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query := `
		SELECT a.id, a.actor_id, COALESCE(u.full_name,''), a.action, a.module, a.detail,
		       COALESCE(a.ip_address,''), a.created_at
		FROM audit_log a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE ($1 = '' OR a.module = $1)
		  AND ($2 = '' OR a.detail ILIKE '%' || $2 || '%' OR u.full_name ILIKE '%' || $2 || '%')
		ORDER BY a.created_at DESC
		LIMIT $3`
	rows, err := r.db.Query(ctx, query, moduleFilter, search, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit log: %w", err)
	}
	defer rows.Close()
	var out []model.AuditEntry
	for rows.Next() {
		var e model.AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorName, &e.Action, &e.Module,
			&e.Detail, &e.IPAddress, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// ════════════════════════════════════════════════════════════════════════════
// E-Signatures
// ════════════════════════════════════════════════════════════════════════════

type ESignRepo struct {
	db *pgxpool.Pool
}

func NewESignRepo(db *pgxpool.Pool) *ESignRepo {
	return &ESignRepo{db: db}
}

func (r *ESignRepo) List(ctx context.Context) ([]model.ESignDocument, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, name, type, pages, clauses, status, created_by, created_at
		 FROM esign_documents ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list esign documents: %w", err)
	}
	defer rows.Close()

	var docs []model.ESignDocument
	for rows.Next() {
		var d model.ESignDocument
		if err := rows.Scan(&d.ID, &d.Name, &d.Type, &d.Pages, &d.Clauses,
			&d.Status, &d.CreatedBy, &d.CreatedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}

	// Attach signers (one extra query per doc is fine at this volume; revisit with a
	// single JOIN + aggregation if the org ever has hundreds of pending documents).
	for i := range docs {
		signers, err := r.signersForDoc(ctx, docs[i].ID)
		if err != nil {
			return nil, err
		}
		docs[i].Signers = signers
	}
	return docs, nil
}

func (r *ESignRepo) signersForDoc(ctx context.Context, docID string) ([]model.ESignSigner, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, document_id, name, role, user_id, signed, signature_data, signed_at
		 FROM esign_signers WHERE document_id = $1 ORDER BY id`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ESignSigner
	for rows.Next() {
		var s model.ESignSigner
		if err := rows.Scan(&s.ID, &s.DocumentID, &s.Name, &s.Role, &s.UserID,
			&s.Signed, &s.SignatureData, &s.SignedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Create inserts the document plus one signer row per requested name.
// Runs inside a transaction so a partially-created document never appears.
func (r *ESignRepo) Create(ctx context.Context, req model.CreateESignDocumentRequest, createdBy string) (*model.ESignDocument, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	id := uuid.New().String()
	var d model.ESignDocument
	err = tx.QueryRow(ctx,
		`INSERT INTO esign_documents (id, name, type, pages, clauses, status, created_by)
		 VALUES ($1,$2,$3,2,$4,'pending',$5)
		 RETURNING id, name, type, pages, clauses, status, created_by, created_at`,
		id, req.Name, req.Type, req.Clauses, createdBy).
		Scan(&d.ID, &d.Name, &d.Type, &d.Pages, &d.Clauses, &d.Status, &d.CreatedBy, &d.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create esign document: %w", err)
	}

	signerNames := req.SignerNames
	if len(signerNames) == 0 {
		signerNames = []string{createdBy} // fall back to the creator as sole signer
	}
	for _, name := range signerNames {
		sid := uuid.New().String()
		if _, err := tx.Exec(ctx,
			`INSERT INTO esign_signers (id, document_id, name, role, signed)
			 VALUES ($1,$2,$3,'staff',false)`, sid, id, name); err != nil {
			return nil, fmt.Errorf("create signer %q: %w", name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	d.Signers, _ = r.signersForDoc(ctx, id)
	return &d, nil
}

// Sign marks one signer's row complete with their drawn signature, then
// flips the parent document to "complete" once every signer has signed.
func (r *ESignRepo) Sign(ctx context.Context, signerID, signatureData string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var docID string
	err = tx.QueryRow(ctx,
		`UPDATE esign_signers SET signed = true, signature_data = $1, signed_at = NOW()
		 WHERE id = $2 RETURNING document_id`, signatureData, signerID).Scan(&docID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("signer not found")
		}
		return fmt.Errorf("sign document: %w", err)
	}

	var remaining int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM esign_signers WHERE document_id = $1 AND signed = false`, docID).
		Scan(&remaining); err != nil {
		return err
	}
	if remaining == 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE esign_documents SET status = 'complete' WHERE id = $1`, docID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
