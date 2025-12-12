// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestJSONRPCRequestMarshal(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test",
		Params:  json.RawMessage(`{"key":"value"}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded JSONRPCRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc '2.0', got %s", decoded.JSONRPC)
	}
	if decoded.Method != "test" {
		t.Errorf("expected method 'test', got %s", decoded.Method)
	}
}

func TestJSONRPCResponseMarshal(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  map[string]string{"status": "ok"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded JSONRPCResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Error != nil {
		t.Error("expected no error")
	}
}

func TestJSONRPCResponseWithError(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Error: &JSONRPCError{
			Code:    InvalidParams,
			Message: "Invalid params",
			Data:    "missing required field",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded JSONRPCResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Error == nil {
		t.Fatal("expected error")
	}
	if decoded.Error.Code != InvalidParams {
		t.Errorf("expected code %d, got %d", InvalidParams, decoded.Error.Code)
	}
}

func TestToolMarshal(t *testing.T) {
	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool for testing",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]JSONSchema{
				"name": {Type: "string"},
				"age":  {Type: "number"},
			},
			Required: []string{"name"},
		},
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded Tool
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Name != "test_tool" {
		t.Errorf("expected name 'test_tool', got %s", decoded.Name)
	}
	if len(decoded.InputSchema.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(decoded.InputSchema.Properties))
	}
	if len(decoded.InputSchema.Required) != 1 {
		t.Errorf("expected 1 required field, got %d", len(decoded.InputSchema.Required))
	}
}

func TestToolCallParamsMarshal(t *testing.T) {
	params := ToolCallParams{
		Name: "my_tool",
		Arguments: map[string]any{
			"arg1": "value1",
			"arg2": 42,
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ToolCallParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Name != "my_tool" {
		t.Errorf("expected name 'my_tool', got %s", decoded.Name)
	}
	if decoded.Arguments["arg1"] != "value1" {
		t.Errorf("expected arg1 'value1', got %v", decoded.Arguments["arg1"])
	}
}

func TestToolCallResultMarshal(t *testing.T) {
	result := ToolCallResult{
		Content: []ContentBlock{
			{Type: "text", Text: "Hello, world!"},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ToolCallResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(decoded.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(decoded.Content))
	}
	if decoded.Content[0].Type != "text" {
		t.Errorf("expected type 'text', got %s", decoded.Content[0].Type)
	}
	if decoded.Content[0].Text != "Hello, world!" {
		t.Errorf("expected text 'Hello, world!', got %s", decoded.Content[0].Text)
	}
}

func TestInitializeResultMarshal(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: Capabilities{
			Tools: &ToolsCapability{ListChanged: true},
		},
		ServerInfo: ServerInfo{
			Name:    "test-server",
			Version: "1.0.0",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded InitializeResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version '2024-11-05', got %s", decoded.ProtocolVersion)
	}
	if decoded.ServerInfo.Name != "test-server" {
		t.Errorf("expected server name 'test-server', got %s", decoded.ServerInfo.Name)
	}
}

func TestNewErrorResultWithMessage(t *testing.T) {
	err := errors.New("something went wrong")
	result := NewErrorResult(err)

	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Text != "something went wrong" {
		t.Errorf("expected error message, got %s", result.Content[0].Text)
	}
}

func TestErrorCodes(t *testing.T) {
	if ParseError != -32700 {
		t.Errorf("ParseError should be -32700, got %d", ParseError)
	}
	if InvalidRequest != -32600 {
		t.Errorf("InvalidRequest should be -32600, got %d", InvalidRequest)
	}
	if MethodNotFound != -32601 {
		t.Errorf("MethodNotFound should be -32601, got %d", MethodNotFound)
	}
	if InvalidParams != -32602 {
		t.Errorf("InvalidParams should be -32602, got %d", InvalidParams)
	}
	if InternalError != -32603 {
		t.Errorf("InternalError should be -32603, got %d", InternalError)
	}
}
