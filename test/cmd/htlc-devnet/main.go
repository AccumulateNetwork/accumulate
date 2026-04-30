// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// HTLC devnet test - tests Hash Time-Locked Contracts against a running devnet
//
// Prerequisites:
// 1. Build accumulated with HTLC support: go build ./cmd/accumulated
// 2. Start a fresh devnet with current code (on different port to avoid conflicts):
//    ./accumulated run devnet -w /tmp/htlc-devnet --port 27656 -b 1 -v 1 -f 0 --name HTLCTest
// 3. Run this test with the correct API endpoint:
//    DEVNET_API="http://127.0.0.1:27660/v2" go run ./test/cmd/htlc-devnet/...
//
// Important Notes:
// - The devnet MUST be running a binary built from current source code that includes
//   the HTLC feature (TransactionTypeSyntheticLockedDeposit, ReleaseLockedOperation).
// - If you see "Internal error" when sending locked tokens, the devnet binary is outdated.
// - Lite accounts require credits to sign transactions. The test adds credits using
//   the AddCredits transaction, which itself requires credits (chicken-and-egg problem).
//   For production testing, consider using pre-funded accounts or the simulator tests.
// - For comprehensive HTLC testing, use the simulator-based tests in test/e2e/txn_htlc_test.go
//   which can directly manipulate account state.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var DevnetAPI = "http://127.0.0.1:26660/v2"

func init() {
	// Allow override via environment variable
	if api := os.Getenv("DEVNET_API"); api != "" {
		DevnetAPI = api
	}
}

func main() {
	fmt.Println("=== HTLC Devnet Test ===")
	fmt.Println()

	// Connect to devnet
	c, err := client.New(DevnetAPI)
	if err != nil {
		fatalf("Failed to connect to devnet: %v", err)
	}

	// Test 1: Successful HTLC release with SHA-256
	fmt.Println("Test 1: Successful HTLC release with SHA-256")
	testHTLCSuccessfulRelease(c)

	fmt.Println()
	fmt.Println("=== All HTLC tests passed ===")
}

func testHTLCSuccessfulRelease(c *client.Client) {
	ctx := context.Background()

	// Generate test keys
	_, alicePriv, _ := ed25519.GenerateKey(nil)
	_, bobPriv, _ := ed25519.GenerateKey(nil)
	alicePub := alicePriv.Public().(ed25519.PublicKey)
	bobPub := bobPriv.Public().(ed25519.PublicKey)

	fmt.Printf("  Using lite accounts for HTLC test\n")

	// Step 1: Setup Alice's lite token account
	aliceLiteId := protocol.LiteAuthorityForKey(alicePub, protocol.SignatureTypeED25519)
	aliceLite := aliceLiteId.JoinPath(protocol.AcmeUrl().ShortString())

	// Faucet lite account multiple times to get enough ACME
	faucetLite(ctx, c, aliceLite)
	faucetLite(ctx, c, aliceLite)
	faucetLite(ctx, c, aliceLite)
	waitForBalance(ctx, c, aliceLite, 20*protocol.AcmePrecision)
	fmt.Printf("  Funded Alice's lite account: %s\n", aliceLite)

	// Add credits to Alice's lite identity for signing
	addCreditsLite(ctx, c, aliceLite, aliceLiteId, 1, alicePub, alicePriv)
	time.Sleep(2 * time.Second)
	fmt.Printf("  Added credits to Alice's lite identity\n")

	// Step 2: Setup Bob's lite token account
	bobLiteId := protocol.LiteAuthorityForKey(bobPub, protocol.SignatureTypeED25519)
	bobLite := bobLiteId.JoinPath(protocol.AcmeUrl().ShortString())

	faucetLite(ctx, c, bobLite)
	waitForBalance(ctx, c, bobLite, 10*protocol.AcmePrecision)
	fmt.Printf("  Funded Bob's lite account: %s\n", bobLite)

	// Add credits to Bob's lite identity for releasing the lock
	addCreditsLite(ctx, c, bobLite, bobLiteId, 1, bobPub, bobPriv)
	time.Sleep(2 * time.Second)
	fmt.Printf("  Added credits to Bob's lite identity\n")

	// Step 3: Create the HTLC
	preimage := []byte("test-secret-preimage-32-bytes!!")
	hash := sha256.Sum256(preimage)
	expiration := time.Now().Add(1 * time.Hour)

	fmt.Printf("  Preimage: %s\n", hex.EncodeToString(preimage))
	fmt.Printf("  Hash: %s\n", hex.EncodeToString(hash[:]))
	fmt.Printf("  Expiration: %s\n", expiration.Format(time.RFC3339))

	// Alice sends locked tokens to Bob's lite account
	lockedTxID := sendLockedTokensLite(ctx, c, aliceLite, bobLite, 5*protocol.AcmePrecision, hash, expiration, alicePub, alicePriv)
	fmt.Printf("  Alice sent locked deposit to Bob, TxID: %s\n", lockedTxID)

	// Wait for the locked deposit to be delivered
	time.Sleep(3 * time.Second)

	// Query the transaction to get produced synthetic
	producedTxID := getProducedLockedDeposit(ctx, c, lockedTxID)
	if producedTxID == nil {
		fatalf("No SyntheticLockedDeposit was produced")
	}
	fmt.Printf("  Locked deposit TxID: %s\n", producedTxID)

	// Step 4: Bob releases with correct preimage
	releaseLockedDepositLite(ctx, c, bobLite, producedTxID, preimage, bobPub, bobPriv)
	fmt.Printf("  Bob released locked deposit with preimage\n")

	// Step 5: Verify Bob received tokens
	time.Sleep(3 * time.Second)
	bobBalance := getBalance(ctx, c, bobLite)
	expectedMin := uint64(14 * protocol.AcmePrecision) // 10 (faucet) + 5 (HTLC) - 1 (credits) = 14
	if bobBalance < expectedMin {
		fatalf("Bob didn't receive tokens: balance = %d, expected >= %d", bobBalance, expectedMin)
	}
	fmt.Printf("  Bob's balance: %d (expected >= %d)\n", bobBalance, expectedMin)

	fmt.Println("  PASSED")
}

func faucetLite(ctx context.Context, c *client.Client, account *url.URL) {
	req := new(api.GeneralQuery)
	req.Url = account

	var resp api.TxResponse
	err := c.RequestAPIv2(ctx, "faucet", req, &resp)
	if err != nil {
		fatalf("Failed to call faucet: %v", err)
	}
}

//nolint:unused
func createIdentity(ctx context.Context, c *client.Client, signer, identity *url.URL, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = signer
	txn.Body = &protocol.CreateIdentity{
		Url:        identity,
		KeyHash:    doSha256(pub),
		KeyBookUrl: identity.JoinPath("book"),
	}

	signAndSubmit(ctx, c, txn, signer, pub, priv, 1)
}

//nolint:unused
func createTokenAccount(ctx context.Context, c *client.Client, signer, account, token *url.URL, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = signer
	txn.Body = &protocol.CreateTokenAccount{
		Url:      account,
		TokenUrl: token,
	}

	signAndSubmit(ctx, c, txn, signer.JoinPath("book", "1"), pub, priv, 1)
}

//nolint:unused
func addCredits(ctx context.Context, c *client.Client, from, to *url.URL, amount uint64, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = from
	txn.Body = &protocol.AddCredits{
		Recipient: to,
		Amount:    *big.NewInt(int64(amount * protocol.AcmePrecision)),
		Oracle:    protocol.InitialAcmeOracleValue,
	}

	signAndSubmit(ctx, c, txn, from, pub, priv, 1)
}

func addCreditsLite(ctx context.Context, c *client.Client, from, to *url.URL, amount uint64, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = from
	txn.Body = &protocol.AddCredits{
		Recipient: to,
		Amount:    *big.NewInt(int64(amount * protocol.AcmePrecision)),
		Oracle:    protocol.InitialAcmeOracleValue,
	}

	// Sign from the lite identity (RootIdentity of the token account)
	signAndSubmit(ctx, c, txn, from.RootIdentity(), pub, priv, 1)
}

//nolint:unused
func sendTokens(ctx context.Context, c *client.Client, from, to *url.URL, amount uint64, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = from
	txn.Body = &protocol.SendTokens{
		To: []*protocol.TokenRecipient{{
			Url:    to,
			Amount: *big.NewInt(int64(amount)),
		}},
	}

	// Use RootIdentity for lite accounts, or just the from URL for ADI accounts
	signer := from.RootIdentity()
	signAndSubmit(ctx, c, txn, signer, pub, priv, 1)
}

//nolint:unused
func sendLockedTokens(ctx context.Context, c *client.Client, from, to *url.URL, amount uint64, hash [32]byte, expiration time.Time, pub ed25519.PublicKey, priv ed25519.PrivateKey) *url.TxID {
	txn := new(protocol.Transaction)
	txn.Header.Principal = from
	txn.Header.HashLock = &protocol.HashLockOptions{
		HashAlgorithm: protocol.HashAlgorithmSHA256,
		Hash:          hash[:],
		Expiration:    &expiration,
	}
	txn.Body = &protocol.SendTokens{
		To: []*protocol.TokenRecipient{{
			Url:    to,
			Amount: *big.NewInt(int64(amount)),
		}},
	}

	env := signAndPrepare(txn, from.RootIdentity().JoinPath("book", "1"), pub, priv, 1)
	submit(ctx, c, env)

	return txn.ID()
}

func sendLockedTokensLite(ctx context.Context, c *client.Client, from, to *url.URL, amount uint64, hash [32]byte, expiration time.Time, pub ed25519.PublicKey, priv ed25519.PrivateKey) *url.TxID {
	txn := new(protocol.Transaction)
	txn.Header.Principal = from
	txn.Header.HashLock = &protocol.HashLockOptions{
		HashAlgorithm: protocol.HashAlgorithmSHA256,
		Hash:          hash[:],
		Expiration:    &expiration,
	}
	txn.Body = &protocol.SendTokens{
		To: []*protocol.TokenRecipient{{
			Url:    to,
			Amount: *big.NewInt(int64(amount)),
		}},
	}

	// For lite accounts, sign from the lite identity
	env := signAndPrepare(txn, from.RootIdentity(), pub, priv, 1)
	submit(ctx, c, env)

	return txn.ID()
}

func getProducedLockedDeposit(ctx context.Context, c *client.Client, txid *url.TxID) *url.TxID {
	req := new(api.TxnQuery)
	hash := txid.Hash()
	req.Txid = hash[:]
	req.Wait = 10 * time.Second

	resp, err := c.QueryTx(ctx, req)
	if err != nil {
		fatalf("Failed to query transaction: %v", err)
	}

	// Find SyntheticLockedDeposit in produced
	for _, produced := range resp.Produced {
		txReq := new(api.TxnQuery)
		producedHash := produced.Hash()
		txReq.Txid = producedHash[:]
		txResp, err := c.QueryTx(ctx, txReq)
		if err != nil {
			continue
		}
		if txResp.Transaction.Body.Type() == protocol.TransactionTypeSyntheticLockedDeposit {
			return txResp.Transaction.ID()
		}
	}

	return nil
}

//nolint:unused
func releaseLockedDeposit(ctx context.Context, c *client.Client, principal *url.URL, lockedTxID *url.TxID, preimage []byte, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = principal
	txn.Body = &protocol.ReleaseLockedOperation{
		LockedTxID: lockedTxID,
		Preimage:   preimage,
	}

	signAndSubmit(ctx, c, txn, principal.RootIdentity().JoinPath("book", "1"), pub, priv, 1)
}

func releaseLockedDepositLite(ctx context.Context, c *client.Client, principal *url.URL, lockedTxID *url.TxID, preimage []byte, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = principal
	txn.Body = &protocol.ReleaseLockedOperation{
		LockedTxID: lockedTxID,
		Preimage:   preimage,
	}

	// For lite accounts, sign from the lite identity
	signAndSubmit(ctx, c, txn, principal.RootIdentity(), pub, priv, 1)
}

func signAndSubmit(ctx context.Context, c *client.Client, txn *protocol.Transaction, signer *url.URL, pub ed25519.PublicKey, priv ed25519.PrivateKey, version uint64) {
	env := signAndPrepare(txn, signer, pub, priv, version)
	submit(ctx, c, env)
}

func signAndPrepare(txn *protocol.Transaction, signer *url.URL, pub ed25519.PublicKey, priv ed25519.PrivateKey, version uint64) *messaging.Envelope {
	b := new(signing.Builder)
	b.InitMode = signing.InitWithSimpleHash
	b.Type = protocol.SignatureTypeED25519
	b.Url = signer
	b.Version = version
	b.SetTimestamp(uint64(time.Now().UnixNano()))
	b.SetPrivateKey(priv)

	sig, err := b.Initiate(txn)
	if err != nil {
		fatalf("Failed to initiate: %v", err)
	}

	env := new(messaging.Envelope)
	env.Transaction = []*protocol.Transaction{txn}
	env.Signatures = []protocol.Signature{sig}
	return env
}

func submit(ctx context.Context, c *client.Client, env *messaging.Envelope) {
	req := new(api.ExecuteRequest)
	req.Envelope = env

	resp, err := c.ExecuteDirect(ctx, req)
	if err != nil {
		fatalf("Failed to submit transaction: %v", err)
	}

	// Check result - may be array or single status
	data, _ := json.Marshal(resp.Result)

	// Try array first
	var statuses []protocol.TransactionStatus
	if err := json.Unmarshal(data, &statuses); err == nil {
		for _, status := range statuses {
			if status.Code != 0 && !status.Code.Success() {
				fatalf("Transaction failed: %v", status.Error)
			}
		}
		return
	}

	// Try single status
	var status protocol.TransactionStatus
	if err := json.Unmarshal(data, &status); err != nil {
		// Ignore unmarshal errors - check for explicit failure
		return
	}

	if status.Code != 0 && !status.Code.Success() {
		fatalf("Transaction failed: %v", status.Error)
	}
}

func getBalance(ctx context.Context, c *client.Client, account *url.URL) uint64 {
	req := new(api.GeneralQuery)
	req.Url = account

	var raw json.RawMessage
	var resp api.ChainQueryResponse
	resp.Data = &raw

	err := c.RequestAPIv2(ctx, "query", req, &resp)
	if err != nil {
		return 0
	}

	acct, err := protocol.UnmarshalAccountJSON(raw)
	if err != nil {
		return 0
	}

	switch a := acct.(type) {
	case *protocol.LiteTokenAccount:
		return a.Balance.Uint64()
	case *protocol.TokenAccount:
		return a.Balance.Uint64()
	}
	return 0
}

func waitForBalance(ctx context.Context, c *client.Client, account *url.URL, minBalance uint64) {
	for i := 0; i < 30; i++ {
		balance := getBalance(ctx, c, account)
		if balance >= minBalance {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	fatalf("Timeout waiting for balance on %s", account)
}

//nolint:unused
func doSha256(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "FAILED: "+format+"\n", args...)
	os.Exit(1)
}
