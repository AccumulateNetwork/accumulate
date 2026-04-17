package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const testNodeURL = "http://localhost:26660"

func skipIfNoNetwork(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short flag set, no network expected")
	}
	resp, err := http.Get(testNodeURL + "/version")
	if err != nil {
		t.Skipf("skipping: network not reachable: %v", err)
	}
	resp.Body.Close()
}

// submitBinary posts a binary envelope to /submit and returns the HTTP status and body.
func submitBinary(t *testing.T, data []byte) (int, string) {
	t.Helper()
	resp, err := http.Post(testNodeURL+"/submit", "application/octet-stream", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST /submit failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestBinarySubmitGarbage(t *testing.T) {
	skipIfNoNetwork(t)

	status, body := submitBinary(t, []byte("garbage"))
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for garbage, got %d: %s", status, body)
	}
	t.Logf("Garbage correctly rejected with %d: %s", status, body)
}

func TestBinarySubmitSendToSelf(t *testing.T) {
	skipIfNoNetwork(t)

	// Derive the faucet key the same way the load test and genesis do
	faucetSK := deriveKey("FAUCET")
	faucetURL, err := protocol.LiteTokenAddress(faucetSK[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		t.Fatalf("LiteTokenAddress: %v", err)
	}
	t.Logf("Faucet URL: %v", faucetURL)

	nonce := uint64(time.Now().UnixNano())
	env, err := build.Transaction().
		For(faucetURL).
		SendTokens(1, protocol.AcmePrecisionPower).To(faucetURL).
		SignWith(faucetURL).
		Version(1).
		Timestamp(&nonce).
		PrivateKey(faucetSK).
		Done()
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}

	data, err := env.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	t.Logf("Envelope size: %d bytes", len(data))

	status, body := submitBinary(t, data)
	t.Logf("Response: %d %s", status, body)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", status, body)
	}
}

func TestBinarySubmitFundAndTransfer(t *testing.T) {
	skipIfNoNetwork(t)

	faucetSK := deriveKey("FAUCET")
	faucetURL, err := protocol.LiteTokenAddress(faucetSK[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		t.Fatalf("LiteTokenAddress: %v", err)
	}
	t.Logf("Faucet URL: %v", faucetURL)

	// Derive a new test account
	testSK := deriveKey("FAUCET", "test-binary-submit", 1)
	testURL, err := protocol.LiteTokenAddress(testSK[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		t.Fatalf("LiteTokenAddress for test account: %v", err)
	}
	t.Logf("Test account URL: %v", testURL)

	nonce := uint64(time.Now().UnixNano())

	// Step 1: Fund the test account from faucet
	t.Run("fund", func(t *testing.T) {
		env, err := build.Transaction().
			For(faucetURL).
			SendTokens(100, protocol.AcmePrecisionPower).To(testURL).
			SignWith(faucetURL).
			Version(1).
			Timestamp(&nonce).
			PrivateKey(faucetSK).
			Done()
		if err != nil {
			t.Fatalf("build fund txn: %v", err)
		}

		data, err := env.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary: %v", err)
		}
		t.Logf("Fund envelope size: %d bytes", len(data))

		status, body := submitBinary(t, data)
		t.Logf("Fund response: %d %s", status, body)
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", status, body)
		}
	})

	// Step 2: Buy credits for the test account
	t.Run("buy-credits", func(t *testing.T) {
		env, err := build.Transaction().
			For(testURL).
			AddCredits().
			WithOracle(1000).
			Spend(3).
			To(testURL.RootIdentity()).
			SignWith(testURL).
			Version(1).
			Timestamp(&nonce).
			PrivateKey(testSK).
			Done()
		if err != nil {
			t.Fatalf("build credits txn: %v", err)
		}

		data, err := env.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary: %v", err)
		}
		t.Logf("Credits envelope size: %d bytes", len(data))

		status, body := submitBinary(t, data)
		t.Logf("Credits response: %d %s", status, body)
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", status, body)
		}
	})

	// Step 3: Send tokens from test account (may fail if credits haven't settled,
	// but the submission itself should return 200)
	t.Run("send-tokens", func(t *testing.T) {
		// Derive another account as the destination
		destSK := deriveKey("FAUCET", "test-binary-submit", 2)
		destURL, err := protocol.LiteTokenAddress(destSK[32:], "ACME", protocol.SignatureTypeED25519)
		if err != nil {
			t.Fatalf("LiteTokenAddress for dest: %v", err)
		}

		env, err := build.Transaction().
			For(testURL).
			SendTokens(1, protocol.AcmePrecisionPower).To(destURL).
			SignWith(testURL).
			Version(1).
			Timestamp(&nonce).
			PrivateKey(testSK).
			Done()
		if err != nil {
			t.Fatalf("build send txn: %v", err)
		}

		data, err := env.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary: %v", err)
		}
		t.Logf("Send envelope size: %d bytes", len(data))

		status, body := submitBinary(t, data)
		t.Logf("Send response: %d %s", status, body)
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", status, body)
		}
	})
}

// TestBinarySubmitDebugLoadtest replicates exactly what the load test does
// to help identify why it gets 100% errors.
func TestBinarySubmitDebugLoadtest(t *testing.T) {
	skipIfNoNetwork(t)

	faucetSeed := "FAUCET"
	faucetSK := deriveKey(faucetSeed)
	faucetURL, err := protocol.LiteTokenAddress(faucetSK[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		t.Fatalf("LiteTokenAddress: %v", err)
	}
	t.Logf("Faucet: %v", faucetURL)
	t.Logf("Faucet pubkey: %x", ed25519.PrivateKey(faucetSK).Public())

	nodeURL := testNodeURL + "/submit"
	httpClient := &http.Client{Timeout: 30 * time.Second}

	nonce := uint64(time.Now().UnixMilli())

	// Replicate funder seeding from the load test
	funderSK := deriveKey(faucetSeed, "funder", 0)
	funderURL, _ := protocol.LiteTokenAddress(funderSK[32:], "ACME", protocol.SignatureTypeED25519)
	t.Logf("Funder 0: %v", funderURL)

	// Send ACME to funder (exactly as load test does)
	fundingAmount := uint64(100000)
	env, err := build.Transaction().
		For(faucetURL).
		SendTokens(fundingAmount, protocol.AcmePrecisionPower).To(funderURL).
		SignWith(faucetURL).
		Version(1).
		Timestamp(&nonce).
		PrivateKey(faucetSK).
		Done()
	if err != nil {
		t.Logf("WARNING: build fund-funder envelope error: %v", err)
	}
	if env == nil {
		t.Fatal("build.Transaction returned nil envelope")
	}

	data, err := env.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	t.Logf("Fund-funder envelope: %d bytes", len(data))

	// Submit exactly as the load test does
	ok := submit(context.Background(), httpClient, nodeURL, env)
	t.Logf("Fund-funder submit (via loadtest submit func): %v", ok)

	// Also try direct POST to /submit
	status, body := submitBinary(t, data)
	t.Logf("Fund-funder direct POST: %d %s", status, body)

	if !ok {
		// Debug: try with explicit context
		t.Log("Submit returned false. Debugging URL construction...")
		submitURL := fmt.Sprintf("%s/submit", testNodeURL)
		t.Logf("Expected submit URL: %s", submitURL)

		resp, err := http.Post(submitURL, "application/octet-stream", bytes.NewReader(data))
		if err != nil {
			t.Fatalf("Direct POST failed: %v", err)
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		t.Logf("Direct POST response: %d %s", resp.StatusCode, string(respBody))
	}
}
