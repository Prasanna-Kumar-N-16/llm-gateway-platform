package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/config"
)

// Server wraps the HTTP server and its dependencies. It owns the lifecycle of
// the underlying listener and provides graceful shutdown.
type Server struct {
	cfg    *config.Config
	log    *slog.Logger
	http   *http.Server
	routes *http.ServeMux
}

// New constructs a Server from configuration and a logger. Routes are
// registered here so the mux is fully wired before Start is called.
func New(cfg *config.Config, log *slog.Logger) *Server {
	mux := http.NewServeMux()
	s := &Server{
		cfg:    cfg,
		log:    log,
		routes: mux,
		http: &http.Server{
			Addr:         cfg.HTTPAddr,
			Handler:      mux,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.routes.HandleFunc("GET /healthz", s.handleHealth)
	s.routes.HandleFunc("GET /readyz", s.handleReady)
}

// Start begins serving and blocks until the context is canceled, then performs
// a graceful shutdown bounded by the configured shutdown timeout.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("http server listening", slog.String("addr", s.cfg.HTTPAddr))
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.log.Info("shutdown signal received, draining connections")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
