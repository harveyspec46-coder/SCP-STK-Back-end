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
