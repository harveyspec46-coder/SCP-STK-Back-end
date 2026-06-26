package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scp-stk/hub/internal/auth"
	"github.com/scp-stk/hub/internal/handler"
)

type Handlers struct {
	CRM           *handler.CRMHandler
	Tasks         *handler.TaskHandler
	Finance       *handler.FinanceHandler
	Grants        *handler.GrantHandler
	Voting        *handler.VotingHandler
	Notifications *handler.NotificationHandler

	// Added alongside admin auto-recognition, participants/volunteers,
	// MOUs, resources, workshops, shift scheduling, the audit log, and
	// in-app e-signatures.
	Admin        *handler.AdminHandler
	Participants *handler.ParticipantHandler
	MOUs         *handler.MOUHandler
	Resources    *handler.ResourceHandler
	Workshops    *handler.WorkshopHandler
	Shifts       *handler.ShiftHandler
	Audit        *handler.AuditHandler
	ESign        *handler.ESignHandler
}

func New(jwtSecret string, pool *pgxpool.Pool, h Handlers) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware ────────────────────────────────────────────────────
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "https://scp-stk-hub.vercel.app"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// ── Health check (public) ────────────────────────────────────────────────
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"scp-stk-hub"}`))
	})

	// ── Protected API routes ─────────────────────────────────────────────────
	r.Route("/api", func(r chi.Router) {
		r.Use(auth.Middleware(jwtSecret, pool))

		// ── CRM — Clients + Jobs (funnel) — ADMIN ONLY ───────────────────────
		// Per org policy, only the 3 board Admins can see the Job Funnel,
		// not Managers and not Staff. RequireExactRole enforces this even
		// though Manager normally outranks Staff in the general hierarchy.
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireExactRole("admin"))

			r.Route("/crm/clients", func(r chi.Router) {
				r.Get("/", h.CRM.ListClients)
				r.Post("/", h.CRM.CreateClient)
				r.Get("/{id}", h.CRM.GetClient)
				r.Put("/{id}", h.CRM.UpdateClient)
				r.Delete("/{id}", h.CRM.DeleteClient)
			})

			r.Route("/crm/jobs", func(r chi.Router) {
				r.Get("/", h.CRM.ListJobs)
				r.Post("/", h.CRM.CreateJob)
				r.Get("/{id}", h.CRM.GetJob)
				r.Put("/{id}", h.CRM.UpdateJob)
				r.Patch("/{id}/stage", h.CRM.AdvanceStage)
				r.Post("/{id}/assign", h.CRM.AssignStaff)
				r.Delete("/{id}/assign/{uid}", h.CRM.RemoveAssignment)
				r.Post("/{id}/tasks", h.CRM.CreateJobTask)
			})

			r.Get("/crm/staff/{uid}/workload", h.CRM.StaffWorkload)
		})

		// ── Tasks — board only (not in the Staff nav, but staff still need
		// to read+complete tasks assigned to them via My Tasks) ──────────────
		r.Route("/tasks", func(r chi.Router) {
			r.Get("/", h.Tasks.List)
			r.Post("/", h.Tasks.Create)
			r.Patch("/{id}/status", h.Tasks.UpdateStatus)
			r.With(auth.RequireRole("manager")).
				Delete("/{id}", h.Tasks.Delete)
			r.Get("/productivity", h.Tasks.Productivity)
		})

		// ── Finance + Hours + Payroll — ADMIN ONLY ───────────────────────────
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireExactRole("admin"))

			r.Route("/hours", func(r chi.Router) {
				r.Get("/", h.Finance.ListHours)
				r.Post("/", h.Finance.LogHours)
			})
			r.Route("/payroll", func(r chi.Router) {
				r.Get("/", h.Finance.GetPayroll)
				r.Patch("/{uid}/adjust", h.Finance.AdjustPay)
			})
			r.Get("/finance/summary", h.Finance.Summary)
		})

		// ── Grants — board (admin + manager) ─────────────────────────────────
		r.Route("/grants", func(r chi.Router) {
			r.Get("/", h.Grants.List)
			r.With(auth.RequireRole("manager")).Post("/", h.Grants.Create)
			r.With(auth.RequireRole("manager")).Patch("/{id}/stage", h.Grants.AdvanceStage)
			r.With(auth.RequireRole("manager")).Post("/{id}/assign", h.Grants.Assign)
		})

		// ── Voting ────────────────────────────────────────────────────────────
		r.Route("/resolutions", func(r chi.Router) {
			r.Get("/", h.Voting.List)
			r.With(auth.RequireRole("manager")).Post("/", h.Voting.Create)
			r.Post("/{id}/vote", h.Voting.CastVote)
		})

		// ── Notifications ─────────────────────────────────────────────────────
		r.Route("/notifications", func(r chi.Router) {
			r.Get("/", h.Notifications.List)
			r.Patch("/{id}/read", h.Notifications.MarkRead)
		})

		// ── Participants & Volunteers — board (admin + manager) ──────────────
		r.Route("/participants", func(r chi.Router) {
			r.Use(auth.RequireRole("manager"))
			r.Get("/", h.Participants.List)
			r.Post("/", h.Participants.Create)
			r.Patch("/{id}/stage", h.Participants.AdvanceStage)
		})

		// ── MOUs & Contracts — board ───────────────────────────────────────────
		r.Route("/mous", func(r chi.Router) {
			r.Use(auth.RequireRole("manager"))
			r.Get("/", h.MOUs.List)
			r.Post("/", h.MOUs.Create)
			r.Patch("/{id}/renew", h.MOUs.Renew)
		})

		// ── Resources (docs + videos) — readable by everyone, board uploads ──
		r.Route("/resources", func(r chi.Router) {
			r.Get("/", h.Resources.List)
			r.With(auth.RequireRole("manager")).Post("/", h.Resources.Create)
		})

		// ── Workshops — readable by everyone, board manages ───────────────────
		r.Route("/workshops", func(r chi.Router) {
			r.Get("/", h.Workshops.List)
			r.With(auth.RequireRole("manager")).Post("/", h.Workshops.Create)
			r.With(auth.RequireRole("manager")).Patch("/{id}/recording", h.Workshops.AddRecording)
		})

		// ── Shift schedule — board only ───────────────────────────────────────
		r.Route("/shifts", func(r chi.Router) {
			r.Use(auth.RequireRole("manager"))
			r.Get("/", h.Shifts.List)
			r.Post("/", h.Shifts.Create)
			r.Patch("/{id}/move", h.Shifts.Move)
			r.Delete("/{id}", h.Shifts.Delete)
		})

		// ── Audit log — admin only ─────────────────────────────────────────────
		r.With(auth.RequireExactRole("admin")).
			Get("/audit-log", h.Audit.List)

		// ── E-Signatures — board (admin + manager), never staff ──────────────
		r.Route("/esign", func(r chi.Router) {
			r.Use(auth.RequireRole("manager"))
			r.Get("/documents", h.ESign.List)
			r.Post("/documents", h.ESign.Create)
			r.Post("/signers/{signerId}/sign", h.ESign.Sign)
		})

		// ── Admin — allowlist + Users & IDs panel — admin only ───────────────
		r.Route("/admin", func(r chi.Router) {
			r.Use(auth.RequireExactRole("admin"))
			r.Get("/allowlist", h.Admin.ListAllowlist)
			r.Post("/allowlist", h.Admin.AddAllowlistEntry)
			r.Delete("/allowlist/{email}", h.Admin.RemoveAllowlistEntry)
			r.Get("/users", h.Admin.ListUsers)
			r.Patch("/users/{id}/display-id", h.Admin.AssignDisplayID)
		})
	})

	return r
}
