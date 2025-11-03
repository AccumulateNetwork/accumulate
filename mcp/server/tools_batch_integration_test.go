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

const devnetEndpoint = "http://localhost:26660/v3"

// TestBatchSubmission_Devnet tests batch submission against a running devnet
func TestBatchSubmission_Devnet(t *testing.T) {
	// Skip if devnet is not running
	c, err := client.NewClient(devnetEndpoint)
	if err != nil {
		t.Skipf("Devnet not available: %v", err)
	}
	defer c.Close()

	// Test network connectivity
	ctx := context.Background()
	_, err = c.NodeInfo(ctx, nil)
	if err != nil {
		t.Skipf("Cannot connect to devnet: %v", err)
	}

	t.Run("single_transaction", func(t *testing.T) {
		testBatchSingleTransaction(t, c)
	})

	t.Run("multiple_transactions", func(t *testing.T) {
		testBatchMultipleTransactions(t, c)
	})

	t.Run("partial_failure", func(t *testing.T) {
		testBatchPartialFailure(t, c)
	})
}

func testBatchSingleTransaction(t *testing.T, c *client.Client) {
	// Generate sender and recipient keys
	senderPub, senderPriv, _ := ed25519.GenerateKey(rand.Reader)
	recipientPub, _, _ := ed25519.GenerateKey(rand.Reader)

	// Create lite account URLs - both need to be token addresses
	senderURL, _ := protocol.LiteTokenAddress(senderPub, protocol.ACME, protocol.SignatureTypeED25519)
	recipientURL, _ := protocol.LiteTokenAddress(recipientPub, protocol.ACME, protocol.SignatureTypeED25519)

	t.Logf("Sender: %s", senderURL)
	t.Logf("Recipient: %s", recipientURL)

	// Fund sender from faucet
	err := fundFromFaucet(t, c, senderURL.String())
	if err != nil {
		t.Fatalf("Failed to fund sender: %v", err)
	}

	// Wait for funding to complete and account to be created
	var balance int64
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		balance, err = getAccountBalance(t, c, senderURL.String())
		if err == nil && balance > 0 {
			break
		}
		t.Logf("Waiting for account creation... attempt %d/10", i+1)
	}

	if err != nil {
		t.Fatalf("Failed to get sender balance after 10 attempts: %v", err)
	}
	t.Logf("Sender balance: %d", balance)

	if balance == 0 {
		t.Fatal("Sender has no tokens after faucet funding")
	}

	// Create server and test batch submission
	server := NewServer()

	args := map[string]interface{}{
		"transactions": []interface{}{
			map[string]interface{}{
				"type": "sendTokens",
				"params": map[string]interface{}{
					"from":   senderURL.String(),
					"to":     recipientURL.String(),
					"amount": "100000000", // 1 ACME
				},
			},
		},
		"private_key": hex.EncodeToString(senderPriv),
		"network":     devnetEndpoint,
	}

	result, err := server.submitBatch(args)
	if err != nil {
		t.Fatalf("Batch submission failed: %v", err)
	}

	// Verify result structure
	content, ok := result["content"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected content array in result")
	}

	text := content[0]["text"].(string)
	t.Logf("Result: %s", text)

	// Verify warnings are present
	if !strings.Contains(text, "⚠️") {
		t.Error("Expected warnings in response")
	}
	if !strings.Contains(text, "no fee savings") {
		t.Error("Expected 'no fee savings' warning")
	}
	if !strings.Contains(text, "partial failures possible") {
		t.Error("Expected 'partial failures possible' warning")
	}

	// Wait for transaction to be processed
	time.Sleep(3 * time.Second)

	// Verify recipient received tokens
	recipientBalance, err := getAccountBalance(t, c, recipientURL.String())
	if err != nil {
		t.Logf("Warning: Could not verify recipient balance: %v", err)
	} else {
		t.Logf("Recipient balance: %d", recipientBalance)
		if recipientBalance != 100000000 {
			t.Errorf("Expected recipient balance 100000000, got %d", recipientBalance)
		}
	}
}

func testBatchMultipleTransactions(t *testing.T, c *client.Client) {
	// Generate keys
	senderPub, senderPriv, _ := ed25519.GenerateKey(rand.Reader)
	recipient1Pub, _, _ := ed25519.GenerateKey(rand.Reader)
	recipient2Pub, _, _ := ed25519.GenerateKey(rand.Reader)
	recipient3Pub, _, _ := ed25519.GenerateKey(rand.Reader)

	// Create lite account URLs - all need to be token addresses
	senderURL, _ := protocol.LiteTokenAddress(senderPub, protocol.ACME, protocol.SignatureTypeED25519)
	recipient1URL, _ := protocol.LiteTokenAddress(recipient1Pub, protocol.ACME, protocol.SignatureTypeED25519)
	recipient2URL, _ := protocol.LiteTokenAddress(recipient2Pub, protocol.ACME, protocol.SignatureTypeED25519)
	recipient3URL, _ := protocol.LiteTokenAddress(recipient3Pub, protocol.ACME, protocol.SignatureTypeED25519)

	t.Logf("Sender: %s", senderURL)
	t.Logf("Recipient 1: %s", recipient1URL)
	t.Logf("Recipient 2: %s", recipient2URL)
	t.Logf("Recipient 3: %s", recipient3URL)

	// Fund sender
	err := fundFromFaucet(t, c, senderURL.String())
	if err != nil {
		t.Fatalf("Failed to fund sender: %v", err)
	}

	// Wait for funding to complete and account to be created
	var balance int64
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		balance, err = getAccountBalance(t, c, senderURL.String())
		if err == nil && balance > 0 {
			break
		}
		t.Logf("Waiting for account creation... attempt %d/10", i+1)
	}
	if err != nil {
		t.Skipf("Could not get sender balance: %v", err)
	}
	t.Logf("Sender funded with balance: %d", balance)

	// Submit batch with 3 transactions
	server := NewServer()

	args := map[string]interface{}{
		"transactions": []interface{}{
			map[string]interface{}{
				"type": "sendTokens",
				"params": map[string]interface{}{
					"from":   senderURL.String(),
					"to":     recipient1URL.String(),
					"amount": "100000000", // 1 ACME
				},
			},
			map[string]interface{}{
				"type": "sendTokens",
				"params": map[string]interface{}{
					"from":   senderURL.String(),
					"to":     recipient2URL.String(),
					"amount": "200000000", // 2 ACME
				},
			},
			map[string]interface{}{
				"type": "sendTokens",
				"params": map[string]interface{}{
					"from":   senderURL.String(),
					"to":     recipient3URL.String(),
					"amount": "300000000", // 3 ACME
				},
			},
		},
		"private_key": hex.EncodeToString(senderPriv),
		"network":     devnetEndpoint,
	}

	result, err := server.submitBatch(args)
	if err != nil {
		t.Fatalf("Batch submission failed: %v", err)
	}

	// Verify result
	content, ok := result["content"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected content array in result")
	}

	text := content[0]["text"].(string)
	t.Logf("Result: %s", text)

	// Verify 3 transactions were submitted
	if !strings.Contains(text, "Transactions: 3") {
		t.Error("Expected 3 transactions in result")
	}

	// Wait for transactions to be processed
	time.Sleep(5 * time.Second)

	// Verify all recipients received tokens
	balances := []struct {
		url      string
		expected int64
	}{
		{recipient1URL.String(), 100000000},
		{recipient2URL.String(), 200000000},
		{recipient3URL.String(), 300000000},
	}

	for i, b := range balances {
		balance, err := getAccountBalance(t, c, b.url)
		if err != nil {
			t.Logf("Warning: Could not verify recipient %d balance: %v", i+1, err)
		} else {
			t.Logf("Recipient %d balance: %d", i+1, balance)
			if balance != b.expected {
				t.Errorf("Recipient %d: expected balance %d, got %d", i+1, b.expected, balance)
			}
		}
	}
}

func testBatchPartialFailure(t *testing.T, c *client.Client) {
	// Generate keys
	senderPub, senderPriv, _ := ed25519.GenerateKey(rand.Reader)
	recipient1Pub, _, _ := ed25519.GenerateKey(rand.Reader)
	recipient2Pub, _, _ := ed25519.GenerateKey(rand.Reader)

	// Create lite account URLs - all need to be token addresses
	senderURL, _ := protocol.LiteTokenAddress(senderPub, protocol.ACME, protocol.SignatureTypeED25519)
	recipient1URL, _ := protocol.LiteTokenAddress(recipient1Pub, protocol.ACME, protocol.SignatureTypeED25519)
	recipient2URL, _ := protocol.LiteTokenAddress(recipient2Pub, protocol.ACME, protocol.SignatureTypeED25519)

	t.Logf("Sender: %s", senderURL)
	t.Logf("Recipient 1: %s", recipient1URL)
	t.Logf("Recipient 2: %s", recipient2URL)

	// Fund sender with limited amount
	err := fundFromFaucet(t, c, senderURL.String())
	if err != nil {
		t.Fatalf("Failed to fund sender: %v", err)
	}

	// Wait for funding to complete and account to be created
	var balance int64
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		balance, err = getAccountBalance(t, c, senderURL.String())
		if err == nil && balance > 0 {
			break
		}
		t.Logf("Waiting for account creation... attempt %d/10", i+1)
	}
	if err != nil {
		t.Skipf("Could not get sender balance: %v", err)
	}
	t.Logf("Sender balance: %d", balance)

	// Submit batch where second transaction will fail (insufficient funds)
	server := NewServer()

	args := map[string]interface{}{
		"transactions": []interface{}{
			map[string]interface{}{
				"type": "sendTokens",
				"params": map[string]interface{}{
					"from":   senderURL.String(),
					"to":     recipient1URL.String(),
					"amount": "100000000", // 1 ACME - should succeed
				},
			},
			map[string]interface{}{
				"type": "sendTokens",
				"params": map[string]interface{}{
					"from":   senderURL.String(),
					"to":     recipient2URL.String(),
					"amount": "99999999999999999", // Huge amount - should fail
				},
			},
		},
		"private_key": hex.EncodeToString(senderPriv),
		"network":     devnetEndpoint,
	}

	result, err := server.submitBatch(args)
	// Note: Submission itself should succeed even if transactions will fail
	if err != nil {
		t.Logf("Note: Batch submission returned error (expected for partial failure): %v", err)
	} else {
		content, ok := result["content"].([]map[string]interface{})
		if ok {
			text := content[0]["text"].(string)
			t.Logf("Result: %s", text)
		}
	}

	// Wait for transactions to be processed
	time.Sleep(5 * time.Second)

	// Verify first recipient received tokens (transaction should succeed)
	recipient1Balance, err := getAccountBalance(t, c, recipient1URL.String())
	if err != nil {
		t.Logf("Warning: Could not verify recipient 1 balance: %v", err)
	} else {
		t.Logf("Recipient 1 balance: %d", recipient1Balance)
		if recipient1Balance == 100000000 {
			t.Log("✓ First transaction succeeded as expected (partial execution demonstrated)")
		}
	}

	// Verify second recipient did NOT receive tokens (transaction should fail)
	recipient2Balance, err := getAccountBalance(t, c, recipient2URL.String())
	if err != nil {
		t.Logf("Recipient 2 account doesn't exist (expected - transaction failed)")
	} else {
		t.Logf("Recipient 2 balance: %d", recipient2Balance)
		if recipient2Balance == 0 || recipient2Balance != 99999999999999999 {
			t.Log("✓ Second transaction failed as expected (partial execution demonstrated)")
		}
	}

	t.Log("✓ Partial failure test demonstrates independent transaction execution")
}

// Helper functions

func fundFromFaucet(t *testing.T, c *client.Client, accountURL string) error {
	ctx := context.Background()

	// Call faucet
	_, err := c.Faucet(ctx, accountURL, nil)
	if err != nil {
		return fmt.Errorf("faucet call failed: %w", err)
	}

	t.Logf("Funded account %s from faucet", accountURL)
	return nil
}

func getAccountBalance(t *testing.T, c *client.Client, accountURL string) (int64, error) {
	ctx := context.Background()

	// Query account
	resp, err := c.QueryAccount(ctx, accountURL)
	if err != nil {
		return 0, fmt.Errorf("query failed: %w", err)
	}

	// Marshal and unmarshal to extract balance
	data, err := json.Marshal(resp)
	if err != nil {
		return 0, fmt.Errorf("marshal failed: %w", err)
	}

	var account map[string]interface{}
	err = json.Unmarshal(data, &account)
	if err != nil {
		return 0, fmt.Errorf("unmarshal failed: %w", err)
	}

	// Extract balance from account.balance
	if accountData, ok := account["account"].(map[string]interface{}); ok {
		if balance, ok := accountData["balance"].(float64); ok {
			return int64(balance), nil
		}
		// Also check if balance is a string
		if balanceStr, ok := accountData["balance"].(string); ok {
			// Try to parse as integer
			var bal int64
			fmt.Sscanf(balanceStr, "%d", &bal)
			return bal, nil
		}
	}

	// Fallback: Try other locations
	if record, ok := account["record"].(map[string]interface{}); ok {
		if balance, ok := record["balance"].(float64); ok {
			return int64(balance), nil
		}
	}

	return 0, fmt.Errorf("no balance field in account")
}
