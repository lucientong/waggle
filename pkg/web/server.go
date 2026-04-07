// Package web provides an embedded HTTP server for Waggle's DAG visualization panel.
//
// The server embeds the frontend static files from the web/ directory and
// exposes a REST API for DAG structure, metrics, and a Server-Sent Events (SSE)
// endpoint for real-time agent status updates.
package web

import (
	"context"
	"embed"
	"log/slog"
	"net/http"
	"time"

	"github.com/lucientong/waggle/pkg/observe"
	"github.com/lucientong/waggle/pkg/waggle"
)

//go:embed static
var staticFiles embed.FS

// ServerConfig holds configuration for the visualization server.
type ServerConfig struct {
	// Addr is the TCP address to listen on (e.g., ":8080").
	Addr string
	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration
	// ShutdownTimeout is how long to wait for active connections to close on shutdown.
	ShutdownTimeout time.Duration
}

// DefaultConfig returns a ServerConfig with sensible defaults.
func DefaultConfig() ServerConfig {
	return ServerConfig{
		Addr:            ":8080",
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 5 * time.Second,
	}
}

// Server is the Waggle visualization HTTP server.
type Server struct {
	config     ServerConfig
	httpServer *http.Server
	hub        *sseHub
	waggleRef  *waggle.Waggle // may be nil if not connected to an orchestrator
	metrics    *observe.Metrics
}

// NewServer creates a new visualization server.
//
// waggleRef and metrics may be nil; the server will still serve the UI and
// return empty data from the API endpoints.
func NewServer(cfg ServerConfig, w *waggle.Waggle, m *observe.Metrics) *Server {
	s := &Server{
		config:    cfg,
		hub:       newSSEHub(),
		waggleRef: w,
		metrics:   m,
	}

	mux := http.NewServeMux()

	// Static files: serve the embedded web/ directory.
	// The embed.FS uses the directory name "static" (see //go:embed static above),
	// but we serve it at root, so we strip the "static/" prefix.
	staticHandler := http.FileServer(http.FS(staticFiles))
	mux.Handle("/", http.StripPrefix("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rewrite root to index.html.
		if r.URL.Path == "/" || r.URL.Path == "" {
			r.URL.Path = "/static/index.html"
		} else {
			r.URL.Path = "/static" + r.URL.Path
		}
		staticHandler.ServeHTTP(w, r)
	})))

	// API endpoints.
	mux.HandleFunc("GET /api/dag", s.handleDAG)
	mux.HandleFunc("GET /api/metrics", s.handleMetrics)
	mux.HandleFunc("GET /api/events", s.handleSSE)

	// Health check.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint
	})

	s.httpServer = &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// Start the SSE hub event loop immediately so that both Start() and
	// httptest.NewServer(Handler()) usage patterns work correctly.
	go s.hub.run()

	return s
}

// Start begins serving HTTP requests.
// The SSE hub is already running (started in NewServer).
// It blocks until the server exits or an error occurs.
func (s *Server) Start() error {
	slog.Info("waggle web server starting", "addr", s.config.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server and the SSE hub.
func (s *Server) Shutdown() error {
	close(s.hub.stop) // stop the SSE hub goroutine
	ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// Close stops the SSE hub goroutine without shutting down the HTTP server.
// This is useful in tests that use httptest.NewServer (which manages its own lifecycle).
func (s *Server) Close() {
	close(s.hub.stop)
}

// PublishEvent sends an observability event to all connected SSE clients.
// This can be called from the executor or agent wrappers.
func (s *Server) PublishEvent(event observe.Event) {
	s.hub.publish(event)
}

// EventChannel returns a write-only channel that the server listens on for events.
// Connect the executor's event output to this channel.
func (s *Server) EventChannel() chan<- observe.Event {
	return s.hub.inbound
}

// Handler returns the underlying http.Handler for use in tests with httptest.NewServer.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}
