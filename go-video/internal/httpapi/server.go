// Package httpapi exposes the render endpoints over chi, mirroring the Python
// FastAPI service: bearer auth, the same validation rules, and fire-and-enqueue
// semantics returning {"status":"Ok"}.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/viralshort/go-video/internal/queue"
	"github.com/viralshort/go-video/internal/render"
)

// Submitter enqueues background work (the worker pool in prod, a fake in tests).
type Submitter interface {
	Submit(job queue.Job) bool
}

type Server struct {
	secret string
	deps   *render.Deps
	queue  Submitter
}

func NewServer(secret string, deps *render.Deps, q Submitter) *Server {
	return &Server{secret: secret, deps: deps, queue: q}
}

// Handler builds the chi router.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)
		r.Post("/generation/narration", s.handleNarration)
		r.Post("/generation/askreddit", s.handleAskReddit)
	})
	return r
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth || subtle.ConstantTimeCompare([]byte(token), []byte(s.secret)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"detail": "Unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleNarration(w http.ResponseWriter, r *http.Request) {
	var req narrationRequest
	if !decode(w, r, &req) {
		return
	}
	job, err := req.validate()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"detail": err.Error()})
		return
	}
	deps := s.deps
	if !s.queue.Submit(func(ctx context.Context) { deps.RunNarration(ctx, job) }) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"detail": "queue full, retry later"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "Ok"})
}

func (s *Server) handleAskReddit(w http.ResponseWriter, r *http.Request) {
	var req askRedditRequest
	if !decode(w, r, &req) {
		return
	}
	job, err := req.validate()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"detail": err.Error()})
		return
	}
	deps := s.deps
	if !s.queue.Submit(func(ctx context.Context) { deps.RunAskReddit(ctx, job) }) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"detail": "queue full, retry later"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "Ok"})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	// Unknown fields are ignored (matching Pydantic's default), so callers can
	// send extra fields without being rejected.
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "invalid JSON body"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
