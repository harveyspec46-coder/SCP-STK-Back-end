# SCP-STK Hub — Go Backend

Full-stack internal platform for Sawyer Culberson Project of Save the Kids.

## Stack

| Layer      | Technology                              |
|------------|-----------------------------------------|
| Backend    | Go 1.22 · chi router · pgx/v5           |
| Database   | PostgreSQL via Supabase                 |
| Auth       | Supabase Auth (JWT)                     |
| Frontend   | React 18 · Vite · TypeScript            |
| Deploy     | Docker · Railway / Fly.io               |

---

## Quick Start

### 1. Supabase setup

1. Create a new project at [supabase.com](https://supabase.com)
2. Go to **SQL Editor** and run `migrations/001_schema.sql` in full
3. Run `migrations/002_extend_schema.sql` next — **edit the 3 admin emails near
   the top first** (Daniyal, Leila, Reese). Anyone on that allowlist is
   automatically granted Admin + an ADM-XXX ID the moment they sign up. Your
   existing seed admin account is untouched — the allowlist only applies to
   new signups.
4. Go to **Settings → API** and copy:
   - `Project URL` → `SUPABASE_URL`
   - `anon public` key → `SUPABASE_ANON_KEY`
   - `service_role` key → `SUPABASE_SERVICE_KEY`
   - `JWT Secret` (Settings → Auth → JWT Secret) → `SUPABASE_JWT_SECRET`
5. Go to **Settings → Database → Connection string** (URI mode) → `DATABASE_URL`

### How admin recognition works

`admin_allowlist` is a one-column table of pre-approved emails. A Postgres
trigger (`handle_new_user`) fires every time someone signs up through
Supabase Auth: if their email is on the allowlist, they're inserted into
`users` with `role='admin'` and an auto-generated `display_id` (ADM-002,
ADM-003, …) — no manual step needed. Any other `@scproject17.onmicrosoft.com`
email becomes a Manager (ID assigned later via the Users & IDs panel); any
other domain becomes Staff. To add a 4th admin later, either re-run an
`INSERT INTO admin_allowlist` statement or use `POST /api/admin/allowlist`.

### 2. Environment

```bash
cp .env.example .env
# Fill in all values from step 1
```

### 3. Run locally

```bash
go mod tidy
go run ./cmd/server
# Server starts on :8080
```

### 4. Run with Docker

```bash
docker build -t scp-hub .
docker run --env-file .env -p 8080:8080 scp-hub
```

---

## API Reference

All endpoints require `Authorization: Bearer <supabase_jwt>` except `/health`.

### CRM — Job Funnel

| Method | Path | Description |
|--------|------|-------------|
| GET    | `/api/crm/jobs` | Kanban board grouped by stage |
| GET    | `/api/crm/jobs?stage=job_scheduled` | Filter by stage |
| GET    | `/api/crm/jobs?service_type=cleaning` | Filter by service |
| POST   | `/api/crm/jobs` | Create new job (starts at `job_scheduled`) |
| PATCH  | `/api/crm/jobs/:id/stage` | Advance funnel stage |
| POST   | `/api/crm/jobs/:id/assign` | Assign staff (concurrent jobs allowed) |
| DELETE | `/api/crm/jobs/:id/assign/:uid` | Remove staff from job |
| POST   | `/api/crm/jobs/:id/tasks` | Add task to job |
| GET    | `/api/crm/staff/:uid/workload` | See all active jobs for a staff member |

### Funnel Stages

```
job_scheduled → staff_assigned → arrived_at_site → completed → invoiced
```

Timestamps are recorded automatically:
- `arrived_at` is set when stage advances to `arrived_at_site`
- `completed_at` is set when stage advances to `completed`

### Create Job (POST /api/crm/jobs)

```json
{
  "client_id":    "uuid",
  "service_type": "cleaning",
  "address":      "123 Main St, Seattle WA",
  "description":  "Deep clean 3-bedroom house, focus on kitchen and bathrooms",
  "tools_used":   ["mop", "vacuum", "steam cleaner", "degreaser"],
  "scheduled_at": "2026-04-15T09:00:00Z",
  "price":        150.00,
  "notes":        "Client has a dog — bring pet-safe products"
}
```

### Assign Staff (POST /api/crm/jobs/:id/assign)

```json
{
  "user_id":    "uuid",
  "role_on_job": "lead"
}
```

Staff can be assigned to multiple jobs simultaneously. The response includes
a workload message showing how many other active jobs they have.

### Advance Stage (PATCH /api/crm/jobs/:id/stage)

```json
{
  "stage": "staff_assigned"
}
```

Valid values: `job_scheduled`, `staff_assigned`, `arrived_at_site`, `completed`, `invoiced`

---

## User Roles

| Role    | CRM full | Delete | Finance edit | Manage users | Propose votes |
|---------|----------|--------|--------------|--------------|---------------|
| admin   | ✓        | ✓      | ✓            | ✓            | ✓             |
| manager | ✓        | ✗      | view only    | ✗            | ✓             |
| staff   | ✓        | ✗      | ✗            | ✗            | ✗             |

Set a user's role by updating `app_metadata` in Supabase Auth:
```json
{ "role": "admin", "office": "north" }
```

---

## Build Phases

| Phase | Modules | Status |
|-------|---------|--------|
| 1 | Core: Programs, Documents, Users, Auth | ✅ Code complete |
| 2 | CRM: Clients, Jobs funnel, Assignments, Tasks | ✅ Code complete |
| 3 | Finance: Hours, Payroll, Adjustments, Pay stubs | ✅ Code complete |
| 4 | Grants, Voting, Notifications | ✅ Code complete |
| 5 | WhatsApp (Make), Email (MS Graph) | Wired in config, implement webhook dispatcher |

---

## Deploy to Railway

```bash
railway login
railway new
railway link
railway vars set DATABASE_URL=... SUPABASE_JWT_SECRET=... # etc
railway up
```

## Deploy to Fly.io

```bash
fly launch
fly secrets set DATABASE_URL=... SUPABASE_JWT_SECRET=...
fly deploy
```
