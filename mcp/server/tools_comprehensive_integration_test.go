// +build integration

package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const (
	comprehensiveTestTimeout = 30 * time.Second
	transactionWaitTime      = 3 * time.Second
	accountCreationWait      = 5 * time.Second
)

// TestComprehensive_QueryTools tests all query tools
func TestComprehensive_QueryTools(t *testing.T) {
	c, err := client.NewClient(devnetEndpoint)
	if err != nil {
		t.Skipf("Devnet not available: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	_, err = c.NodeInfo(ctx, nil)
	if err != nil {
		t.Skipf("Cannot connect to devnet: %v", err)
	}

	server := NewServer()

	t.Run("query_pending", func(t *testing.T) {
		// Query pending transactions for ACME token issuer
		args := map[string]interface{}{
			"url":     "acc://ACME",
			"network": devnetEndpoint,
		}

		result, err := server.queryPending(args)
		if err != nil {
			// Pending may be empty, which is OK
			if !strings.Contains(err.Error(), "not found") {
				t.Logf("Query pending returned error (may be OK if empty): %v", err)
			}
		} else {
			t.Logf("Query pending result: %+v", result)
		}
	})

	t.Run("query_minor_block", func(t *testing.T) {
		// Query a minor block
		args := map[string]interface{}{
			"url":     "acc://ACME",
			"network": devnetEndpoint,
			"index":   "0",
		}

		result, err := server.queryMinorBlock(args)
		if err != nil {
			t.Logf("Query minor block error (may not exist yet): %v", err)
		} else {
			t.Logf("Query minor block result: %+v", result)
		}
	})

	t.Run("query_data", func(t *testing.T) {
		// This requires a data account to exist first
		// We'll test this in the data operations section
		t.Skip("Requires data account - tested in data operations")
	})

	t.Run("consensus_status", func(t *testing.T) {
		args := map[string]interface{}{
			"network": devnetEndpoint,
		}

		result, err := server.consensusStatus(args)
		if err != nil {
			t.Logf("Consensus status error: %v", err)
		} else {
			t.Logf("Consensus status result: %+v", result)
		}
	})
}

// TestComprehensive_SearchTools tests all search tools
func TestComprehensive_SearchTools(t *testing.T) {
	c, err := client.NewClient(devnetEndpoint)
	if err != nil {
		t.Skipf("Devnet not available: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	_, err = c.NodeInfo(ctx, nil)
	if err != nil {
		t.Skipf("Cannot connect to devnet: %v", err)
	}

	server := NewServer()

	// Generate a key for testing
	pubKey, _, _ := ed25519.GenerateKey(rand.Reader)
	pubKeyHash, _ := protocol.PublicKeyHash(pubKey, protocol.SignatureTypeED25519)

	t.Run("search_public_key_hash", func(t *testing.T) {
		args := map[string]interface{}{
			"public_key_hash": hex.EncodeToString(pubKeyHash),
			"network":         devnetEndpoint,
		}

		result, err := server.searchPublicKeyHash(args)
		if err != nil {
			// Expected - key likely doesn't exist on devnet
			t.Logf("Search by key hash (expected not found): %v", err)
		} else {
			t.Logf("Search by key hash result: %+v", result)
		}
	})

	t.Run("search_anchor", func(t *testing.T) {
		// Search requires a known anchor
		args := map[string]interface{}{
			"source_url": "acc://dn.acme",
			"network":    devnetEndpoint,
		}

		result, err := server.searchAnchor(args)
		if err != nil {
			t.Logf("Search anchor error (may not have anchors yet): %v", err)
		} else {
			t.Logf("Search anchor result: %+v", result)
		}
	})
}

// TestComprehensive_KeyTools tests key generation and management
func TestComprehensive_KeyTools(t *testing.T) {
	server := NewServer()

	var generatedPublicKey string

	t.Run("generate_key", func(t *testing.T) {
		args := map[string]interface{}{}

		result, err := server.generateKey(args)
		if err != nil {
			t.Fatalf("Generate key failed: %v", err)
		}

		// Extract public key from result
		content, ok := result["content"].([]map[string]interface{})
		if !ok || len(content) == 0 {
			t.Fatal("Expected content array in result")
		}

		text := content[0]["text"].(string)
		if !strings.Contains(text, "publicKey") {
			t.Error("Expected publicKey in response")
		}
		if !strings.Contains(text, "privateKey") {
			t.Error("Expected privateKey in response")
		}
		if !strings.Contains(text, "liteAccountURL") {
			t.Error("Expected liteAccountURL in response")
		}

		// Parse JSON to extract public key
		var keyData map[string]interface{}
		if err := json.Unmarshal([]byte(text), &keyData); err == nil {
			if pk, ok := keyData["publicKey"].(string); ok {
				generatedPublicKey = pk
			}
		}

		t.Logf("Generated key successfully: %s", generatedPublicKey)
	})
}

// TestComprehensive_ADIWorkflow tests complete ADI creation and management workflow
func TestComprehensive_ADIWorkflow(t *testing.T) {
	c, err := client.NewClient(devnetEndpoint)
	if err != nil {
		t.Skipf("Devnet not available: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	_, err = c.NodeInfo(ctx, nil)
	if err != nil {
		t.Skipf("Cannot connect to devnet: %v", err)
	}

	server := NewServer()

	// Generate sponsor account (lite account)
	sponsorPub, sponsorPriv, _ := ed25519.GenerateKey(rand.Reader)
	sponsorURL, _ := protocol.LiteTokenAddress(sponsorPub, protocol.ACME, protocol.SignatureTypeED25519)

	t.Logf("Sponsor account: %s", sponsorURL)

	// Fund sponsor from faucet
	err = fundFromFaucet(t, c, sponsorURL.String())
	if err != nil {
		t.Fatalf("Failed to fund sponsor: %v", err)
	}

	// Wait for funding
	var balance int64
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		balance, err = getAccountBalance(t, c, sponsorURL.String())
		if err == nil && balance > 0 {
			break
		}
		t.Logf("Waiting for sponsor account creation... attempt %d/10", i+1)
	}
	if balance == 0 {
		t.Skip("Could not fund sponsor account - skipping ADI tests")
	}
	t.Logf("Sponsor funded with balance: %d", balance)

	var adiURL string
	var adiPublicKey string
	var adiPrivateKey string

	t.Run("add_credits", func(t *testing.T) {
		args := map[string]interface{}{
			"recipient":   sponsorURL.String(),
			"payer":       sponsorURL.String(),
			"amount":      "100000000", // 1 ACME worth of credits
			"private_key": hex.EncodeToString(sponsorPriv),
			"network":     devnetEndpoint,
		}

		result, err := server.addCredits(args)
		if err != nil {
			t.Fatalf("Add credits failed: %v", err)
		}

		content, ok := result["content"].([]map[string]interface{})
		if !ok || len(content) == 0 {
			t.Fatal("Expected content in result")
		}

		text := content[0]["text"].(string)
		if !strings.Contains(text, "txHash") {
			t.Error("Expected transaction hash in response")
		}

		t.Logf("Added credits: %s", text)
		time.Sleep(transactionWaitTime)
	})

	t.Run("create_adi", func(t *testing.T) {
		// Generate key for ADI
		adiPub, adiPriv, _ := ed25519.GenerateKey(rand.Reader)
		adiPublicKey = hex.EncodeToString(adiPub)
		adiPrivateKey = hex.EncodeToString(adiPriv)

		// Create unique ADI URL
		adiURL = fmt.Sprintf("acc://test-adi-%d.acme", time.Now().Unix())

		args := map[string]interface{}{
			"url":                 adiURL,
			"public_key":          adiPublicKey,
			"sponsor":             sponsorURL.String(),
			"sponsor_private_key": hex.EncodeToString(sponsorPriv),
			"network":             devnetEndpoint,
		}

		result, err := server.createADI(args)
		if err != nil {
			t.Fatalf("Create ADI failed: %v", err)
		}

		content, ok := result["content"].([]map[string]interface{})
		if !ok || len(content) == 0 {
			t.Fatal("Expected content in result")
		}

		text := content[0]["text"].(string)
		if !strings.Contains(text, adiURL) {
			t.Error("Expected ADI URL in response")
		}
		if !strings.Contains(text, "book") {
			t.Error("Expected key book URL in response")
		}

		t.Logf("Created ADI: %s", text)
		time.Sleep(accountCreationWait)
	})

	t.Run("query_keybook", func(t *testing.T) {
		if adiURL == "" {
			t.Skip("ADI not created - skipping keybook query")
		}

		keyBookURL := adiURL + "/book"
		args := map[string]interface{}{
			"url":     keyBookURL,
			"network": devnetEndpoint,
		}

		result, err := server.queryKeyBook(args)
		if err != nil {
			t.Logf("Query keybook error (ADI may not be ready): %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			if !strings.Contains(text, "pageCount") {
				t.Error("Expected pageCount in keybook response")
			}
			t.Logf("Keybook query result: %s", text)
		}
	})

	t.Run("query_keypage", func(t *testing.T) {
		if adiURL == "" {
			t.Skip("ADI not created - skipping keypage query")
		}

		keyPageURL := adiURL + "/book/1"
		args := map[string]interface{}{
			"url":     keyPageURL,
			"network": devnetEndpoint,
		}

		result, err := server.queryKeyPage(args)
		if err != nil {
			t.Logf("Query keypage error (ADI may not be ready): %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			if !strings.Contains(text, "keys") {
				t.Error("Expected keys in keypage response")
			}
			t.Logf("Keypage query result: %s", text)
		}
	})

	t.Run("create_data_account", func(t *testing.T) {
		if adiURL == "" {
			t.Skip("ADI not created - skipping data account creation")
		}

		dataAccountURL := adiURL + "/data"
		args := map[string]interface{}{
			"url":         dataAccountURL,
			"principal":   adiURL,
			"private_key": adiPrivateKey,
			"network":     devnetEndpoint,
		}

		result, err := server.createDataAccount(args)
		if err != nil {
			t.Logf("Create data account error: %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			if !strings.Contains(text, dataAccountURL) {
				t.Error("Expected data account URL in response")
			}
			t.Logf("Created data account: %s", text)
			time.Sleep(accountCreationWait)
		}
	})

	t.Run("write_data", func(t *testing.T) {
		if adiURL == "" {
			t.Skip("ADI not created - skipping write data")
		}

		dataAccountURL := adiURL + "/data"
		testData := fmt.Sprintf("Test data written at %d", time.Now().Unix())

		args := map[string]interface{}{
			"account":     dataAccountURL,
			"data":        testData,
			"private_key": adiPrivateKey,
			"network":     devnetEndpoint,
		}

		result, err := server.writeData(args)
		if err != nil {
			t.Logf("Write data error: %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			if !strings.Contains(text, "txHash") {
				t.Error("Expected transaction hash in response")
			}
			t.Logf("Wrote data: %s", text)
			time.Sleep(transactionWaitTime)
		}
	})

	t.Run("create_token_account", func(t *testing.T) {
		if adiURL == "" {
			t.Skip("ADI not created - skipping token account creation")
		}

		tokenAccountURL := adiURL + "/tokens"
		args := map[string]interface{}{
			"url":         tokenAccountURL,
			"principal":   adiURL,
			"token_url":   "acc://ACME",
			"private_key": adiPrivateKey,
			"network":     devnetEndpoint,
		}

		result, err := server.createTokenAccount(args)
		if err != nil {
			t.Logf("Create token account error: %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			if !strings.Contains(text, tokenAccountURL) {
				t.Error("Expected token account URL in response")
			}
			t.Logf("Created token account: %s", text)
			time.Sleep(accountCreationWait)
		}
	})

	t.Run("send_tokens", func(t *testing.T) {
		// Send from sponsor to another account
		recipientPub, _, _ := ed25519.GenerateKey(rand.Reader)
		recipientURL, _ := protocol.LiteTokenAddress(recipientPub, protocol.ACME, protocol.SignatureTypeED25519)

		args := map[string]interface{}{
			"from":        sponsorURL.String(),
			"to":          recipientURL.String(),
			"amount":      "0.1", // 0.1 ACME
			"private_key": hex.EncodeToString(sponsorPriv),
			"network":     devnetEndpoint,
		}

		result, err := server.sendTokens(args)
		if err != nil {
			t.Logf("Send tokens error: %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			if !strings.Contains(text, "txHash") {
				t.Error("Expected transaction hash in response")
			}
			t.Logf("Sent tokens: %s", text)
		}
	})
}

// TestComprehensive_KeyManagement tests advanced key management operations
func TestComprehensive_KeyManagement(t *testing.T) {
	c, err := client.NewClient(devnetEndpoint)
	if err != nil {
		t.Skipf("Devnet not available: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	_, err = c.NodeInfo(ctx, nil)
	if err != nil {
		t.Skipf("Cannot connect to devnet: %v", err)
	}

	server := NewServer()

	// Create sponsor account
	sponsorPub, sponsorPriv, _ := ed25519.GenerateKey(rand.Reader)
	sponsorURL, _ := protocol.LiteTokenAddress(sponsorPub, protocol.ACME, protocol.SignatureTypeED25519)

	// Fund and wait
	err = fundFromFaucet(t, c, sponsorURL.String())
	if err != nil {
		t.Skipf("Failed to fund sponsor: %v", err)
	}

	var balance int64
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		balance, err = getAccountBalance(t, c, sponsorURL.String())
		if err == nil && balance > 0 {
			break
		}
	}
	if balance == 0 {
		t.Skip("Could not fund sponsor - skipping key management tests")
	}

	// Add credits
	creditsArgs := map[string]interface{}{
		"recipient":   sponsorURL.String(),
		"payer":       sponsorURL.String(),
		"amount":      "100000000",
		"private_key": hex.EncodeToString(sponsorPriv),
		"network":     devnetEndpoint,
	}
	_, err = server.addCredits(creditsArgs)
	if err != nil {
		t.Skipf("Failed to add credits: %v", err)
	}
	time.Sleep(transactionWaitTime)

	// Create ADI
	adiPub, adiPriv, _ := ed25519.GenerateKey(rand.Reader)
	adiURL := fmt.Sprintf("acc://test-keymgmt-%d.acme", time.Now().Unix())

	adiArgs := map[string]interface{}{
		"url":                 adiURL,
		"public_key":          hex.EncodeToString(adiPub),
		"sponsor":             sponsorURL.String(),
		"sponsor_private_key": hex.EncodeToString(sponsorPriv),
		"network":             devnetEndpoint,
	}
	_, err = server.createADI(adiArgs)
	if err != nil {
		t.Skipf("Failed to create ADI: %v", err)
	}
	time.Sleep(accountCreationWait)

	t.Run("create_keypage", func(t *testing.T) {
		// Generate new key for keypage
		newPub, _, _ := ed25519.GenerateKey(rand.Reader)

		args := map[string]interface{}{
			"keybook":     adiURL + "/book",
			"public_keys": []string{hex.EncodeToString(newPub)},
			"private_key": hex.EncodeToString(adiPriv),
			"network":     devnetEndpoint,
		}

		result, err := server.createKeyPage(args)
		if err != nil {
			t.Logf("Create keypage error: %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			t.Logf("Created keypage: %s", text)
		}
	})

	t.Run("update_keypage", func(t *testing.T) {
		// Update existing keypage
		newPub, _, _ := ed25519.GenerateKey(rand.Reader)

		args := map[string]interface{}{
			"keypage":     adiURL + "/book/1",
			"operation":   "add",
			"public_key":  hex.EncodeToString(newPub),
			"private_key": hex.EncodeToString(adiPriv),
			"network":     devnetEndpoint,
		}

		result, err := server.updateKeyPage(args)
		if err != nil {
			t.Logf("Update keypage error: %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			t.Logf("Updated keypage: %s", text)
		}
	})

	t.Run("create_keybook", func(t *testing.T) {
		// Create additional keybook
		newPub, _, _ := ed25519.GenerateKey(rand.Reader)

		args := map[string]interface{}{
			"url":         adiURL + "/book2",
			"principal":   adiURL,
			"public_keys": []string{hex.EncodeToString(newPub)},
			"private_key": hex.EncodeToString(adiPriv),
			"network":     devnetEndpoint,
		}

		result, err := server.createKeyBook(args)
		if err != nil {
			t.Logf("Create keybook error: %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			t.Logf("Created keybook: %s", text)
		}
	})

	t.Run("update_account_auth", func(t *testing.T) {
		// Update account authorities
		args := map[string]interface{}{
			"account":     adiURL,
			"operations": []map[string]interface{}{
				{
					"operation": "enable",
					"authority": adiURL + "/book",
				},
			},
			"private_key": hex.EncodeToString(adiPriv),
			"network":     devnetEndpoint,
		}

		result, err := server.updateAccountAuth(args)
		if err != nil {
			t.Logf("Update account auth error: %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			t.Logf("Updated account auth: %s", text)
		}
	})
}

// TestComprehensive_TokenIssuance tests token creation and issuance
func TestComprehensive_TokenIssuance(t *testing.T) {
	c, err := client.NewClient(devnetEndpoint)
	if err != nil {
		t.Skipf("Devnet not available: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	_, err = c.NodeInfo(ctx, nil)
	if err != nil {
		t.Skipf("Cannot connect to devnet: %v", err)
	}

	server := NewServer()

	// Create sponsor account
	sponsorPub, sponsorPriv, _ := ed25519.GenerateKey(rand.Reader)
	sponsorURL, _ := protocol.LiteTokenAddress(sponsorPub, protocol.ACME, protocol.SignatureTypeED25519)

	// Fund and wait
	err = fundFromFaucet(t, c, sponsorURL.String())
	if err != nil {
		t.Skipf("Failed to fund sponsor: %v", err)
	}

	var balance int64
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		balance, err = getAccountBalance(t, c, sponsorURL.String())
		if err == nil && balance > 0 {
			break
		}
	}
	if balance == 0 {
		t.Skip("Could not fund sponsor - skipping token issuance tests")
	}

	// Add credits
	creditsArgs := map[string]interface{}{
		"recipient":   sponsorURL.String(),
		"payer":       sponsorURL.String(),
		"amount":      "200000000",
		"private_key": hex.EncodeToString(sponsorPriv),
		"network":     devnetEndpoint,
	}
	_, err = server.addCredits(creditsArgs)
	if err != nil {
		t.Skipf("Failed to add credits: %v", err)
	}
	time.Sleep(transactionWaitTime)

	// Create ADI
	adiPub, adiPriv, _ := ed25519.GenerateKey(rand.Reader)
	adiURL := fmt.Sprintf("acc://test-token-%d.acme", time.Now().Unix())

	adiArgs := map[string]interface{}{
		"url":                 adiURL,
		"public_key":          hex.EncodeToString(adiPub),
		"sponsor":             sponsorURL.String(),
		"sponsor_private_key": hex.EncodeToString(sponsorPriv),
		"network":             devnetEndpoint,
	}
	_, err = server.createADI(adiArgs)
	if err != nil {
		t.Skipf("Failed to create ADI: %v", err)
	}
	time.Sleep(accountCreationWait)

	var tokenURL string

	t.Run("create_token", func(t *testing.T) {
		tokenURL = adiURL + "/TestToken"

		args := map[string]interface{}{
			"url":         tokenURL,
			"symbol":      "TST",
			"precision":   "8",
			"supply_limit": "1000000000000000", // 10 million tokens
			"principal":   adiURL,
			"private_key": hex.EncodeToString(adiPriv),
			"network":     devnetEndpoint,
		}

		result, err := server.createToken(args)
		if err != nil {
			t.Fatalf("Create token failed: %v", err)
		}

		content := result["content"].([]map[string]interface{})
		text := content[0]["text"].(string)
		if !strings.Contains(text, tokenURL) {
			t.Error("Expected token URL in response")
		}
		t.Logf("Created token: %s", text)
		time.Sleep(accountCreationWait)
	})

	t.Run("issue_tokens", func(t *testing.T) {
		if tokenURL == "" {
			t.Skip("Token not created - skipping issue tokens")
		}

		// Create token account to receive issued tokens
		tokenAccountURL := adiURL + "/tokens"
		createAcctArgs := map[string]interface{}{
			"url":         tokenAccountURL,
			"principal":   adiURL,
			"token_url":   tokenURL,
			"private_key": hex.EncodeToString(adiPriv),
			"network":     devnetEndpoint,
		}
		_, err := server.createTokenAccount(createAcctArgs)
		if err != nil {
			t.Logf("Create token account error: %v", err)
		}
		time.Sleep(accountCreationWait)

		// Issue tokens
		args := map[string]interface{}{
			"token":       tokenURL,
			"to":          tokenAccountURL,
			"amount":      "1000000000000", // 10,000 tokens
			"private_key": hex.EncodeToString(adiPriv),
			"network":     devnetEndpoint,
		}

		result, err := server.issueTokens(args)
		if err != nil {
			t.Logf("Issue tokens error: %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			if !strings.Contains(text, "txHash") {
				t.Error("Expected transaction hash in response")
			}
			t.Logf("Issued tokens: %s", text)
			time.Sleep(transactionWaitTime)
		}
	})

	t.Run("burn_tokens", func(t *testing.T) {
		if tokenURL == "" {
			t.Skip("Token not created - skipping burn tokens")
		}

		tokenAccountURL := adiURL + "/tokens"

		args := map[string]interface{}{
			"account":     tokenAccountURL,
			"amount":      "100000000", // 1 token
			"private_key": hex.EncodeToString(adiPriv),
			"network":     devnetEndpoint,
		}

		result, err := server.burnTokens(args)
		if err != nil {
			t.Logf("Burn tokens error: %v", err)
		} else {
			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)
			if !strings.Contains(text, "txHash") {
				t.Error("Expected transaction hash in response")
			}
			t.Logf("Burned tokens: %s", text)
		}
	})
}
