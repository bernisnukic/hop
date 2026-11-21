// Package server is hop's HTTP API.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bernisnukic/hop/internal/store"
)

const (
	codeLength = 7
	maxURLLen  = 2048
	createTry  = 5
)

// Server serves the link API and the redirects.
type Server struct {
	store   store.Store
	metrics *Metrics
	log     *slog.Logger
	baseURL string

	ready    atomic.Bool // set once the store is migrated
	draining atomic.Bool // set on SIGTERM so the pod leaves the Service first
}

// New returns a Server. baseURL is used to build the short URL in responses.
func New(s store.Store, m *Metrics, log *slog.Logger, baseURL string) *Server {
	return &Server{store: s, metrics: m, log: log, baseURL: strings.TrimRight(baseURL, "/")}
}

// SetReady marks the backend as migrated and usable.
func (s *Server) SetReady() { s.ready.Store(true) }

// Drain makes /readyz fail so the endpoint is removed before shutdown.
func (s *Server) Drain() { s.draining.Store(true) }

// Handler returns the routed, instrumented handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("POST /api/links", s.createLink)
	mux.HandleFunc("GET /api/links/{code}", s.getLink)
	mux.HandleFunc("GET /{code}", s.redirect)
	return s.metrics.instrument(mux)
}

// healthz is the liveness probe: the process is up and serving HTTP.
func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// readyz is the readiness probe: migrated, not draining, database reachable.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	switch {
	case s.draining.Load():
		http.Error(w, "draining", http.StatusServiceUnavailable)
		return
	case !s.ready.Load():
		http.Error(w, "starting", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		http.Error(w, "database unreachable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ready\n"))
}

type createRequest struct {
	URL string `json:"url"`
}

type linkResponse struct {
	store.Link
	ShortURL string `json:"short_url"`
}

func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body must be JSON like {\"url\": \"https://...\"}")
		return
	}
	if msg := validateURL(req.URL); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	for range createTry {
		l, err := s.store.Create(r.Context(), store.NewCode(codeLength), req.URL)
		if errors.Is(err, store.ErrCodeTaken) {
			continue
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.metrics.created.Inc()
		writeJSON(w, http.StatusCreated, s.response(l))
		return
	}
	s.fail(w, r, errors.New("no free code after retries"))
}

func (s *Server) getLink(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.Get(r.Context(), r.PathValue("code"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such link")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.response(l))
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.Resolve(r.Context(), r.PathValue("code"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, l.URL, http.StatusFound)
}

func (s *Server) response(l store.Link) linkResponse {
	return linkResponse{Link: l, ShortURL: s.baseURL + "/" + l.Code}
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func validateURL(raw string) string {
	if raw == "" {
		return "url is required"
	}
	if len(raw) > maxURLLen {
		return "url is longer than 2048 characters"
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "url must be an absolute http or https URL"
	}
	return ""
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
