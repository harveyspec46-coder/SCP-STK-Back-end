package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scp-stk/hub/internal/config"
	"github.com/scp-stk/hub/internal/db"
	"github.com/scp-stk/hub/internal/handler"
	"github.com/scp-stk/hub/internal/repository"
	"github.com/scp-stk/hub/internal/router"
)

func main() {
	// ── Config ───────────────────────────────────────────────────────────────
	cfg := config.Load()

	// ── Database pool ────────────────────────────────────────────────────────
	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()
	log.Printf("connected to Supabase PostgreSQL (%s)", cfg.Env)

	// ── Repositories ─────────────────────────────────────────────────────────
	crmRepo := repository.NewCRMRepo(pool)
	taskRepo := repository.NewTaskRepo(pool)
	finRepo := repository.NewFinanceRepo(pool)
	grantRepo := repository.NewGrantRepo(pool)
	voteRepo := repository.NewVotingRepo(pool)
	notifRepo := repository.NewNotificationRepo(pool)

	adminRepo := repository.NewAdminRepo(pool)
	participantRepo := repository.NewParticipantRepo(pool)
	mouRepo := repository.NewMOURepo(pool)
	resourceRepo := repository.NewResourceRepo(pool)
	workshopRepo := repository.NewWorkshopRepo(pool)
	shiftRepo := repository.NewShiftRepo(pool)
	auditRepo := repository.NewAuditRepo(pool)
	esignRepo := repository.NewESignRepo(pool)

	// ── Handlers ─────────────────────────────────────────────────────────────
	handlers := router.Handlers{
		CRM:           handler.NewCRMHandler(crmRepo, taskRepo, notifRepo),
		Tasks:         handler.NewTaskHandler(taskRepo, notifRepo),
		Finance:       handler.NewFinanceHandler(finRepo),
		Grants:        handler.NewGrantHandler(grantRepo, notifRepo),
		Voting:        handler.NewVotingHandler(voteRepo, notifRepo),
		Notifications: handler.NewNotificationHandler(notifRepo),

		Admin:        handler.NewAdminHandler(adminRepo, auditRepo),
		Participants: handler.NewParticipantHandler(participantRepo, auditRepo),
		MOUs:         handler.NewMOUHandler(mouRepo, auditRepo),
		Resources:    handler.NewResourceHandler(resourceRepo),
		Workshops:    handler.NewWorkshopHandler(workshopRepo),
		Shifts:       handler.NewShiftHandler(shiftRepo, auditRepo),
		Audit:        handler.NewAuditHandler(auditRepo),
		ESign:        handler.NewESignHandler(esignRepo, auditRepo),
	}

	// ── Router ───────────────────────────────────────────────────────────────
	httpHandler := router.New(cfg.SupabaseJWTSecret, pool, handlers, cfg.SupabaseURL)

	// ── HTTP server ──────────────────────────────────────────────────────────
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      httpHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start in goroutine so we can listen for shutdown signals
	go func() {
		log.Printf("SCP-STK Hub listening on :%s [%s]", cfg.Port, cfg.Env)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// ── Grant deadline alerts: check every hour, notify all admins/managers
	//    72 hours before a grant's deadline (incoming/in_progress only) ───────
	go func() {
		checkDeadlines := func() {
			grants, err := grantRepo.DueForDeadlineAlert(ctx)
			if err != nil {
				log.Printf("grant deadline check failed: %v", err)
				return
			}
			if len(grants) == 0 {
				return
			}
			recipients, err := grantRepo.AdminManagerUserIDs(ctx)
			if err != nil {
				log.Printf("failed to load admin/manager list for grant alerts: %v", err)
				return
			}
			for _, g := range grants {
				deadlineStr := ""
				if g.Deadline != nil {
					deadlineStr = g.Deadline.Format("Jan 2, 2006")
				}
				body := "Grant deadline approaching: " + g.Title + " (" + g.Funder + ") — due " + deadlineStr
				for _, uid := range recipients {
					_ = notifRepo.Create(ctx, uid, "grant_deadline", body, &g.ID)
				}
				if err := grantRepo.MarkDeadlineNotified(ctx, g.ID); err != nil {
					log.Printf("failed to mark grant %s as notified: %v", g.ID, err)
				}
			}
			log.Printf("grant deadline alerts sent for %d grant(s)", len(grants))
		}
		checkDeadlines() // run once at startup so we don't wait a full hour
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			checkDeadlines()
		}
	}()

	// ── Graceful shutdown ────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("server stopped")
}
