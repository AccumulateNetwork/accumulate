// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ServerConfig holds configuration for the metrics HTTP server.
type ServerConfig struct {
	// Address is the address to listen on (e.g., ":9090" or "127.0.0.1:9090").
	// Defaults to ":9090".
	Address string

	// Path is the HTTP path to serve metrics on.
	// Defaults to "/metrics".
	Path string

	// ReadTimeout is the HTTP read timeout.
	// Defaults to 10 seconds.
	ReadTimeout time.Duration

	// WriteTimeout is the HTTP write timeout.
	// Defaults to 10 seconds.
	WriteTimeout time.Duration
}

// applyDefaults fills in default values for unset configuration fields.
func (c *ServerConfig) applyDefaults() {
	if c.Address == "" {
		c.Address = ":9090"
	}
	if c.Path == "" {
		c.Path = "/metrics"
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 10 * time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 10 * time.Second
	}
}

// Server is an HTTP server that exposes Prometheus metrics.
type Server struct {
	config ServerConfig
	server *http.Server
	mu     sync.Mutex
}

// NewServer creates a new metrics server with the given configuration.
func NewServer(config ServerConfig) *Server {
	config.applyDefaults()
	return &Server{config: config}
}

// Start starts the metrics HTTP server.
// This method is non-blocking and returns immediately after the server starts.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return fmt.Errorf("server already started")
	}

	mux := http.NewServeMux()
	mux.Handle(s.config.Path, promhttp.Handler())

	// Add a health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	s.server = &http.Server{
		Addr:         s.config.Address,
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	listener, err := net.Listen("tcp", s.config.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.config.Address, err)
	}

	// Capture the server in the goroutine to avoid race conditions
	srv := s.server
	go func() {
		slog.Info("Starting metrics server",
			"address", s.config.Address,
			"path", s.config.Path)

		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("Metrics server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the metrics HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return nil
	}

	slog.Info("Stopping metrics server")

	err := s.server.Shutdown(ctx)
	s.server = nil
	return err
}

// Address returns the configured address.
func (s *Server) Address() string {
	return s.config.Address
}

// Path returns the configured metrics path.
func (s *Server) Path() string {
	return s.config.Path
}
