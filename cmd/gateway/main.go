package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/config"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/gateway"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/server"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg.LogLevel)
	log.Info("starting llm-gateway-platform",
		slog.String("env", cfg.Env),
		slog.String("addr", cfg.HTTPAddr),
	)

	rtr, err := gateway.Build(cfg, log)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := server.New(cfg, log, rtr)
	return srv.Start(ctx)
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler)
}
