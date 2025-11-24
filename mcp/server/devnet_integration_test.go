// +build integration

package server

import (
	"testing"
)

// TestDevnetIntegration tests real operations against devnet
// Run with: go test -tags=integration -v -run TestDevnetIntegration
func TestDevnetIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	server := NewServer()

	t.Log("🌐 Testing Real Devnet Integration")
	t.Log("=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")

	// Test 1: Query a known devnet account
	t.Run("Query devnet directory", func(t *testing.T) {
		t.Log("\n📡 Test 1: Query Directory Network on devnet")

		args := map[string]interface{}{
			"url":     "acc://dn.acme",
			"network": "devnet",
		}

		result, err := server.queryAccount(args)

		if err != nil {
			t.Logf("⚠️  Query failed (expected if devnet is down): %v", err)
			// Don't fail the test - devnet might be down
		} else {
			t.Logf("✅ Query succeeded!")
			t.Logf("   Result type: %T", result)
			if data, ok := result["data"]; ok {
				t.Logf("   Has data: %v", data != nil)
			}
		}
	})

	// Test 2: Node info
	t.Run("Get devnet node info", func(t *testing.T) {
		t.Log("\n📊 Test 2: Get node info from devnet")

		args := map[string]interface{}{
			"network": "devnet",
		}

		result, err := server.nodeInfo(args)

		if err != nil {
			t.Logf("⚠️  Node info failed (expected if devnet is down): %v", err)
		} else {
			t.Logf("✅ Node info retrieved!")
			if version, ok := result["version"]; ok {
				t.Logf("   Version: %v", version)
			}
			if network, ok := result["network"]; ok {
				t.Logf("   Network: %v", network)
			}
		}
	})

	// Test 3: Network status
	t.Run("Get devnet network status", func(t *testing.T) {
		t.Log("\n🔍 Test 3: Get network status from devnet")

		args := map[string]interface{}{
			"network": "devnet",
		}

		result, err := server.networkStatus(args)

		if err != nil {
			t.Logf("⚠️  Network status failed: %v", err)
		} else {
			t.Logf("✅ Network status retrieved!")
			if height, ok := result["height"]; ok {
				t.Logf("   Block height: %v", height)
			}
		}
	})

	// Test 4: Create lite account URL
	t.Run("Create lite account", func(t *testing.T) {
		t.Log("\n🔑 Test 4: Create lite account URL")

		// Test public key
		args := map[string]interface{}{
			"public_key": "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
		}

		result, err := server.createLiteAccount(args)

		if err != nil {
			t.Fatalf("❌ Create lite account failed: %v", err)
		}

		t.Logf("✅ Lite account created!")
		if url, ok := result["lite_account_url"]; ok {
			t.Logf("   URL: %v", url)
		}
	})

	t.Log("\n" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")
	t.Log("🎯 Devnet Integration Tests Complete")
}

// TestDevnetPromptWorkflow tests a complete prompt workflow against devnet
func TestDevnetPromptWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	server := NewServer()

	t.Log("\n📋 Testing Prompt Workflow Against Devnet")
	t.Log("=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")

	// Step 1: Get the setup-dev-wallet prompt for devnet
	t.Run("Get setup-dev-wallet prompt", func(t *testing.T) {
		t.Log("\n📝 Step 1: Retrieve setup-dev-wallet prompt for devnet")

		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "prompts/get",
			"params": map[string]interface{}{
				"name": "setup-dev-wallet",
				"arguments": map[string]interface{}{
					"network": "devnet",
				},
			},
		}

		response := server.HandleRequest(request)

		if _, ok := response["error"]; ok {
			t.Fatalf("❌ Failed to get prompt: %v", response["error"])
		}

		result := response["result"].(map[string]interface{})
		messages := result["messages"].([]map[string]interface{})

		if len(messages) == 0 {
			t.Fatal("❌ No messages in prompt")
		}

		content := messages[0]["content"].(map[string]interface{})
		text := content["text"].(string)

		t.Logf("✅ Prompt retrieved (%d characters)", len(text))
		t.Logf("   Contains 'devnet': %v", contains(text, "devnet"))
		t.Logf("   Contains 'wallet_init': %v", contains(text, "wallet_init"))
		t.Logf("   Contains 'wallet_generate_key': %v", contains(text, "wallet_generate_key"))
	})

	// Step 2: Verify tools referenced in prompt exist
	t.Run("Verify prompt tools exist", func(t *testing.T) {
		t.Log("\n🔧 Step 2: Verify all tools referenced in prompt")

		// Get all tools
		toolsRequest := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/list",
		}

		toolsResponse := server.HandleRequest(toolsRequest)
		toolsResult := toolsResponse["result"].(map[string]interface{})
		tools := toolsResult["tools"].([]map[string]interface{})

		// Build set of tool names
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool["name"].(string)] = true
		}

		// Tools that should be in the setup-dev-wallet prompt
		requiredTools := []string{
			"wallet_init",
			"wallet_vault_open",
			"wallet_generate_key",
			"wallet_set_network",
			"accumulate_create_lite_account",
			"accumulate_faucet",
			"wallet_get_status",
		}

		allExist := true
		for _, toolName := range requiredTools {
			if toolNames[toolName] {
				t.Logf("   ✅ %s - exists", toolName)
			} else {
				t.Errorf("   ❌ %s - MISSING", toolName)
				allExist = false
			}
		}

		if allExist {
			t.Log("✅ All required tools exist")
		}
	})

	t.Log("\n" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")
	t.Log("🎯 Prompt Workflow Tests Complete")
}

// TestDevnetToolsAvailability checks which tools work with devnet
func TestDevnetToolsAvailability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	server := NewServer()

	t.Log("\n🔧 Testing Tool Availability on Devnet")
	t.Log("=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")

	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
		critical bool // If true, failure is serious
	}{
		{
			name:     "Query DN account",
			toolName: "accumulate_query_account",
			args: map[string]interface{}{
				"url":     "acc://dn.acme",
				"network": "devnet",
			},
			critical: true,
		},
		{
			name:     "Node info",
			toolName: "accumulate_node_info",
			args: map[string]interface{}{
				"network": "devnet",
			},
			critical: true,
		},
		{
			name:     "Network status",
			toolName: "accumulate_network_status",
			args: map[string]interface{}{
				"network": "devnet",
			},
			critical: true,
		},
		{
			name:     "Create lite account",
			toolName: "accumulate_create_lite_account",
			args: map[string]interface{}{
				"public_key": "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
			},
			critical: false,
		},
	}

	results := make(map[string]bool)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := server.executeTool(tt.toolName, tt.args)

			if err != nil {
				if tt.critical {
					t.Logf("⚠️  CRITICAL: %s failed: %v", tt.name, err)
				} else {
					t.Logf("ℹ️  %s failed (may be expected): %v", tt.name, err)
				}
				results[tt.toolName] = false
			} else {
				t.Logf("✅ %s succeeded", tt.name)
				results[tt.toolName] = true

				// Log some result data
				if result != nil {
					t.Logf("   Result keys: %v", getKeys(result))
				}
			}
		})
	}

	// Summary
	t.Log("\n📊 Tool Availability Summary:")
	working := 0
	total := len(results)
	for tool, works := range results {
		if works {
			working++
			t.Logf("   ✅ %s", tool)
		} else {
			t.Logf("   ❌ %s", tool)
		}
	}

	t.Logf("\n🎯 %d/%d tools working with devnet", working, total)

	if working == 0 {
		t.Error("⚠️  WARNING: No tools are working with devnet - network may be down or unreachable")
	}
}

// Helper function to get keys from a map
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
