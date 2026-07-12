package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/VolodymyrStetsenko/secureledger/internal/app"
	"github.com/VolodymyrStetsenko/secureledger/internal/httpapi"
	"github.com/VolodymyrStetsenko/secureledger/internal/risk"
	"github.com/VolodymyrStetsenko/secureledger/internal/store"
	"github.com/VolodymyrStetsenko/secureledger/internal/store/memory"
	postgresstore "github.com/VolodymyrStetsenko/secureledger/internal/store/postgres"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: envLogLevel("SECURELEDGER_LOG_LEVEL", slog.LevelInfo),
	}))
	if err := run(log); err != nil {
		log.Error("secureledger stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	repo, closeRepository, repositoryName, err := openRepository(ctx)
	if err != nil {
		return err
	}
	defer closeRepository()

	var notifier app.RiskNotifier
	if outbox, ok := repo.(risk.Outbox); ok {
		worker := risk.NewWorker(outbox, risk.NewLogPublisher(log), log)
		go worker.Run(ctx)
		log.Info("durable risk outbox enabled")
	} else {
		dispatcher := risk.NewDispatcher(log, 128)
		go dispatcher.Run(ctx)
		notifier = dispatcher
		log.Warn("risk delivery is process-local", "repository", repositoryName)
	}

	service := app.New(repo, notifier, app.Config{
		MaxTransferMinor:   envInt64("SECURELEDGER_MAX_TRANSFER_MINOR", 100_000_000),
		RiskThresholdMinor: envInt64("SECURELEDGER_RISK_THRESHOLD_MINOR", 1_000_000),
	})

	server := &http.Server{
		Addr:              envString("SECURELEDGER_ADDR", ":8080"),
		Handler:           httpapi.New(service, log),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("secureledger listening", "addr", server.Addr, "repository", repositoryName)
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown requested")
	case err := <-errCh:
		return fmt.Errorf("serve HTTP: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	return nil
}

func openRepository(ctx context.Context) (store.Repository, func(), string, error) {
	switch strings.ToLower(strings.TrimSpace(envString("SECURELEDGER_STORE", "memory"))) {
	case "memory":
		return memory.New(), func() {}, "memory", nil
	case "postgres", "postgresql":
		openCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		repo, err := postgresstore.Open(openCtx, os.Getenv("SECURELEDGER_DATABASE_URL"))
		if err != nil {
			return nil, nil, "", fmt.Errorf("open PostgreSQL repository: %w", err)
		}
		return repo, repo.Close, "postgres", nil
	default:
		return nil, nil, "", fmt.Errorf("unsupported SECURELEDGER_STORE value")
	}
}

func envString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt64(name string, fallback int64) int64 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envLogLevel(name string, fallback slog.Level) slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return fallback
	}
}
