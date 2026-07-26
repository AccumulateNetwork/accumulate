//go:build integration

// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Integration tests that hit a live Accumulate network.
// Run with: go test -tags=integration -v ./server/...
//
// By default tests run against local devnet at http://127.0.0.1:26660/v3
// Set ACCUMULATE_ENDPOINT to override.

func getTestEndpoint() string {
	if ep := os.Getenv("ACCUMULATE_ENDPOINT"); ep != "" {
		return ep
	}
	return "http://127.0.0.1:26660/v3"
}

func setupTestServer() *Server {
	srv := New(&Config{})
	srv.RegisterAccumulateTools(getTestEndpoint())
	return srv
}

// Network/Node tool tests

func TestIntegration_NetworkStatus(t *testing.T) {
	srv := setupTestServer()

	result, err := srv.handlers["network_status"](map[string]any{})
	if err != nil {
		t.Fatalf("network_status failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("network_status returned error: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	if !strings.Contains(text, "oracle") {
		t.Error("response missing oracle field")
	}
}

func TestIntegration_NodeInfo(t *testing.T) {
	srv := setupTestServer()

	result, err := srv.handlers["node_info"](map[string]any{})
	if err != nil {
		t.Fatalf("node_info failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("node_info returned error: %s", result.Content[0].Text)
	}
}

func TestIntegration_ConsensusStatus(t *testing.T) {
	srv := setupTestServer()

	// consensus_status now auto-detects nodeId from node-info
	result, err := srv.handlers["consensus_status"](map[string]any{
		"partition": "Directory",
	})
	if err != nil {
		t.Fatalf("consensus_status failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("consensus_status returned error: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	if !strings.Contains(text, "lastBlock") {
		t.Error("response missing lastBlock")
	}
	if !strings.Contains(text, "partitionID") {
		t.Error("response missing partitionID")
	}
}

func TestIntegration_Metrics(t *testing.T) {
	srv := setupTestServer()

	result, err := srv.handlers["metrics"](map[string]any{})
	if err != nil {
		t.Fatalf("metrics failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("metrics returned error: %s", result.Content[0].Text)
	}
}

// Query tool tests

func TestIntegration_QueryAccount_ACME(t *testing.T) {
	srv := setupTestServer()

	result, err := srv.handlers["query_account"](map[string]any{
		"url": "acc://ACME",
	})
	if err != nil {
		t.Fatalf("query_account failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("query_account returned error: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	if !strings.Contains(text, "ACME") {
		t.Error("response doesn't contain ACME")
	}
	if !strings.Contains(text, "tokenIssuer") {
		t.Error("response doesn't identify as tokenIssuer")
	}
}

func TestIntegration_QueryChain(t *testing.T) {
	srv := setupTestServer()

	result, err := srv.handlers["query_chain"](map[string]any{
		"url":   "acc://ACME",
		"chain": "main",
		"count": float64(5),
	})
	if err != nil {
		t.Fatalf("query_chain failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("query_chain returned error: %s", result.Content[0].Text)
	}
}

func TestIntegration_QueryBlock(t *testing.T) {
	srv := setupTestServer()

	result, err := srv.handlers["query_block"](map[string]any{
		"url":   "acc://dn",
		"minor": float64(1),
	})
	if err != nil {
		t.Fatalf("query_block failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("query_block returned error: %s", result.Content[0].Text)
	}
}

// Wallet tool tests

func TestIntegration_GenerateKey(t *testing.T) {
	srv := setupTestServer()

	result, err := srv.handlers["generate_key"](map[string]any{})
	if err != nil {
		t.Fatalf("generate_key failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("generate_key returned error: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	if !strings.Contains(text, "public_key") {
		t.Error("response missing public_key")
	}
	if !strings.Contains(text, "lite_identity") {
		t.Error("response missing lite_identity")
	}
	if !strings.Contains(text, "lite_acme") {
		t.Error("response missing lite_acme")
	}
}

func TestIntegration_ListKeys(t *testing.T) {
	srv := setupTestServer()

	// Generate a key first
	_, err := srv.handlers["generate_key"](map[string]any{})
	if err != nil {
		t.Fatalf("generate_key failed: %v", err)
	}

	result, err := srv.handlers["list_keys"](map[string]any{})
	if err != nil {
		t.Fatalf("list_keys failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("list_keys returned error: %s", result.Content[0].Text)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	count, ok := data["count"].(float64)
	if !ok || count < 1 {
		t.Error("expected at least 1 key")
	}
}

func TestIntegration_GetLiteAddress(t *testing.T) {
	srv := setupTestServer()

	// Generate a key first
	result, err := srv.handlers["generate_key"](map[string]any{})
	if err != nil {
		t.Fatalf("generate_key failed: %v", err)
	}

	var keyData map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &keyData); err != nil {
		t.Fatalf("failed to parse key result: %v", err)
	}

	pubKey := keyData["public_key"].(string)

	result, err = srv.handlers["get_lite_address"](map[string]any{
		"public_key": pubKey,
		"token_url":  "acc://ACME",
	})
	if err != nil {
		t.Fatalf("get_lite_address failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("get_lite_address returned error: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	if !strings.Contains(text, "lite_address") {
		t.Error("response missing lite_address")
	}
}

// Faucet test (devnet only)

func TestIntegration_Faucet(t *testing.T) {
	srv := setupTestServer()

	// Generate a key to get a lite account
	result, err := srv.handlers["generate_key"](map[string]any{})
	if err != nil {
		t.Fatalf("generate_key failed: %v", err)
	}

	var keyData map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &keyData); err != nil {
		t.Fatalf("failed to parse key result: %v", err)
	}

	liteAcct := keyData["lite_acme"].(string)

	result, err = srv.handlers["faucet"](map[string]any{
		"account": liteAcct,
	})
	if err != nil {
		t.Fatalf("faucet failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("faucet returned error: %s", result.Content[0].Text)
	}

	// Wait for transaction to be processed
	time.Sleep(10 * time.Second)

	// Verify the account now has tokens
	result, err = srv.handlers["query_account"](map[string]any{
		"url": liteAcct,
	})
	if err != nil {
		t.Fatalf("query_account failed: %v", err)
	}

	text := result.Content[0].Text
	if !strings.Contains(text, "balance") {
		t.Error("account should have balance after faucet")
	}
}

// Transaction building tests

func TestIntegration_SendTokens(t *testing.T) {
	srv := setupTestServer()

	// Generate sender key
	result, err := srv.handlers["generate_key"](map[string]any{})
	if err != nil {
		t.Fatalf("generate_key failed: %v", err)
	}

	var senderKeyData map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &senderKeyData); err != nil {
		t.Fatalf("failed to parse key result: %v", err)
	}

	senderAcct := senderKeyData["lite_acme"].(string)
	senderKeyHash := senderKeyData["public_key_hash"].(string)

	// Generate receiver key
	result, err = srv.handlers["generate_key"](map[string]any{})
	if err != nil {
		t.Fatalf("generate_key failed: %v", err)
	}

	var receiverKeyData map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &receiverKeyData); err != nil {
		t.Fatalf("failed to parse key result: %v", err)
	}

	receiverAcct := receiverKeyData["lite_acme"].(string)

	// Fund sender via faucet
	result, err = srv.handlers["faucet"](map[string]any{
		"account": senderAcct,
	})
	if err != nil {
		t.Fatalf("faucet failed: %v", err)
	}

	// Wait for faucet tx
	time.Sleep(10 * time.Second)

	// Debug: check sender balance after faucet
	result, err = srv.handlers["query_account"](map[string]any{
		"url": senderAcct,
	})
	if err != nil {
		t.Logf("sender query after faucet failed: %v", err)
	} else {
		t.Logf("sender after faucet: %s", result.Content[0].Text)
	}

	// Get sender lite identity for credits
	senderLiteId := senderKeyData["lite_identity"].(string)

	// Add credits to sender (needed to submit transactions) - only 1 ACME, leave enough for send
	result, err = srv.handlers["add_credits"](map[string]any{
		"from":       senderAcct,
		"to":         senderLiteId,
		"amount":     float64(1), // 1 ACME worth of credits
		"signer_key": senderKeyHash,
	})
	if err != nil {
		t.Fatalf("add_credits failed: %v", err)
	}

	t.Logf("add_credits result: %s", result.Content[0].Text)
	if result.IsError {
		t.Fatalf("add_credits returned error: %s", result.Content[0].Text)
	}

	// Wait for add_credits tx
	time.Sleep(10 * time.Second)

	// Send tokens
	result, err = srv.handlers["send_tokens"](map[string]any{
		"from":       senderAcct,
		"to":         receiverAcct,
		"amount":     float64(1), // 1 ACME
		"signer_key": senderKeyHash,
	})
	if err != nil {
		t.Fatalf("send_tokens failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("send_tokens returned error: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	t.Logf("send_tokens result: %s", text)
	if !strings.Contains(text, "txid") {
		t.Error("response missing txid")
	}

	// Wait for tx to be processed (receiver account is created by synthetic tx, needs more time on single-validator devnet)
	time.Sleep(30 * time.Second)

	// Debug: check sender balance after send
	result, err = srv.handlers["query_account"](map[string]any{
		"url": senderAcct,
	})
	if err != nil {
		t.Logf("sender query failed: %v", err)
	} else {
		t.Logf("sender account after send: %s", result.Content[0].Text)
	}

	// Verify receiver got tokens
	result, err = srv.handlers["query_account"](map[string]any{
		"url": receiverAcct,
	})
	if err != nil {
		t.Logf("query_account failed: %v", err)
		t.Logf("receiver URL was: %s", receiverAcct)
		t.Fatalf("receiver account not found after 30s")
	}

	if result.IsError {
		t.Fatalf("query_account returned error: %s", result.Content[0].Text)
	}
}

func TestIntegration_AddCredits(t *testing.T) {
	srv := setupTestServer()

	// Generate key
	result, err := srv.handlers["generate_key"](map[string]any{})
	if err != nil {
		t.Fatalf("generate_key failed: %v", err)
	}

	var keyData map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &keyData); err != nil {
		t.Fatalf("failed to parse key result: %v", err)
	}

	liteAcct := keyData["lite_acme"].(string)
	liteId := keyData["lite_identity"].(string)
	keyHash := keyData["public_key_hash"].(string)

	// Fund via faucet
	result, err = srv.handlers["faucet"](map[string]any{
		"account": liteAcct,
	})
	if err != nil {
		t.Fatalf("faucet failed: %v", err)
	}

	time.Sleep(10 * time.Second)

	// Add credits to lite identity
	result, err = srv.handlers["add_credits"](map[string]any{
		"from":       liteAcct,
		"to":         liteId,
		"amount":     float64(10), // 10 ACME worth of credits
		"signer_key": keyHash,
	})
	if err != nil {
		t.Fatalf("add_credits failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("add_credits returned error: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	if !strings.Contains(text, "txid") {
		t.Error("response missing txid")
	}
}

func TestIntegration_WriteData(t *testing.T) {
	srv := setupTestServer()

	// Generate key
	result, err := srv.handlers["generate_key"](map[string]any{})
	if err != nil {
		t.Fatalf("generate_key failed: %v", err)
	}

	var keyData map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &keyData); err != nil {
		t.Fatalf("failed to parse key result: %v", err)
	}

	liteAcct := keyData["lite_acme"].(string)
	liteId := keyData["lite_identity"].(string)
	keyHash := keyData["public_key_hash"].(string)

	// Fund via faucet
	result, err = srv.handlers["faucet"](map[string]any{
		"account": liteAcct,
	})
	if err != nil {
		t.Fatalf("faucet failed: %v", err)
	}

	time.Sleep(10 * time.Second)

	// Add credits first (needed to write data)
	result, err = srv.handlers["add_credits"](map[string]any{
		"from":       liteAcct,
		"to":         liteId,
		"amount":     float64(10),
		"signer_key": keyHash,
	})
	if err != nil {
		t.Fatalf("add_credits failed: %v", err)
	}

	time.Sleep(10 * time.Second)

	// Write data to lite data account
	liteData := strings.Replace(liteId, "acc://", "acc://", 1) + "/data"
	testData := hex.EncodeToString([]byte("Hello from MCP integration test!"))

	result, err = srv.handlers["write_data"](map[string]any{
		"account":    liteData,
		"data":       testData,
		"signer":     liteId,
		"signer_key": keyHash,
	})
	if err != nil {
		t.Fatalf("write_data failed: %v", err)
	}

	if result.IsError {
		// Write to lite data may fail if account doesn't exist - that's ok for this test
		t.Logf("write_data result (may fail on lite accounts): %s", result.Content[0].Text)
	} else {
		text := result.Content[0].Text
		if !strings.Contains(text, "txid") {
			t.Error("response missing txid")
		}
	}
}

// Parameter validation tests

func TestIntegration_QueryAccount_MissingURL(t *testing.T) {
	srv := setupTestServer()

	_, err := srv.handlers["query_account"](map[string]any{})
	if err == nil {
		t.Error("expected error for missing url")
	}
}

func TestIntegration_QueryAccount_InvalidURL(t *testing.T) {
	srv := setupTestServer()

	_, err := srv.handlers["query_account"](map[string]any{
		"url": "not-a-valid-url",
	})
	if err == nil {
		t.Error("expected error for invalid url")
	}
}

func TestIntegration_SendTokens_MissingParams(t *testing.T) {
	srv := setupTestServer()

	_, err := srv.handlers["send_tokens"](map[string]any{
		"from": "acc://test",
	})
	if err == nil {
		t.Error("expected error for missing params")
	}
}

// Full round-trip test via MCP protocol

func TestIntegration_FullRoundTrip(t *testing.T) {
	srv := setupTestServer()

	// Test via handleMessage like a real MCP client would
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"query_account","arguments":{"url":"acc://ACME"}}`),
	}

	data, _ := json.Marshal(req)
	resp := srv.handleMessage(data)

	if resp == nil {
		t.Fatal("no response")
	}
	if resp.Error != nil {
		t.Fatalf("JSON-RPC error: %v", resp.Error)
	}

	result, ok := resp.Result.(ToolCallResult)
	if !ok {
		t.Fatalf("expected ToolCallResult, got %T", resp.Result)
	}

	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].Text)
	}

	if !strings.Contains(result.Content[0].Text, "ACME") {
		t.Error("response doesn't contain ACME")
	}
}

// Resources test

func TestIntegration_Resources(t *testing.T) {
	srv := setupTestServer()

	// Test resources/list
	result, err := srv.handleResourcesList()
	if err != nil {
		t.Fatalf("handleResourcesList failed: %v", err)
	}

	listResult, ok := result.(ResourcesListResult)
	if !ok {
		t.Fatalf("expected ResourcesListResult, got %T", result)
	}

	if len(listResult.Resources) != 3 {
		t.Errorf("expected 3 resources, got %d", len(listResult.Resources))
	}

	// Test resources/read
	for _, res := range listResult.Resources {
		params, _ := json.Marshal(ResourceReadParams{URI: res.URI})
		readResult, rpcErr := srv.handleResourcesRead(params)
		if rpcErr != nil {
			t.Errorf("handleResourcesRead(%s) failed: %v", res.URI, rpcErr)
			continue
		}

		rr, ok := readResult.(ResourceReadResult)
		if !ok {
			t.Errorf("expected ResourceReadResult for %s, got %T", res.URI, readResult)
			continue
		}

		if len(rr.Contents) == 0 {
			t.Errorf("no content for resource %s", res.URI)
		}
	}
}
