-- ============================================================
-- SCP-STK Hub — Full Database Schema
-- Run against your Supabase project via the SQL editor
-- or supabase db push
-- ============================================================

-- ── Extensions ──────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ── Enums ───────────────────────────────────────────────────

CREATE TYPE user_role   AS ENUM ('admin', 'manager', 'staff');
CREATE TYPE office_type AS ENUM ('north', 'south');
CREATE TYPE service_type AS ENUM (
    'cleaning', 'landscaping', 'painting', 'moving', 'snow_removal'
);
CREATE TYPE job_stage AS ENUM (
    'job_scheduled', 'staff_assigned', 'arrived_at_site', 'completed', 'invoiced'
);
CREATE TYPE task_status  AS ENUM ('open', 'in_progress', 'done');
CREATE TYPE grant_stage  AS ENUM ('incoming', 'applied', 'in_progress', 'awarded');
CREATE TYPE vote_choice  AS ENUM ('yes', 'no', 'abstain');
CREATE TYPE finance_type AS ENUM ('revenue', 'expense', 'payout');

-- ── Users ────────────────────────────────────────────────────
-- Mirrors Supabase auth.users. id = auth.uid()

CREATE TABLE users (
    id           UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    full_name    TEXT NOT NULL,
    role         user_role   NOT NULL DEFAULT 'staff',
    office       office_type NOT NULL DEFAULT 'north',
    phone        TEXT,
    hourly_rate  NUMERIC(8,2) NOT NULL DEFAULT 22.50,
    active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Auto-populate from Supabase Auth on first login
CREATE OR REPLACE FUNCTION handle_new_user()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO users (id, full_name)
    VALUES (NEW.id, COALESCE(NEW.raw_user_meta_data->>'full_name', NEW.email))
    ON CONFLICT (id) DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

CREATE TRIGGER on_auth_user_created
    AFTER INSERT ON auth.users
    FOR EACH ROW EXECUTE PROCEDURE handle_new_user();

-- ── Programs ─────────────────────────────────────────────────

CREATE TABLE programs (
    id     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name   TEXT NOT NULL,
    sub    TEXT NOT NULL DEFAULT '',
    office TEXT NOT NULL DEFAULT 'both', -- north | south | both
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed the 8 SCP-STK programs
INSERT INTO programs (name, sub, office) VALUES
    ('I Am 1 / Me2',          'Lived-Experience Street Outreach',       'both'),
    ('LELN',                   'Lived Experience Legal Navigation',       'north'),
    ('FSURP + Stabilize 72',   'Relational Infrastructure',              'both'),
    ('AVP + Trauma Healing',   'Healing & Safety',                       'south'),
    ('Pay It Forward',         'Vocational Pathway (6–12 months)',        'both'),
    ('Discovery Bay',          'Ecofarm Respite',                        'north'),
    ('Lynn Dancer Squad',      'Personal Assistant Program',             'both'),
    ('Family Pathways / WYFF', 'Family Pathways Diversion Stability',    'south');

-- ── Documents ────────────────────────────────────────────────

CREATE TABLE documents (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    program_id  UUID NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    url         TEXT NOT NULL,
    file_type   TEXT NOT NULL DEFAULT 'pdf',
    uploaded_by UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── CRM Clients (service customers) ──────────────────────────

CREATE TABLE crm_clients (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    full_name  TEXT NOT NULL,
    phone      TEXT NOT NULL,
    email      TEXT,
    address    TEXT NOT NULL,
    notes      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_crm_clients_name  ON crm_clients (full_name);
CREATE INDEX idx_crm_clients_phone ON crm_clients (phone);

-- ── CRM Jobs (the sales/job funnel) ──────────────────────────

CREATE TABLE crm_jobs (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    client_id    UUID NOT NULL REFERENCES crm_clients(id) ON DELETE CASCADE,
    service_type service_type NOT NULL,
    stage        job_stage    NOT NULL DEFAULT 'job_scheduled',
    address      TEXT NOT NULL,          -- may differ from client address
    description  TEXT NOT NULL DEFAULT '',
    tools_used   TEXT[] NOT NULL DEFAULT '{}',
    scheduled_at TIMESTAMPTZ,
    arrived_at   TIMESTAMPTZ,            -- set automatically on stage → arrived_at_site
    completed_at TIMESTAMPTZ,            -- set automatically on stage → completed
    price        NUMERIC(10,2) NOT NULL DEFAULT 0,
    notes        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_crm_jobs_stage       ON crm_jobs (stage);
CREATE INDEX idx_crm_jobs_client      ON crm_jobs (client_id);
CREATE INDEX idx_crm_jobs_scheduled   ON crm_jobs (scheduled_at);
CREATE INDEX idx_crm_jobs_service     ON crm_jobs (service_type);

-- ── Job Assignments (staff → job, many-to-many) ───────────────
-- A staff member can be assigned to multiple jobs simultaneously.
-- UNIQUE constraint prevents duplicate assignments but allows re-assignment with role change.

CREATE TABLE crm_job_assignments (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id      UUID NOT NULL REFERENCES crm_jobs(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_on_job TEXT NOT NULL DEFAULT 'support', -- lead | support
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (job_id, user_id)
);

CREATE INDEX idx_assignments_job  ON crm_job_assignments (job_id);
CREATE INDEX idx_assignments_user ON crm_job_assignments (user_id);

-- ── Tasks ─────────────────────────────────────────────────────
-- Tasks can be standalone or linked to a specific job.

CREATE TABLE tasks (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id      UUID REFERENCES crm_jobs(id) ON DELETE SET NULL, -- nullable
    assigned_to UUID NOT NULL REFERENCES users(id),
    title       TEXT NOT NULL,
    description TEXT,
    status      task_status NOT NULL DEFAULT 'open',
    due_at      TIMESTAMPTZ,
    created_by  UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tasks_assigned ON tasks (assigned_to);
CREATE INDEX idx_tasks_job      ON tasks (job_id);
CREATE INDEX idx_tasks_status   ON tasks (status);

-- ── Hours Log ─────────────────────────────────────────────────

CREATE TABLE hours_log (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_id     UUID REFERENCES crm_jobs(id) ON DELETE SET NULL,
    hours      NUMERIC(5,2) NOT NULL CHECK (hours > 0),
    date       DATE NOT NULL,
    notes      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_hours_user ON hours_log (user_id);
CREATE INDEX idx_hours_date ON hours_log (date);

-- ── Payroll Adjustments ───────────────────────────────────────

CREATE TABLE payroll_adjustments (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period     TEXT NOT NULL, -- YYYY-MM
    adjustment NUMERIC(10,2) NOT NULL DEFAULT 0,
    reason     TEXT,
    approved   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, period)
);

-- ── Finance Entries ───────────────────────────────────────────

CREATE TABLE finance_entries (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type        finance_type NOT NULL,
    amount      NUMERIC(12,2) NOT NULL,
    description TEXT NOT NULL,
    date        DATE NOT NULL,
    job_id      UUID REFERENCES crm_jobs(id) ON DELETE SET NULL,
    created_by  UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_finance_date ON finance_entries (date);
CREATE INDEX idx_finance_type ON finance_entries (type);

-- ── Grants ────────────────────────────────────────────────────

CREATE TABLE grants (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title       TEXT NOT NULL,
    funder      TEXT NOT NULL,
    amount      NUMERIC(12,2) NOT NULL DEFAULT 0,
    stage       grant_stage NOT NULL DEFAULT 'incoming',
    deadline    TIMESTAMPTZ,
    assigned_to UUID REFERENCES users(id) ON DELETE SET NULL,
    office_tag  TEXT NOT NULL DEFAULT 'both',
    notes       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_grants_stage    ON grants (stage);
CREATE INDEX idx_grants_deadline ON grants (deadline);

-- ── Resolutions + Votes ───────────────────────────────────────

CREATE TABLE resolutions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title       TEXT NOT NULL,
    body        TEXT NOT NULL,
    proposed_by UUID NOT NULL REFERENCES users(id),
    opens_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closes_at   TIMESTAMPTZ NOT NULL,
    status      TEXT NOT NULL DEFAULT 'open', -- open | closed | passed | failed
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE votes (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    resolution_id UUID NOT NULL REFERENCES resolutions(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    choice        vote_choice NOT NULL,
    voted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (resolution_id, user_id)
);

-- Auto-close resolutions past their closes_at date
-- (Run via a Supabase cron job or pg_cron)
-- SELECT cron.schedule('close-votes', '*/15 * * * *', $$
--     UPDATE resolutions
--     SET status = CASE WHEN yes_votes > no_votes THEN 'passed' ELSE 'failed' END
--     FROM (
--         SELECT resolution_id,
--             COUNT(*) FILTER (WHERE choice='yes') AS yes_votes,
--             COUNT(*) FILTER (WHERE choice='no')  AS no_votes
--         FROM votes GROUP BY resolution_id
--     ) v
--     WHERE resolutions.id = v.resolution_id
--       AND resolutions.status = 'open'
--       AND closes_at < NOW()
-- $$);

-- ── Notifications ─────────────────────────────────────────────

CREATE TABLE notifications (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       TEXT NOT NULL,   -- task_assigned | vote_opened | job_stage | mou_expiring | hours_milestone
    body       TEXT NOT NULL,
    ref_id     UUID,            -- linked job/task/grant/resolution ID
    read       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notif_user ON notifications (user_id, read);

-- ── Row Level Security (RLS) ──────────────────────────────────
-- Enable RLS on all tables. The Go backend uses the service role key
-- which bypasses RLS, but direct Supabase client calls from the frontend
-- are restricted by these policies.

ALTER TABLE users              ENABLE ROW LEVEL SECURITY;
ALTER TABLE programs           ENABLE ROW LEVEL SECURITY;
ALTER TABLE documents          ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_clients        ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_jobs           ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_job_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE tasks              ENABLE ROW LEVEL SECURITY;
ALTER TABLE hours_log          ENABLE ROW LEVEL SECURITY;
ALTER TABLE grants             ENABLE ROW LEVEL SECURITY;
ALTER TABLE resolutions        ENABLE ROW LEVEL SECURITY;
ALTER TABLE votes              ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications      ENABLE ROW LEVEL SECURITY;

-- Users can read all users (for assignment dropdowns)
CREATE POLICY "users_select_all" ON users
    FOR SELECT TO authenticated USING (TRUE);

-- Users can update only their own profile
CREATE POLICY "users_update_own" ON users
    FOR UPDATE TO authenticated USING (auth.uid() = id);

-- All authenticated users can read programs, clients, jobs, grants, resolutions
CREATE POLICY "programs_select"     ON programs     FOR SELECT TO authenticated USING (TRUE);
CREATE POLICY "crm_clients_select"  ON crm_clients  FOR SELECT TO authenticated USING (TRUE);
CREATE POLICY "crm_jobs_select"     ON crm_jobs     FOR SELECT TO authenticated USING (TRUE);
CREATE POLICY "grants_select"       ON grants       FOR SELECT TO authenticated USING (TRUE);
CREATE POLICY "resolutions_select"  ON resolutions  FOR SELECT TO authenticated USING (TRUE);
CREATE POLICY "docs_select"         ON documents    FOR SELECT TO authenticated USING (TRUE);

-- Tasks: staff see their own, managers/admins see all
CREATE POLICY "tasks_select" ON tasks
    FOR SELECT TO authenticated
    USING (
        assigned_to = auth.uid()
        OR EXISTS (SELECT 1 FROM users WHERE id = auth.uid() AND role IN ('admin','manager'))
    );

-- Hours: staff see own, managers/admins see all
CREATE POLICY "hours_select" ON hours_log
    FOR SELECT TO authenticated
    USING (
        user_id = auth.uid()
        OR EXISTS (SELECT 1 FROM users WHERE id = auth.uid() AND role IN ('admin','manager'))
    );

-- Notifications: users see only their own
CREATE POLICY "notif_select" ON notifications
    FOR SELECT TO authenticated USING (user_id = auth.uid());

-- Votes: users see only their own vote
CREATE POLICY "votes_select" ON votes
    FOR SELECT TO authenticated USING (user_id = auth.uid());
