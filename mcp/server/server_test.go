package server

import (
	"testing"
)

func TestNewServer(t *testing.T) {
	server := NewServer()

	if server == nil {
		t.Fatal("expected non-nil server")
	}

	if server.name != "mcp-accumulate" {
		t.Errorf("expected name 'mcp-accumulate', got '%s'", server.name)
	}

	if server.version != "0.2.0" {
		t.Errorf("expected version '0.2.0', got '%s'", server.version)
	}
}

func TestHandleRequest_Initialize(t *testing.T) {
	server := NewServer()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
		},
	}

	response := server.HandleRequest(request)

	// Check response structure
	if response["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc '2.0', got '%v'", response["jsonrpc"])
	}

	if response["id"] != 1 {
		t.Errorf("expected id 1, got '%v'", response["id"])
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		t.Fatal("expected result to be a map")
	}

	// Check protocol version
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("expected protocolVersion '2024-11-05', got '%v'", result["protocolVersion"])
	}

	// Check server info
	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("expected serverInfo to be a map")
	}

	if serverInfo["name"] != "mcp-accumulate" {
		t.Errorf("expected server name 'mcp-accumulate', got '%v'", serverInfo["name"])
	}

	if serverInfo["version"] != "0.2.0" {
		t.Errorf("expected server version '0.2.0', got '%v'", serverInfo["version"])
	}

	// Check capabilities
	capabilities, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("expected capabilities to be a map")
	}

	tools, ok := capabilities["tools"].(map[string]interface{})
	if !ok {
		t.Fatal("expected tools capability to be a map")
	}

	if tools == nil {
		t.Error("expected tools capability to be non-nil")
	}
}

func TestHandleRequest_ListTools(t *testing.T) {
	server := NewServer()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	}

	response := server.HandleRequest(request)

	// Check response structure
	if response["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc '2.0', got '%v'", response["jsonrpc"])
	}

	if response["id"] != 2 {
		t.Errorf("expected id 2, got '%v'", response["id"])
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		t.Fatal("expected result to be a map")
	}

	// Check tools array
	tools, ok := result["tools"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected tools to be an array")
	}

	// Should have 63 tools (all categories combined)
	if len(tools) != 63 {
		t.Errorf("expected 63 tools, got %d", len(tools))
	}

	// Check that each tool has required fields
	for i, tool := range tools {
		if _, ok := tool["name"]; !ok {
			t.Errorf("tool %d missing 'name' field", i)
		}
		if _, ok := tool["description"]; !ok {
			t.Errorf("tool %d missing 'description' field", i)
		}
		if _, ok := tool["inputSchema"]; !ok {
			t.Errorf("tool %d missing 'inputSchema' field", i)
		}
	}
}

func TestHandleRequest_CallTool_MissingParams(t *testing.T) {
	server := NewServer()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		// Missing params
	}

	response := server.HandleRequest(request)

	// Should return error
	if _, ok := response["error"]; !ok {
		t.Error("expected error response for missing params")
	}
}

func TestHandleRequest_CallTool_MissingToolName(t *testing.T) {
	server := NewServer()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]interface{}{
			// Missing name
			"arguments": map[string]interface{}{},
		},
	}

	response := server.HandleRequest(request)

	// Should return error
	if _, ok := response["error"]; !ok {
		t.Error("expected error response for missing tool name")
	}
}

func TestHandleRequest_InvalidMethod(t *testing.T) {
	server := NewServer()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "invalid/method",
	}

	response := server.HandleRequest(request)

	// Should return error
	errorResponse, ok := response["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error response")
	}

	if errorResponse["code"] != -32601 {
		t.Errorf("expected error code -32601, got %v", errorResponse["code"])
	}

	message, ok := errorResponse["message"].(string)
	if !ok {
		t.Fatal("expected error message to be a string")
	}

	if message != "Method not found: invalid/method" {
		t.Errorf("unexpected error message: %s", message)
	}
}

func TestHandleRequest_MissingMethod(t *testing.T) {
	server := NewServer()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      6,
		// Missing method
	}

	response := server.HandleRequest(request)

	// Should return error
	errorResponse, ok := response["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error response")
	}

	if errorResponse["code"] != -32600 {
		t.Errorf("expected error code -32600, got %v", errorResponse["code"])
	}
}

func TestExecuteTool_UnknownTool(t *testing.T) {
	server := NewServer()

	_, err := server.executeTool("unknown_tool", map[string]interface{}{})

	if err == nil {
		t.Error("expected error for unknown tool")
	}

	if err.Error() != "unknown tool: unknown_tool" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestExecuteTool_AllToolsRegistered(t *testing.T) {
	server := NewServer()

	// List of all 33 tools
	expectedTools := []string{
		// Original 4 tools
		"accumulate_query_account",
		"accumulate_query_tx",
		"accumulate_create_lite_account",
		"accumulate_send_tokens",

		// Tier 1: Core Query Tools
		"accumulate_query_chain",
		"accumulate_query_data",
		"accumulate_query_directory",
		"accumulate_query_pending",
		"accumulate_query_minor_block",
		"accumulate_query_major_block",

		// Tier 3: Network Status Tools
		"accumulate_node_info",
		"accumulate_network_status",
		"accumulate_consensus_status",
		"accumulate_metrics",
		"accumulate_faucet",

		// Tier 4: Advanced Search Tools
		"accumulate_search_public_key",
		"accumulate_search_public_key_hash",
		"accumulate_search_anchor",

		// Phase 1: KeyBook and KeyPage Query Tools
		"accumulate_query_keybook",
		"accumulate_query_keypage",

		// Phase 1.5: ADI Management Tools
		"accumulate_generate_key",
		"accumulate_add_credits",
		"accumulate_create_adi",

		// Phase 2: Data & Token Account Operations
		"accumulate_create_data_account",
		"accumulate_write_data",
		"accumulate_create_token_account",

		// Phase 3: Key Management Operations
		"accumulate_create_keypage",
		"accumulate_update_keypage",
		"accumulate_create_keybook",
		"accumulate_update_account_auth",

		// Phase 4: Token Issuance Operations
		"accumulate_create_token",
		"accumulate_issue_tokens",
		"accumulate_burn_tokens",
	}

	// Add wallet tools to the list
	walletTools := []string{
		"wallet_init",
		"wallet_vault_open",
		"wallet_vault_lock",
		"wallet_generate_key",
		"wallet_list_keys",
		"wallet_set_network",
		"wallet_get_status",
	}
	expectedTools = append(walletTools, expectedTools...)

	if len(expectedTools) != 40 {
		t.Fatalf("expected 40 tools in test list, got %d", len(expectedTools))
	}

	// Verify each tool is in the routing table
	// We can't call them without mocking the client, but we can verify they don't return "unknown tool" error
	for _, toolName := range expectedTools {
		// This will fail with network errors since we don't have a real network,
		// but it should NOT fail with "unknown tool" error
		_, err := server.executeTool(toolName, map[string]interface{}{
			"network": "http://invalid-test-url",
		})

		if err != nil && err.Error() == "unknown tool: "+toolName {
			t.Errorf("tool '%s' is not registered in routing table", toolName)
		}
		// We expect other errors (network, validation, etc.) which is fine
	}
}

func TestGetAllTools_Count(t *testing.T) {
	tools := GetAllTools()

	if len(tools) != 63 {
		t.Errorf("expected 63 tools from GetAllTools(), got %d", len(tools))
	}
}

func TestGetAllTools_Structure(t *testing.T) {
	tools := GetAllTools()

	// Verify each tool has correct structure
	for i, tool := range tools {
		// Check name
		name, ok := tool["name"].(string)
		if !ok {
			t.Errorf("tool %d: name is not a string", i)
			continue
		}

		if name == "" {
			t.Errorf("tool %d: name is empty", i)
		}

		// Check description
		description, ok := tool["description"].(string)
		if !ok {
			t.Errorf("tool %d (%s): description is not a string", i, name)
			continue
		}

		if description == "" {
			t.Errorf("tool %d (%s): description is empty", i, name)
		}

		// Check inputSchema
		inputSchema, ok := tool["inputSchema"].(map[string]interface{})
		if !ok {
			t.Errorf("tool %d (%s): inputSchema is not a map", i, name)
			continue
		}

		// Check inputSchema has type
		schemaType, ok := inputSchema["type"].(string)
		if !ok {
			t.Errorf("tool %d (%s): inputSchema type is not a string", i, name)
			continue
		}

		if schemaType != "object" {
			t.Errorf("tool %d (%s): inputSchema type is '%s', expected 'object'", i, name, schemaType)
		}

		// Check inputSchema has properties
		if _, ok := inputSchema["properties"]; !ok {
			t.Errorf("tool %d (%s): inputSchema missing properties", i, name)
		}
	}
}

func TestGetAllTools_SpecificTools(t *testing.T) {
	tools := GetAllTools()
	toolMap := make(map[string]map[string]interface{})

	for _, tool := range tools {
		if name, ok := tool["name"].(string); ok {
			toolMap[name] = tool
		}
	}

	// Test a few specific tools
	tests := []struct {
		name        string
		shouldExist bool
	}{
		{"accumulate_query_account", true},
		{"accumulate_send_tokens", true},
		{"accumulate_create_adi", true},
		{"accumulate_create_token", true},
		{"accumulate_issue_tokens", true},
		{"accumulate_burn_tokens", true},
		{"accumulate_create_keypage", true},
		{"accumulate_update_account_auth", true},
		{"accumulate_nonexistent_tool", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, exists := toolMap[tt.name]
			if exists != tt.shouldExist {
				if tt.shouldExist {
					t.Errorf("tool '%s' should exist but doesn't", tt.name)
				} else {
					t.Errorf("tool '%s' shouldn't exist but does", tt.name)
				}
			}
		})
	}
}

func TestErrorResponse(t *testing.T) {
	server := NewServer()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      123,
	}

	response := server.errorResponse(request, -32600, "Test error message")

	if response["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc '2.0', got '%v'", response["jsonrpc"])
	}

	if response["id"] != 123 {
		t.Errorf("expected id 123, got '%v'", response["id"])
	}

	errorInfo, ok := response["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error to be a map")
	}

	if errorInfo["code"] != -32600 {
		t.Errorf("expected error code -32600, got %v", errorInfo["code"])
	}

	if errorInfo["message"] != "Test error message" {
		t.Errorf("expected error message 'Test error message', got '%v'", errorInfo["message"])
	}
}

func TestHandleCallTool_WithValidTool(t *testing.T) {
	server := NewServer()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      7,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "accumulate_generate_key",
			"arguments": map[string]interface{}{
				// generate_key requires no arguments
			},
		},
	}

	response := server.HandleRequest(request)

	// Should succeed
	result, ok := response["result"].(map[string]interface{})
	if !ok {
		t.Fatal("expected result to be a map")
	}

	// Should have content
	content, ok := result["content"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected content to be an array")
	}

	if len(content) == 0 {
		t.Error("expected non-empty content")
	}
}
