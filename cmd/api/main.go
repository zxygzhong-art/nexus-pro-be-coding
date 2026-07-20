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

	"nexus-pro-api/internal/config"
	"nexus-pro-api/internal/startup"
)

// main 啟動 API 程序並協調關閉流程。
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
		logger.Error("api startup aborted", "error", err)
		os.Exit(1)
	}
	defer modules.shutdown(logger)

	if err := startup.Print(os.Stdout, modules.report); err != nil {
		logger.Warn("startup report render failed", "error", err)
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info("nexus-pro-api started", "addr", cfg.HTTPAddr)
		errs <- modules.server.ListenAndServe()
	}()
	if modules.metricsServer != nil {
		go func() {
			logger.Info("metrics server started", "addr", modules.metricsServer.Addr)
			if err := modules.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("metrics server failed", "error", err)
			}
		}()
	}

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
	// 業務 server 已經停止；接著停止 metrics listener。
	// 如此 scrape 能在 drain 視窗結束前持續觀測程序。
	if modules.metricsServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := modules.metricsServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("metrics server shutdown failed", "error", err)
		}
		cancel()
	}
	// 在關閉 Postgres pool 與 Redis client 前，先取消 worker context 並等待背景工作收斂。
	// 延後的模組關閉流程會在背景工作收斂後關閉 Postgres pool 與 Redis client。
	stop()
	modules.waitForBackgroundWorkers(5*time.Second, logger)
}

// logLevel 處理 log level。
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
