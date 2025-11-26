package server

import (
	"testing"
)

// TestMCPServerIntegration verifies that all MCP capabilities work together
func TestMCPServerIntegration(t *testing.T) {
	server := NewServer()

	t.Run("Initialize advertises all capabilities", func(t *testing.T) {
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

		response := server.HandleRequest(request)

		result, ok := response["result"].(map[string]interface{})
		if !ok {
			t.Fatal("expected result object")
		}

		capabilities, ok := result["capabilities"].(map[string]interface{})
		if !ok {
			t.Fatal("expected capabilities object")
		}

		// Verify all three capabilities are advertised
		if _, ok := capabilities["tools"]; !ok {
			t.Error("tools capability not advertised")
		}
		if _, ok := capabilities["resources"]; !ok {
			t.Error("resources capability not advertised")
		}
		if _, ok := capabilities["prompts"]; !ok {
			t.Error("prompts capability not advertised")
		}

		t.Logf("✅ All capabilities advertised: tools, resources, prompts")
	})

	t.Run("Tools are available", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/list",
		}

		response := server.HandleRequest(request)

		result, ok := response["result"].(map[string]interface{})
		if !ok {
			t.Fatal("expected result object")
		}

		tools, ok := result["tools"].([]map[string]interface{})
		if !ok {
			t.Fatal("expected tools array")
		}

		if len(tools) == 0 {
			t.Error("no tools found")
		}

		t.Logf("✅ Tools available: %d tools", len(tools))

		// Verify some key tools exist
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			if name, ok := tool["name"].(string); ok {
				toolNames[name] = true
			}
		}

		keyTools := []string{
			"accumulate_query_account",
			"accumulate_send_tokens",
			"wallet_init",
			"accumulate_init_follower",
			"accumulate_run_follower",
		}

		for _, keyTool := range keyTools {
			if !toolNames[keyTool] {
				t.Errorf("key tool %s not found", keyTool)
			}
		}

		t.Logf("✅ All key tools present")
	})

	t.Run("Resources are available", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "resources/list",
		}

		response := server.HandleRequest(request)

		result, ok := response["result"].(map[string]interface{})
		if !ok {
			t.Fatal("expected result object")
		}

		resources, ok := result["resources"].([]map[string]interface{})
		if !ok {
			t.Fatal("expected resources array")
		}

		if len(resources) == 0 {
			t.Error("no resources found")
		}

		t.Logf("✅ Resources available: %d resources", len(resources))

		// Verify resources have required fields
		for i, resource := range resources {
			if _, ok := resource["uri"]; !ok {
				t.Errorf("resource %d missing uri field", i)
			}
			if _, ok := resource["name"]; !ok {
				t.Errorf("resource %d missing name field", i)
			}
		}

		t.Logf("✅ All resources have required fields")
	})

	t.Run("Prompts are available", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      4,
			"method":  "prompts/list",
		}

		response := server.HandleRequest(request)

		result, ok := response["result"].(map[string]interface{})
		if !ok {
			t.Fatal("expected result object")
		}

		prompts, ok := result["prompts"].([]PromptDefinition)
		if !ok {
			t.Fatal("expected prompts array")
		}

		if len(prompts) < 9 {
			t.Errorf("expected at least 9 prompts, got %d", len(prompts))
		}

		t.Logf("✅ Prompts available: %d prompts", len(prompts))

		// Verify all designed prompts exist
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
			if _, exists := expectedPrompts[prompt.Name]; exists {
				expectedPrompts[prompt.Name] = true
			}
		}

		for name, found := range expectedPrompts {
			if !found {
				t.Errorf("expected prompt %s not found", name)
			}
		}

		t.Logf("✅ All %d designed prompts present", len(expectedPrompts))
	})

	t.Run("Can retrieve a specific prompt", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      5,
			"method":  "prompts/get",
			"params": map[string]interface{}{
				"name": "quick-node-status",
				"arguments": map[string]interface{}{
					"work_dir": "/test/follower",
				},
			},
		}

		response := server.HandleRequest(request)

		result, ok := response["result"].(map[string]interface{})
		if !ok {
			// Check if there's an error
			if errObj, ok := response["error"]; ok {
				t.Fatalf("unexpected error: %v", errObj)
			}
			t.Fatal("expected result object")
		}

		messages, ok := result["messages"].([]map[string]interface{})
		if !ok {
			t.Fatal("expected messages array")
		}

		if len(messages) == 0 {
			t.Error("no messages in prompt result")
		}

		// Check message structure
		if len(messages) > 0 {
			msg := messages[0]
			if role, ok := msg["role"].(string); !ok || role != "user" {
				t.Error("message should have role='user'")
			}

			content, ok := msg["content"].(map[string]interface{})
			if !ok {
				t.Fatal("expected content object")
			}

			text, ok := content["text"].(string)
			if !ok {
				t.Fatal("expected text in content")
			}

			if text == "" {
				t.Error("prompt text should not be empty")
			}

			if len(text) < 50 {
				t.Error("prompt text seems too short")
			}

			t.Logf("✅ Prompt retrieved successfully (%d characters)", len(text))
		}
	})

	t.Run("Invalid method returns error", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      6,
			"method":  "invalid/method",
		}

		response := server.HandleRequest(request)

		if _, ok := response["error"]; !ok {
			t.Error("expected error for invalid method")
		}

		t.Logf("✅ Invalid methods properly rejected")
	})

	t.Run("All MCP methods work", func(t *testing.T) {
		methods := []string{
			"initialize",
			"tools/list",
			"resources/list",
			"prompts/list",
		}

		for _, method := range methods {
			request := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      100,
				"method":  method,
			}

			if method == "initialize" {
				request["params"] = map[string]interface{}{
					"protocolVersion": "2024-11-05",
				}
			}

			response := server.HandleRequest(request)

			if _, ok := response["error"]; ok {
				t.Errorf("method %s returned error", method)
			}

			if _, ok := response["result"]; !ok {
				t.Errorf("method %s missing result", method)
			}
		}

		t.Logf("✅ All MCP methods working correctly")
	})

	t.Log("\n🎉 COMPREHENSIVE VERIFICATION COMPLETE")
	t.Log("✅ Tools: Working")
	t.Log("✅ Resources: Working")
	t.Log("✅ Prompts: Working")
	t.Log("✅ All capabilities integrated correctly")
}

// TestPromptToolIntegration verifies prompts reference valid tools
func TestPromptToolIntegration(t *testing.T) {
	server := NewServer()

	// Get all tools
	toolsRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}
	toolsResponse := server.HandleRequest(toolsRequest)
	toolsResult := toolsResponse["result"].(map[string]interface{})
	tools := toolsResult["tools"].([]map[string]interface{})

	// Build set of tool names
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		if name, ok := tool["name"].(string); ok {
			toolNames[name] = true
		}
	}

	// Get a follower deployment prompt
	promptRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "prompts/get",
		"params": map[string]interface{}{
			"name": "deploy-follower-node",
			"arguments": map[string]interface{}{
				"dn_database":  "/snapshots/dn",
				"bvn_database": "/snapshots/bvn",
				"work_dir":     "/follower",
			},
		},
	}

	promptResponse := server.HandleRequest(promptRequest)
	promptResult := promptResponse["result"].(map[string]interface{})
	messages := promptResult["messages"].([]map[string]interface{})

	if len(messages) > 0 {
		content := messages[0]["content"].(map[string]interface{})
		text := content["text"].(string)

		// Verify prompt references valid tools - these are the tools actually used in deploy-follower-node
		referencedTools := []string{
			"accumulate_init_follower",
			"accumulate_run_follower",
			"accumulate_follower_status",
			"accumulate_validate_prerequisites",
			"accumulate_get_sync_progress",
			"accumulate_analyze_logs",
		}

		for _, toolName := range referencedTools {
			if !toolNames[toolName] {
				t.Errorf("prompt references non-existent tool: %s", toolName)
			}
		}

		// Verify prompt actually contains these tool names
		for _, toolName := range referencedTools {
			if !contains(text, toolName) {
				t.Errorf("prompt should reference tool: %s", toolName)
			}
		}

		t.Logf("✅ Prompts correctly reference existing tools")
	}
}
