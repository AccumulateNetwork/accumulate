package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHTTPServer_HandleRequest tests that HTTP server can handle MCP JSON-RPC requests
func TestHTTPServer_HandleRequest(t *testing.T) {
	// Create server instance
	srv := NewServer()
	httpSrv := NewHTTPServer(srv)

	// Create test HTTP request with JSON-RPC initialize call
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Call HTTP handler
	httpSrv.ServeHTTP(w, req)

	// Check response
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Parse response body
	var response map[string]interface{}
	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify response structure
	if response["jsonrpc"] != "2.0" {
		t.Errorf("Expected jsonrpc 2.0, got %v", response["jsonrpc"])
	}

	if response["id"] != float64(1) {
		t.Errorf("Expected id 1, got %v", response["id"])
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected result to be an object")
	}

	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected serverInfo in result")
	}

	if serverInfo["name"] != "mcp-accumulate" {
		t.Errorf("Expected server name mcp-accumulate, got %v", serverInfo["name"])
	}
}

// TestHTTPServer_ToolsListRequest tests listing tools over HTTP
func TestHTTPServer_ToolsListRequest(t *testing.T) {
	srv := NewServer()
	httpSrv := NewHTTPServer(srv)

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	}

	body, _ := json.Marshal(request)
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	httpSrv.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var response map[string]interface{}
	bodyBytes, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyBytes, &response)

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected result in response")
	}

	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("Expected tools array in result")
	}

	if len(tools) == 0 {
		t.Error("Expected at least one tool")
	}
}

// TestHTTPServer_ToolCallRequest tests calling a tool over HTTP
func TestHTTPServer_ToolCallRequest(t *testing.T) {
	srv := NewServer()
	httpSrv := NewHTTPServer(srv)

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "accumulate_db_list",
			"arguments": map[string]interface{}{},
		},
	}

	body, _ := json.Marshal(request)
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	httpSrv.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var response map[string]interface{}
	bodyBytes, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyBytes, &response)

	if response["id"] != float64(3) {
		t.Errorf("Expected id 3, got %v", response["id"])
	}

	// Should have result (even if it's an error result)
	if _, ok := response["result"]; !ok {
		t.Error("Expected result in response")
	}
}

// TestHTTPServer_InvalidJSON tests error handling for invalid JSON
func TestHTTPServer_InvalidJSON(t *testing.T) {
	srv := NewServer()
	httpSrv := NewHTTPServer(srv)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	httpSrv.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

// TestHTTPServer_MethodNotAllowed tests that only POST is allowed
func TestHTTPServer_MethodNotAllowed(t *testing.T) {
	srv := NewServer()
	httpSrv := NewHTTPServer(srv)

	req := httptest.NewRequest("GET", "/mcp", nil)
	w := httptest.NewRecorder()

	httpSrv.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

// TestHTTPServer_StartServer tests that HTTP server can start on a port
func TestHTTPServer_StartServer(t *testing.T) {
	srv := NewServer()

	// Start HTTP server on random port (0 = let OS pick)
	httpServer, err := StartHTTPServer(srv, "localhost", "0")
	if err != nil {
		t.Fatalf("Failed to start HTTP server: %v", err)
	}
	defer httpServer.Close()

	// Server should be listening
	if httpServer.Addr == "" {
		t.Error("Expected server to have an address")
	}
}

// TestHTTPServer_CORS tests CORS headers are set correctly
func TestHTTPServer_CORS(t *testing.T) {
	srv := NewServer()
	httpSrv := NewHTTPServer(srv)

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
	}

	body, _ := json.Marshal(request)
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000") // Set Origin header to trigger CORS response
	w := httptest.NewRecorder()

	httpSrv.ServeHTTP(w, req)

	resp := w.Result()

	// Check CORS headers - only set when Origin header is present
	if resp.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Error("Expected Access-Control-Allow-Origin header")
	}
}
