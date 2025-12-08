// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	srv := New(&Config{})
	if srv == nil {
		t.Fatal("New returned nil")
	}
	if srv.tools == nil {
		t.Error("tools map is nil")
	}
	if srv.handlers == nil {
		t.Error("handlers map is nil")
	}
}

func TestRegisterTool(t *testing.T) {
	srv := New(&Config{})

	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]JSONSchema{
				"arg1": {Type: "string"},
			},
			Required: []string{"arg1"},
		},
	}

	handler := func(args map[string]any) (ToolCallResult, error) {
		return NewTextResult("success"), nil
	}

	srv.RegisterTool(tool, handler)

	if _, ok := srv.tools["test_tool"]; !ok {
		t.Error("tool not registered")
	}
	if _, ok := srv.handlers["test_tool"]; !ok {
		t.Error("handler not registered")
	}
}

func TestHandleInitialize(t *testing.T) {
	srv := New(&Config{})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}`),
	}

	data, _ := json.Marshal(req)
	resp := srv.handleMessage(data)

	if resp == nil {
		t.Fatal("no response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	// ID comes back as float64 after JSON round-trip
	if resp.ID != float64(1) {
		t.Errorf("expected ID 1, got %v", resp.ID)
	}

	result, ok := resp.Result.(InitializeResult)
	if !ok {
		t.Fatalf("expected InitializeResult, got %T", resp.Result)
	}
	if result.ProtocolVersion != ProtocolVersion {
		t.Errorf("expected protocol version %s, got %s", ProtocolVersion, result.ProtocolVersion)
	}
	if result.ServerInfo.Name != ServerName {
		t.Errorf("expected server name %s, got %s", ServerName, result.ServerInfo.Name)
	}
}

func TestHandleToolsList(t *testing.T) {
	srv := New(&Config{})

	// Register a test tool
	srv.RegisterTool(
		Tool{
			Name:        "test_tool",
			Description: "A test tool",
			InputSchema: JSONSchema{Type: "object"},
		},
		func(args map[string]any) (ToolCallResult, error) {
			return NewTextResult("ok"), nil
		},
	)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	data, _ := json.Marshal(req)
	resp := srv.handleMessage(data)

	if resp == nil {
		t.Fatal("no response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(ToolsListResult)
	if !ok {
		t.Fatalf("expected ToolsListResult, got %T", resp.Result)
	}
	if len(result.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "test_tool" {
		t.Errorf("expected tool name 'test_tool', got %s", result.Tools[0].Name)
	}
}

func TestHandleToolsCall(t *testing.T) {
	srv := New(&Config{})

	// Register a test tool
	srv.RegisterTool(
		Tool{
			Name:        "echo",
			Description: "Echoes input",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"message": {Type: "string"},
				},
			},
		},
		func(args map[string]any) (ToolCallResult, error) {
			msg, _ := args["message"].(string)
			return NewTextResult("Echo: " + msg), nil
		},
	)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"echo","arguments":{"message":"hello"}}`),
	}

	data, _ := json.Marshal(req)
	resp := srv.handleMessage(data)

	if resp == nil {
		t.Fatal("no response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(ToolCallResult)
	if !ok {
		t.Fatalf("expected ToolCallResult, got %T", resp.Result)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Text != "Echo: hello" {
		t.Errorf("expected 'Echo: hello', got %s", result.Content[0].Text)
	}
}

func TestHandleUnknownTool(t *testing.T) {
	srv := New(&Config{})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"unknown_tool","arguments":{}}`),
	}

	data, _ := json.Marshal(req)
	resp := srv.handleMessage(data)

	if resp == nil {
		t.Fatal("no response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}

	result, ok := resp.Result.(ToolCallResult)
	if !ok {
		t.Fatalf("expected ToolCallResult, got %T", resp.Result)
	}
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
}

func TestHandleUnknownMethod(t *testing.T) {
	srv := New(&Config{})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "unknown/method",
	}

	data, _ := json.Marshal(req)
	resp := srv.handleMessage(data)

	if resp == nil {
		t.Fatal("no response")
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != MethodNotFound {
		t.Errorf("expected code %d, got %d", MethodNotFound, resp.Error.Code)
	}
}

func TestHandleInvalidJSON(t *testing.T) {
	srv := New(&Config{})

	resp := srv.handleMessage([]byte(`{invalid json`))

	if resp == nil {
		t.Fatal("no response")
	}
	if resp.Error == nil {
		t.Fatal("expected parse error")
	}
	if resp.Error.Code != ParseError {
		t.Errorf("expected code %d, got %d", ParseError, resp.Error.Code)
	}
}

func TestHandleInvalidJSONRPCVersion(t *testing.T) {
	srv := New(&Config{})

	req := JSONRPCRequest{
		JSONRPC: "1.0",
		ID:      6,
		Method:  "ping",
	}

	data, _ := json.Marshal(req)
	resp := srv.handleMessage(data)

	if resp == nil {
		t.Fatal("no response")
	}
	if resp.Error == nil {
		t.Fatal("expected error for invalid JSON-RPC version")
	}
	if resp.Error.Code != InvalidRequest {
		t.Errorf("expected code %d, got %d", InvalidRequest, resp.Error.Code)
	}
}

func TestHandlePing(t *testing.T) {
	srv := New(&Config{})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      7,
		Method:  "ping",
	}

	data, _ := json.Marshal(req)
	resp := srv.handleMessage(data)

	if resp == nil {
		t.Fatal("no response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestServerRun(t *testing.T) {
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	output := &bytes.Buffer{}

	srv := New(&Config{
		Input:  input,
		Output: output,
	})

	err := srv.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Parse response
	var resp JSONRPCResponse
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.ID != float64(1) { // JSON numbers are float64
		t.Errorf("expected ID 1, got %v", resp.ID)
	}
}

func TestNewTextContent(t *testing.T) {
	content := NewTextContent("hello")
	if content.Type != "text" {
		t.Errorf("expected type 'text', got %s", content.Type)
	}
	if content.Text != "hello" {
		t.Errorf("expected text 'hello', got %s", content.Text)
	}
}

func TestNewErrorResult(t *testing.T) {
	err := errors.New("test error")
	result := NewErrorResult(err)
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Text != "test error" {
		t.Errorf("expected 'test error', got %s", result.Content[0].Text)
	}
}

func TestNewTextResult(t *testing.T) {
	result := NewTextResult("success")
	if result.IsError {
		t.Error("expected IsError to be false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Text != "success" {
		t.Errorf("expected text 'success', got %s", result.Content[0].Text)
	}
}
