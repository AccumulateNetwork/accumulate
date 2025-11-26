package server

import (
	"encoding/json"
	"testing"
)

func TestGetAllPrompts(t *testing.T) {
	prompts := GetAllPrompts()

	// Should have at least 9 prompts
	if len(prompts) < 9 {
		t.Errorf("Expected at least 9 prompts, got %d", len(prompts))
	}

	// Verify expected prompts exist
	expectedPrompts := map[string]bool{
		"deploy-follower-node":       false,
		"monitor-follower-health":    false,
		"troubleshoot-follower-sync": false,
		"setup-dev-wallet":           false,
		"quick-node-status":          false,
		"organize-documentation":     false,
		"prepare-mainnet-follower":   false,
		"recovery-from-failure":      false,
		"mainnet-sync-status":        false,
	}

	for _, prompt := range prompts {
		if _, ok := expectedPrompts[prompt.Name]; ok {
			expectedPrompts[prompt.Name] = true
		}
	}

	// Check all expected prompts were found
	for name, found := range expectedPrompts {
		if !found {
			t.Errorf("Expected prompt %s not found", name)
		}
	}
}

func TestValidatePromptArguments(t *testing.T) {
	tests := []struct {
		name        string
		promptName  string
		args        map[string]string
		expectError bool
	}{
		{
			name:       "deploy-follower-node with all required args",
			promptName: "deploy-follower-node",
			args: map[string]string{
				"dn_database":  "/path/to/dn",
				"bvn_database": "/path/to/bvn",
				"work_dir":     "/work",
			},
			expectError: false,
		},
		{
			name:       "deploy-follower-node missing required arg",
			promptName: "deploy-follower-node",
			args: map[string]string{
				"dn_database": "/path/to/dn",
			},
			expectError: true,
		},
		{
			name:        "monitor-follower-health with no args (all optional)",
			promptName:  "monitor-follower-health",
			args:        map[string]string{},
			expectError: false,
		},
		{
			name:        "unknown prompt",
			promptName:  "non-existent-prompt",
			args:        map[string]string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePromptArguments(tt.promptName, tt.args)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestGetPromptTemplate(t *testing.T) {
	tests := []struct {
		name       string
		promptName string
		args       map[string]string
		checkText  string
	}{
		{
			name:       "deploy-follower-node template",
			promptName: "deploy-follower-node",
			args: map[string]string{
				"dn_database":  "/snapshots/dn",
				"bvn_database": "/snapshots/bvn",
				"work_dir":     "/accumulate/follower",
			},
			checkText: "Deploy Accumulate Follower Node",
		},
		{
			name:       "monitor-follower-health template",
			promptName: "monitor-follower-health",
			args:       map[string]string{},
			checkText:  "Monitor Accumulate Follower Health",
		},
		{
			name:       "troubleshoot-follower-sync template",
			promptName: "troubleshoot-follower-sync",
			args: map[string]string{
				"symptom": "no_peers",
			},
			checkText: "Troubleshoot Accumulate Follower Sync Issues",
		},
		{
			name:       "setup-dev-wallet template",
			promptName: "setup-dev-wallet",
			args: map[string]string{
				"network": "testnet",
			},
			checkText: "Setup development wallet for network: testnet",
		},
		{
			name:       "quick-node-status template",
			promptName: "quick-node-status",
			args:       map[string]string{},
			checkText:  "Quick Node Status Check",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template, err := GetPromptTemplate(tt.promptName, tt.args)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if template == "" {
				t.Error("Template should not be empty")
			}

			// Check for expected text in template
			if tt.checkText != "" {
				if !contains(template, tt.checkText) {
					t.Errorf("Template should contain '%s', but it doesn't", tt.checkText)
				}
			}
		})
	}
}

func TestHandleListPrompts(t *testing.T) {
	server := NewServer()

	response := server.handleListPrompts(1)

	// Check response structure
	if response["jsonrpc"] != "2.0" {
		t.Error("Response should have jsonrpc 2.0")
	}

	if response["id"] != 1 {
		t.Error("Response id should match request id")
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		t.Fatal("Response should have result object")
	}

	prompts, ok := result["prompts"].([]PromptDefinition)
	if !ok {
		t.Fatal("Result should have prompts array")
	}

	if len(prompts) < 9 {
		t.Errorf("Expected at least 9 prompts, got %d", len(prompts))
	}
}

func TestHandleGetPrompt(t *testing.T) {
	server := NewServer()

	// Test valid prompt request
	request := map[string]interface{}{
		"id":     2,
		"method": "prompts/get",
		"params": map[string]interface{}{
			"name": "quick-node-status",
			"arguments": map[string]interface{}{
				"work_dir": "/test/work",
			},
		},
	}

	response := server.handleGetPrompt(2, request)

	if response["jsonrpc"] != "2.0" {
		t.Error("Response should have jsonrpc 2.0")
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		t.Fatal("Response should have result object")
	}

	messages, ok := result["messages"].([]map[string]interface{})
	if !ok {
		t.Fatal("Result should have messages array")
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	// Check message structure
	if messages[0]["role"] != "user" {
		t.Error("Message role should be 'user'")
	}

	content, ok := messages[0]["content"].(map[string]interface{})
	if !ok {
		t.Fatal("Message should have content object")
	}

	if content["type"] != "text" {
		t.Error("Content type should be 'text'")
	}

	text, ok := content["text"].(string)
	if !ok {
		t.Fatal("Content should have text string")
	}

	if text == "" {
		t.Error("Text should not be empty")
	}
}

func TestHandleGetPromptMissingRequiredArgs(t *testing.T) {
	server := NewServer()

	// Test prompt request missing required arguments
	request := map[string]interface{}{
		"id":     3,
		"method": "prompts/get",
		"params": map[string]interface{}{
			"name": "deploy-follower-node",
			"arguments": map[string]interface{}{
				"work_dir": "/test/work",
				// Missing dn_database and bvn_database
			},
		},
	}

	response := server.handleGetPrompt(3, request)

	// Should return an error response
	errorObj, ok := response["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected error response")
	}

	if errorObj["code"] != -32602 {
		t.Errorf("Expected error code -32602, got %v", errorObj["code"])
	}
}

func TestPromptsEndToEnd(t *testing.T) {
	server := NewServer()

	// Test the full flow: list prompts, then get a specific prompt
	listResponse := server.handleListPrompts(100)

	result, ok := listResponse["result"].(map[string]interface{})
	if !ok {
		t.Fatal("List response should have result")
	}

	prompts, ok := result["prompts"].([]PromptDefinition)
	if !ok {
		t.Fatal("Result should have prompts array")
	}

	// Get the first prompt
	if len(prompts) == 0 {
		t.Fatal("Should have at least one prompt")
	}

	firstPrompt := prompts[0]

	// Build arguments for the prompt
	args := make(map[string]interface{})
	for _, arg := range firstPrompt.Arguments {
		if arg.Required {
			args[arg.Name] = "/test/value"
		}
	}

	// Get the prompt
	getRequest := map[string]interface{}{
		"id":     101,
		"method": "prompts/get",
		"params": map[string]interface{}{
			"name":      firstPrompt.Name,
			"arguments": args,
		},
	}

	getResponse := server.handleGetPrompt(101, getRequest)

	// Should succeed
	getResult, ok := getResponse["result"].(map[string]interface{})
	if !ok {
		// Check if it's an error
		if errObj, ok := getResponse["error"]; ok {
			t.Fatalf("Got error: %v", errObj)
		}
		t.Fatal("Get response should have result")
	}

	messages, ok := getResult["messages"].([]map[string]interface{})
	if !ok {
		t.Fatal("Result should have messages")
	}

	if len(messages) == 0 {
		t.Error("Should have at least one message")
	}
}

func TestPromptTemplateJSON(t *testing.T) {
	// Test that prompt definitions can be marshaled to JSON
	prompts := GetAllPrompts()

	data, err := json.Marshal(prompts)
	if err != nil {
		t.Fatalf("Failed to marshal prompts to JSON: %v", err)
	}

	// Unmarshal back
	var unmarshaled []PromptDefinition
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal prompts from JSON: %v", err)
	}

	if len(unmarshaled) != len(prompts) {
		t.Errorf("Unmarshaled prompts count mismatch: got %d, want %d", len(unmarshaled), len(prompts))
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
