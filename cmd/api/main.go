package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"nexus-pro-be/internal/config"
	"nexus-pro-be/internal/startup"
)

func main() {
	cfg, err := config.LoadE()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)}))
	if err != nil {
		logger.Error("invalid startup configuration", "error", err)
		os.Exit(1)
	}
	if err := cfg.ValidateStartup(); err != nil {
		logger.Error("invalid startup configuration", "error", err)
		os.Exit(1)
	}

	modules, err := startModules(context.Background(), cfg, logger)
	if err != nil {
		os.Exit(1)
	}
	defer modules.shutdown(logger)

	if err := startup.Print(os.Stdout, modules.report); err != nil {
		logger.Warn("startup report render failed", "error", err)
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info("nexus-pro-be started", "addr", cfg.HTTPAddr)
		errs <- modules.server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	modules.startBackgroundWorkers(ctx, logger)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := modules.server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
			os.Exit(1)
		}
	case err := <-errs:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}
}

func logLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
