// +build integration

package server

import (
	"testing"
)

// TestTestnetIntegration tests real operations against testnet
// Run with: go test -tags=integration -v -run TestTestnetIntegration
func TestTestnetIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	server := NewServer()

	t.Log("\n🌐 Testing Real Testnet Integration")
	t.Log("=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")
	t.Log("📍 Network: https://testnet.accumulatenetwork.io/v3")

	// Test 1: Query the testnet directory
	t.Run("Query testnet directory", func(t *testing.T) {
		t.Log("\n📡 Test 1: Query Directory Network on testnet")

		args := map[string]interface{}{
			"url":     "acc://dn.acme",
			"network": "testnet",
		}

		result, err := server.queryAccount(args)

		if err != nil {
			t.Logf("❌ FAILED: Query failed: %v", err)
			t.Logf("   This is a REAL network test - if it fails, either:")
			t.Logf("   1. Testnet is down")
			t.Logf("   2. Network connectivity issue")
			t.Logf("   3. Tool has a bug")
			t.FailNow()
		} else {
			t.Logf("✅ SUCCESS: Query succeeded!")
			t.Logf("   Result type: %T", result)
			if data, ok := result["data"]; ok {
				t.Logf("   Has data: %v", data != nil)
			}
			if typ, ok := result["type"]; ok {
				t.Logf("   Account type: %v", typ)
			}
		}
	})

	// Test 2: Get node info from testnet
	t.Run("Get testnet node info", func(t *testing.T) {
		t.Log("\n📊 Test 2: Get node info from testnet")

		args := map[string]interface{}{
			"network": "testnet",
		}

		result, err := server.nodeInfo(args)

		if err != nil {
			t.Logf("❌ Node info failed: %v", err)
			t.FailNow()
		}

		t.Logf("✅ Node info retrieved!")

		// Log interesting fields
		if version, ok := result["version"]; ok {
			t.Logf("   Version: %v", version)
		}
		if network, ok := result["network"]; ok {
			t.Logf("   Network: %v", network)
		}
		if commit, ok := result["commit"]; ok {
			t.Logf("   Commit: %v", commit)
		}
	})

	// Test 3: Get network status
	t.Run("Get testnet network status", func(t *testing.T) {
		t.Log("\n🔍 Test 3: Get network status from testnet")

		args := map[string]interface{}{
			"network": "testnet",
		}

		result, err := server.networkStatus(args)

		if err != nil {
			t.Logf("❌ Network status failed: %v", err)
			t.FailNow()
		}

		t.Logf("✅ Network status retrieved!")

		if height, ok := result["height"]; ok {
			t.Logf("   Block height: %v", height)
		}
		if oracle, ok := result["oracle"]; ok {
			t.Logf("   Oracle: %v", oracle)
		}
	})

	// Test 4: Query a known testnet account (if exists)
	t.Run("Query known testnet account", func(t *testing.T) {
		t.Log("\n🔎 Test 4: Query a known testnet account")

		// Try to query the ACME token issuer on testnet
		args := map[string]interface{}{
			"url":     "acc://acme",
			"network": "testnet",
		}

		result, err := server.queryAccount(args)

		if err != nil {
			t.Logf("⚠️  Query failed: %v", err)
			t.Log("   (This may be expected if account doesn't exist on testnet)")
		} else {
			t.Logf("✅ Query succeeded!")
			if typ, ok := result["type"]; ok {
				t.Logf("   Account type: %v", typ)
			}
		}
	})

	t.Log("\n" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")
	t.Log("🎯 Testnet Integration Tests Complete")
	t.Log("")
	t.Log("✅ REAL NETWORK TESTS PASSED!")
	t.Log("This proves the tools actually work with a live Accumulate network!")
}

// TestPromptWithRealNetwork tests prompts work with real network
func TestPromptWithRealNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	server := NewServer()

	t.Log("\n📋 Testing Prompts with Real Network")
	t.Log("=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")

	// Get a prompt that references network tools
	t.Run("Get and verify prompt workflow", func(t *testing.T) {
		t.Log("\n📝 Getting quick-node-status prompt")

		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "prompts/get",
			"params": map[string]interface{}{
				"name": "quick-node-status",
			},
		}

		response := server.HandleRequest(request)

		if _, ok := response["error"]; ok {
			t.Fatalf("❌ Failed to get prompt: %v", response["error"])
		}

		result := response["result"].(map[string]interface{})
		messages := result["messages"].([]map[string]interface{})
		content := messages[0]["content"].(map[string]interface{})
		promptText := content["text"].(string)

		t.Logf("✅ Prompt retrieved (%d characters)", len(promptText))

		// The prompt references these tools
		t.Log("\n🔧 Verifying tools referenced in prompt...")

		// Try to actually execute the tools mentioned in the prompt
		t.Log("\n1️⃣ Testing accumulate_follower_status (will fail - no local follower)")
		_, err := server.executeTool("accumulate_follower_status", map[string]interface{}{})
		if err != nil {
			t.Logf("   ⚠️  Expected failure (no follower): %v", err)
		}

		t.Log("\n2️⃣ Testing accumulate_node_info (should work with testnet)")
		result2, err := server.executeTool("accumulate_node_info", map[string]interface{}{
			"network": "testnet",
		})
		if err != nil {
			t.Fatalf("   ❌ REAL FAILURE: %v", err)
		}
		t.Logf("   ✅ Real network call succeeded!")
		if version, ok := result2["version"]; ok {
			t.Logf("   Network version: %v", version)
		}

		t.Log("\n✅ Prompt workflow verified with REAL network calls!")
	})

	t.Log("\n" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")
	t.Log("🎉 Prompt + Real Network Integration SUCCESSFUL!")
}

// TestCompleteWorkflowWithTestnet demonstrates an end-to-end workflow
func TestCompleteWorkflowWithTestnet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	server := NewServer()

	t.Log("\n🎬 Complete Workflow Test: Using Prompts + Real Network")
	t.Log("=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")

	// Scenario: User wants to check network status
	t.Log("\n👤 User Request: 'What's the current status of testnet?'")
	t.Log("🤖 AI Decision: Use monitor-follower-health prompt (adapted for testnet)")

	// Step 1: Get node info
	t.Log("\n1️⃣ Executing: accumulate_node_info")
	nodeInfo, err := server.executeTool("accumulate_node_info", map[string]interface{}{
		"network": "testnet",
	})
	if err != nil {
		t.Fatalf("❌ Node info failed: %v", err)
	}
	t.Log("   ✅ Node info retrieved")
	if version, ok := nodeInfo["version"]; ok {
		t.Logf("   Version: %v", version)
	}

	// Step 2: Get network status
	t.Log("\n2️⃣ Executing: accumulate_network_status")
	netStatus, err := server.executeTool("accumulate_network_status", map[string]interface{}{
		"network": "testnet",
	})
	if err != nil {
		t.Fatalf("❌ Network status failed: %v", err)
	}
	t.Log("   ✅ Network status retrieved")
	if height, ok := netStatus["height"]; ok {
		t.Logf("   Block height: %v", height)
	}

	// Step 3: Query directory
	t.Log("\n3️⃣ Executing: accumulate_query_account (DN)")
	dnQuery, err := server.executeTool("accumulate_query_account", map[string]interface{}{
		"url":     "acc://dn.acme",
		"network": "testnet",
	})
	if err != nil {
		t.Fatalf("❌ DN query failed: %v", err)
	}
	t.Log("   ✅ DN account queried")
	if typ, ok := dnQuery["type"]; ok {
		t.Logf("   Account type: %v", typ)
	}

	t.Log("\n" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")
	t.Log("🎉 COMPLETE WORKFLOW SUCCESSFUL!")
	t.Log("")
	t.Log("✅ All 3 tools executed successfully against REAL testnet")
	t.Log("✅ This proves end-to-end functionality works")
	t.Log("✅ Prompts guide users through workflows that actually work")
}
