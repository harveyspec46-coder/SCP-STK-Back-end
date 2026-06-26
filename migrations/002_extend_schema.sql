-- ════════════════════════════════════════════════════════════════════════════
-- 002_extend_schema.sql
-- Adds: admin auto-recognition (allowlist), participants/volunteers,
-- extended grants (priority/description/link), MOUs, resources, workshops,
-- shift scheduling, the append-only audit log, and in-app e-signatures.
-- Run this AFTER 001_schema.sql, in the Supabase SQL Editor.
-- ════════════════════════════════════════════════════════════════════════════

-- ── Fix: service_type was created as a fixed enum in 001, but the product
-- decision was always free-text (autocomplete from history, no fixed list).
ALTER TABLE crm_jobs ALTER COLUMN service_type TYPE TEXT;
ALTER TABLE crm_jobs ADD COLUMN IF NOT EXISTS estimated_hours NUMERIC(5,2);

-- ════════════════════════════════════════════════════════════════════════════
-- 1. Admin allowlist — automatic admin recognition on signup
-- ════════════════════════════════════════════════════════════════════════════
-- Any email below is granted role='admin' + an auto-generated ADM-XXX
-- display_id the instant they sign up. Your existing seed admin account
-- keeps whatever role/ID it already has — this table is only consulted
-- for NEW signups, so nothing changes for accounts that already exist.

CREATE TABLE admin_allowlist (
    email      TEXT PRIMARY KEY,
    note       TEXT,
    added_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE admin_allowlist ENABLE ROW LEVEL SECURITY;
CREATE POLICY "allowlist_admin_only" ON admin_allowlist
    FOR ALL TO authenticated
    USING (EXISTS (SELECT 1 FROM users WHERE id = auth.uid() AND role = 'admin'));

-- Seed it with the other 3 board admins. EDIT THESE EMAILS to the exact
-- addresses Daniyal, Leila, and Reese will sign up with before running this.
INSERT INTO admin_allowlist (email, note) VALUES
    ('daniyal@scproject17.onmicrosoft.com',        'Board admin — auto-recognized on signup'),
    ('leila.culberson@scproject17.onmicrosoft.com','Board admin — auto-recognized on signup'),
    ('reese@scproject17.onmicrosoft.com',          'Board admin — auto-recognized on signup')
ON CONFLICT (email) DO NOTHING;

-- ── display_id: the human-facing badge (ADM-001 / MGR-003 / STF-007) ───────
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_id TEXT UNIQUE;

CREATE OR REPLACE FUNCTION next_display_id(p_role user_role) RETURNS TEXT AS $$
DECLARE
    prefix TEXT;
    next_n INT;
BEGIN
    prefix := CASE p_role
        WHEN 'admin' THEN 'ADM'
        WHEN 'manager' THEN 'MGR'
        ELSE 'STF'
    END;
    SELECT COALESCE(MAX(SUBSTRING(display_id FROM 5)::INT), 0) + 1
      INTO next_n
      FROM users
     WHERE display_id LIKE prefix || '-%';
    RETURN prefix || '-' || LPAD(next_n::TEXT, 3, '0');
END;
$$ LANGUAGE plpgsql;

-- ── Updated signup trigger: checks the allowlist, assigns role + ID ────────
-- Replaces the 001 version (which always defaulted everyone to 'staff').
CREATE OR REPLACE FUNCTION handle_new_user() RETURNS TRIGGER AS $$
DECLARE
    v_role       user_role;
    v_display_id TEXT;
    v_is_admin   BOOLEAN;
BEGIN
    SELECT EXISTS (
        SELECT 1 FROM admin_allowlist WHERE LOWER(email) = LOWER(NEW.email)
    ) INTO v_is_admin;

    IF v_is_admin THEN
        v_role := 'admin';
        v_display_id := next_display_id('admin');           -- auto-assigned immediately
    ELSIF LOWER(NEW.email) LIKE '%@scproject17.onmicrosoft.com' THEN
        v_role := 'manager';
        v_display_id := NULL;                                -- pending, assigned manually later
    ELSE
        v_role := 'staff';
        v_display_id := NULL;                                -- pending, assigned manually later
    END IF;

    INSERT INTO users (id, full_name, role, display_id)
    VALUES (NEW.id, COALESCE(NEW.raw_user_meta_data->>'full_name', NEW.email), v_role, v_display_id)
    ON CONFLICT (id) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
-- The trigger "on_auth_user_created" from 001 already points at this function
-- name, so CREATE OR REPLACE above updates its behavior in place — no need
-- to recreate the trigger itself.

-- Optional one-time backfill for your existing seed admin, if it doesn't
-- have a display_id yet. Edit the email, then uncomment and run once:
-- UPDATE users SET role = 'admin', display_id = 'ADM-001'
--   WHERE id = (SELECT id FROM auth.users WHERE email = 'admin@scproject17.onmicrosoft.com')
--     AND display_id IS NULL;

-- ════════════════════════════════════════════════════════════════════════════
-- 2. Participants & Volunteers (distinct from crm_clients, who are paying
--    workforce-services customers, not program participants)
-- ════════════════════════════════════════════════════════════════════════════

CREATE TYPE participant_type AS ENUM ('participant', 'volunteer');
CREATE TYPE intake_mode      AS ENUM ('digital', 'physical', 'link');

CREATE TABLE participants (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    display_id     TEXT UNIQUE,                 -- PAR-001 | VOL-001
    type           participant_type NOT NULL DEFAULT 'participant',
    full_name      TEXT NOT NULL,
    phone          TEXT,
    program_id     UUID REFERENCES programs(id) ON DELETE SET NULL,
    stage          TEXT NOT NULL DEFAULT 'Intake',
    assigned_staff UUID REFERENCES users(id) ON DELETE SET NULL,
    housing_status TEXT,
    language       TEXT DEFAULT 'English',
    intake_mode    intake_mode NOT NULL DEFAULT 'digital',
    notes          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION next_participant_display_id(p_type participant_type) RETURNS TEXT AS $$
DECLARE
    prefix TEXT := CASE p_type WHEN 'volunteer' THEN 'VOL' ELSE 'PAR' END;
    next_n INT;
BEGIN
    SELECT COALESCE(MAX(SUBSTRING(display_id FROM 5)::INT), 0) + 1
      INTO next_n FROM participants WHERE display_id LIKE prefix || '-%';
    RETURN prefix || '-' || LPAD(next_n::TEXT, 3, '0');
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION set_participant_display_id() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.display_id IS NULL THEN
        NEW.display_id := next_participant_display_id(NEW.type);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_participant_display_id
    BEFORE INSERT ON participants
    FOR EACH ROW EXECUTE PROCEDURE set_participant_display_id();

ALTER TABLE participants ENABLE ROW LEVEL SECURITY;
CREATE POLICY "participants_board_only" ON participants
    FOR SELECT TO authenticated
    USING (EXISTS (SELECT 1 FROM users WHERE id = auth.uid() AND role IN ('admin','manager')));

-- ════════════════════════════════════════════════════════════════════════════
-- 3. Grants — add priority / description / link
-- ════════════════════════════════════════════════════════════════════════════

CREATE TYPE grant_priority AS ENUM ('normal', 'high', 'urgent');
ALTER TABLE grants ADD COLUMN IF NOT EXISTS priority grant_priority NOT NULL DEFAULT 'normal';
ALTER TABLE grants ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE grants ADD COLUMN IF NOT EXISTS link TEXT;

-- ════════════════════════════════════════════════════════════════════════════
-- 4. MOUs & Contracts (with expiry alerts)
-- ════════════════════════════════════════════════════════════════════════════

CREATE TYPE mou_status AS ENUM ('active', 'expiring', 'expired');

CREATE TABLE mous (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    partner     TEXT NOT NULL,
    type        TEXT NOT NULL,
    signed_on   DATE,
    expires_on  DATE NOT NULL,
    assigned_to UUID REFERENCES users(id) ON DELETE SET NULL,
    value_scope TEXT,
    status      mou_status NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_mous_expires ON mous (expires_on);

-- Called on every read (MOURepo.List) so the active/expiring/expired badges
-- are always accurate without needing a separate cron job wired up yet.
CREATE OR REPLACE FUNCTION refresh_mou_status() RETURNS VOID AS $$
BEGIN
    UPDATE mous SET status = 'expired'
      WHERE expires_on < CURRENT_DATE AND status <> 'expired';
    UPDATE mous SET status = 'expiring'
      WHERE expires_on >= CURRENT_DATE
        AND expires_on <= CURRENT_DATE + INTERVAL '60 days'
        AND status = 'active';
END;
$$ LANGUAGE plpgsql;

ALTER TABLE mous ENABLE ROW LEVEL SECURITY;
CREATE POLICY "mous_board_only" ON mous
    FOR SELECT TO authenticated
    USING (EXISTS (SELECT 1 FROM users WHERE id = auth.uid() AND role IN ('admin','manager')));

-- ════════════════════════════════════════════════════════════════════════════
-- 5. Resources (documents + training videos)
-- ════════════════════════════════════════════════════════════════════════════

CREATE TYPE resource_type AS ENUM ('doc', 'video');
CREATE TYPE audience_type AS ENUM ('all', 'staff', 'board');

CREATE TABLE resources (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type        resource_type NOT NULL,
    title       TEXT NOT NULL,
    description TEXT,
    url         TEXT,
    audience    audience_type NOT NULL DEFAULT 'all',
    uploaded_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE resources ENABLE ROW LEVEL SECURITY;
CREATE POLICY "resources_select_all" ON resources FOR SELECT TO authenticated USING (TRUE);

-- ════════════════════════════════════════════════════════════════════════════
-- 6. Workshops (virtual sessions library)
-- ════════════════════════════════════════════════════════════════════════════

CREATE TYPE workshop_status AS ENUM ('upcoming', 'completed');

CREATE TABLE workshops (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title          TEXT NOT NULL,
    facilitator    TEXT,
    scheduled_at   TIMESTAMPTZ,
    platform       TEXT,
    meeting_link   TEXT,
    description    TEXT,
    audience       audience_type NOT NULL DEFAULT 'all',
    recording_url  TEXT,
    status         workshop_status NOT NULL DEFAULT 'upcoming',
    attendee_count INT NOT NULL DEFAULT 0,
    created_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE workshops ENABLE ROW LEVEL SECURITY;
CREATE POLICY "workshops_select_all" ON workshops FOR SELECT TO authenticated USING (TRUE);

-- ════════════════════════════════════════════════════════════════════════════
-- 7. Shift schedule
-- ════════════════════════════════════════════════════════════════════════════

CREATE TABLE shifts (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_label   TEXT NOT NULL,
    day_of_week TEXT NOT NULL,                              -- Mon..Sun
    week_start  DATE NOT NULL DEFAULT date_trunc('week', CURRENT_DATE)::DATE,
    slot_hour   TEXT NOT NULL,                              -- "9am" etc — matches the UI's hour labels
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_shifts_week ON shifts (week_start);
CREATE INDEX idx_shifts_user ON shifts (user_id);

ALTER TABLE shifts ENABLE ROW LEVEL SECURITY;
CREATE POLICY "shifts_board_only" ON shifts
    FOR SELECT TO authenticated
    USING (EXISTS (SELECT 1 FROM users WHERE id = auth.uid() AND role IN ('admin','manager')));

-- ════════════════════════════════════════════════════════════════════════════
-- 8. Audit log — append-only
-- ════════════════════════════════════════════════════════════════════════════

CREATE TABLE audit_log (
    id         BIGSERIAL PRIMARY KEY,
    actor_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    action     TEXT NOT NULL,
    module     TEXT NOT NULL,
    detail     TEXT NOT NULL,
    ip_address TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_module  ON audit_log (module);
CREATE INDEX idx_audit_created ON audit_log (created_at DESC);

-- Enforce append-only at the database level — even an admin cannot edit
-- or delete a row through any SQL client, only insert new ones.
CREATE OR REPLACE FUNCTION block_audit_mutation() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only — rows cannot be updated or deleted';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_no_update BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE PROCEDURE block_audit_mutation();
CREATE TRIGGER trg_audit_no_delete BEFORE DELETE ON audit_log
    FOR EACH ROW EXECUTE PROCEDURE block_audit_mutation();

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY "audit_log_admin_select" ON audit_log
    FOR SELECT TO authenticated
    USING (EXISTS (SELECT 1 FROM users WHERE id = auth.uid() AND role = 'admin'));
-- Inserts happen through the Go backend's service-role connection, which
-- bypasses RLS by design — no INSERT policy is needed for normal users.

-- ════════════════════════════════════════════════════════════════════════════
-- 9. E-Signatures — board only (admin + manager), never staff
-- ════════════════════════════════════════════════════════════════════════════

CREATE TYPE doc_type   AS ENUM ('participant', 'mou', 'staff', 'grant');
CREATE TYPE doc_status AS ENUM ('pending', 'complete');

CREATE TABLE esign_documents (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name       TEXT NOT NULL,
    type       doc_type NOT NULL,
    pages      INT NOT NULL DEFAULT 1,
    clauses    TEXT[] NOT NULL DEFAULT '{}',
    status     doc_status NOT NULL DEFAULT 'pending',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE esign_signers (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    document_id    UUID NOT NULL REFERENCES esign_documents(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT 'staff',  -- admin | manager | staff | participant | external
    user_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    signed         BOOLEAN NOT NULL DEFAULT FALSE,
    signature_data TEXT,                            -- base64 PNG data URL from the signing canvas
    signed_at      TIMESTAMPTZ
);
CREATE INDEX idx_esign_signers_doc ON esign_signers (document_id);

ALTER TABLE esign_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE esign_signers   ENABLE ROW LEVEL SECURITY;

CREATE POLICY "esign_docs_board_only" ON esign_documents
    FOR SELECT TO authenticated
    USING (EXISTS (SELECT 1 FROM users WHERE id = auth.uid() AND role IN ('admin','manager')));

CREATE POLICY "esign_signers_board_or_self" ON esign_signers
    FOR SELECT TO authenticated
    USING (
        user_id = auth.uid()
        OR EXISTS (SELECT 1 FROM users WHERE id = auth.uid() AND role IN ('admin','manager'))
    );

-- ════════════════════════════════════════════════════════════════════════════
-- Done. Next: deploy the updated Go backend — router.New now also takes the
-- db pool (see cmd/server/main.go) — then redeploy on Railway/Fly.io.
-- ════════════════════════════════════════════════════════════════════════════
