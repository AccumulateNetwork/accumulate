package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
)

// HTTPServer wraps the MCP server with HTTP transport
type HTTPServer struct {
	server *Server
}

// NewHTTPServer creates a new HTTP server wrapper
func NewHTTPServer(srv *Server) *HTTPServer {
	return &HTTPServer{
		server: srv,
	}
}

// ServeHTTP implements http.Handler interface
func (h *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle preflight OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow POST
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON-RPC request
	var request map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Process request through MCP server
	response := h.server.HandleRequest(request)

	// Send JSON-RPC response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// HTTPServerInstance wraps http.Server with additional functionality
type HTTPServerInstance struct {
	*http.Server
	listener net.Listener
}

// StartHTTPServer starts an HTTP server on the specified host and port
// Returns the server instance and any error
func StartHTTPServer(srv *Server, host, port string) (*HTTPServerInstance, error) {
	httpSrv := NewHTTPServer(srv)

	// Start listening first to get the actual address
	listener, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s:%s: %w", host, port, err)
	}

	// Create HTTP server
	server := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: httpSrv,
	}

	instance := &HTTPServerInstance{
		Server:   server,
		listener: listener,
	}

	// Start server in background
	go func() {
		log.Printf("HTTP server listening on http://%s", server.Addr)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return instance, nil
}

// Close gracefully shuts down the HTTP server
func (s *HTTPServerInstance) Close() error {
	log.Printf("Shutting down HTTP server on %s", s.Addr)
	if err := s.Shutdown(context.Background()); err != nil {
		return err
	}
	return s.listener.Close()
}
