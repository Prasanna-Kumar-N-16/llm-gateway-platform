package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/config"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/router"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

// Server wraps the HTTP server and its dependencies. It owns the lifecycle of
// the underlying listener and provides graceful shutdown.
type Server struct {
	cfg    *config.Config
	log    *slog.Logger
	router *router.Router
	http   *http.Server
	routes *http.ServeMux
}

// New constructs a Server from configuration, a logger, and the router used
// to serve chat requests. Routes are registered here so the mux is fully
// wired before Start is called.
func New(cfg *config.Config, log *slog.Logger, rtr *router.Router) *Server {
	mux := http.NewServeMux()
	s := &Server{
		cfg:    cfg,
		log:    log,
		router: rtr,
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
	s.routes.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
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

// chatCompletionRequest is the wire shape accepted by POST /v1/chat/completions.
type chatCompletionRequest struct {
	Model       string             `json:"model"`
	System      string             `json:"system,omitempty"`
	Messages    []provider.Message `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
}

// chatCompletionResponse is the wire shape returned by POST /v1/chat/completions.
type chatCompletionResponse struct {
	Provider     provider.Name         `json:"provider"`
	Model        string                `json:"model"`
	Content      string                `json:"content"`
	FinishReason provider.FinishReason `json:"finish_reason"`
	Usage        provider.Usage        `json:"usage"`
	Attempts     int                   `json:"attempts"`
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req chatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is not valid JSON")
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "model is required")
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "messages must not be empty")
		return
	}

	result, err := s.router.Chat(r.Context(), &provider.ChatRequest{
		Model:       req.Model,
		System:      req.System,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})
	if err != nil {
		s.writeChatError(w, req.Model, err)
		return
	}

	writeJSON(w, http.StatusOK, chatCompletionResponse{
		Provider:     result.Response.Provider,
		Model:        result.Response.Model,
		Content:      result.Response.Content,
		FinishReason: result.Response.FinishReason,
		Usage:        result.Response.Usage,
		Attempts:     result.Attempts,
	})
}

// writeChatError maps a router.Chat failure onto an HTTP status: context
// deadlines become 504/408, an upstream failure across every target in the
// fallback chain becomes 502 (or the last provider's own status, if any),
// and anything else (e.g. an unrecognized model) is a 400.
func (s *Server) writeChatError(w http.ResponseWriter, model string, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "upstream_timeout", err.Error())
	case errors.Is(err, context.Canceled):
		writeError(w, http.StatusRequestTimeout, "request_canceled", err.Error())
	default:
		var routeErr *router.RouteError
		if errors.As(err, &routeErr) {
			status := http.StatusBadGateway
			var pErr *provider.Error
			if errors.As(err, &pErr) && pErr.Status != 0 {
				status = pErr.Status
			}
			s.log.Warn("chat request failed on all targets",
				slog.String("model", model), slog.Any("error", err))
			writeError(w, status, "upstream_error", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "unknown_model", err.Error())
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
