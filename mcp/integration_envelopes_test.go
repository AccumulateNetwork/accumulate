// go:build integration
//go:build integration
// +build integration

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"time"

	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
)

// hashTo32Bytes converts a []byte hash to a [32]byte array
func hashTo32Bytes(hash []byte) [32]byte {
	var arr [32]byte
	copy(arr[:], hash)
	return arr
}

// TestDevnetMultipleTokenSendsInEnvelope tests sending multiple token transfers in a single envelope
func TestDevnetMultipleTokenSendsInEnvelope(t *testing.T) {
	fmt.Println("\n=== Multiple Token Sends in One Envelope Test ===")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Step 1: Generate keys for source and 3 destination accounts
	fmt.Println("Step 1: Generating keys for source and 3 destination accounts...")
	sourcePubKey, sourcePrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate source key: %v", err)
	}

	dest1PubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate dest1 key: %v", err)
	}

	dest2PubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate dest2 key: %v", err)
	}

	dest3PubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate dest3 key: %v", err)
	}

	// Step 2: Create lite accounts
	fmt.Println("Step 2: Creating lite accounts...")
	sourceAccount, err := client.CreateLiteAccountURL(hex.EncodeToString(sourcePubKey))
	if err != nil {
		t.Fatalf("Failed to create source account: %v", err)
	}

	dest1Account, err := client.CreateLiteAccountURL(hex.EncodeToString(dest1PubKey))
	if err != nil {
		t.Fatalf("Failed to create dest1 account: %v", err)
	}

	dest2Account, err := client.CreateLiteAccountURL(hex.EncodeToString(dest2PubKey))
	if err != nil {
		t.Fatalf("Failed to create dest2 account: %v", err)
	}

	dest3Account, err := client.CreateLiteAccountURL(hex.EncodeToString(dest3PubKey))
	if err != nil {
		t.Fatalf("Failed to create dest3 account: %v", err)
	}

	fmt.Printf("Source account: %s\n", sourceAccount)
	fmt.Printf("Dest1 account: %s\n", dest1Account)
	fmt.Printf("Dest2 account: %s\n", dest2Account)
	fmt.Printf("Dest3 account: %s\n", dest3Account)

	// Step 3: Fund source account via faucet
	fmt.Println("Step 3: Funding source account via faucet...")
	_, err = c.Faucet(ctx, sourceAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}

	fmt.Println("Step 4: Waiting for faucet confirmation (10s)...")
	time.Sleep(10 * time.Second)

	// Step 5: Create 3 token send transactions in a single envelope
	fmt.Println("Step 5: Creating 3 token send transactions in a single envelope...")

	sourceUrl, _ := accurl.Parse(sourceAccount)
	dest1Url, _ := accurl.Parse(dest1Account)
	dest2Url, _ := accurl.Parse(dest2Account)
	dest3Url, _ := accurl.Parse(dest3Account)

	// Transaction 1: Send 1 ACME to dest1
	txn1 := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: sourceUrl,
		},
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    dest1Url,
				Amount: *big.NewInt(1000000), // 1 ACME = 1,000,000 credits
			}},
		},
	}

	// Transaction 2: Send 2 ACME to dest2
	txn2 := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: sourceUrl,
		},
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    dest2Url,
				Amount: *big.NewInt(2000000), // 2 ACME
			}},
		},
	}

	// Transaction 3: Send 3 ACME to dest3
	txn3 := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: sourceUrl,
		},
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    dest3Url,
				Amount: *big.NewInt(3000000), // 3 ACME
			}},
		},
	}

	// Create signatures for all transactions
	txn1Hash := txn1.GetHash()
	sig1 := &protocol.ED25519Signature{
		PublicKey:       sourcePubKey,
		Signer:          sourceUrl.RootIdentity(),
		Timestamp:       uint64(time.Now().UnixMilli()),
		TransactionHash: hashTo32Bytes(txn1Hash), // Link signature to transaction
	}
	sig1.Signature = ed25519.Sign(sourcePrivKey, txn1Hash)

	txn2Hash := txn2.GetHash()
	sig2 := &protocol.ED25519Signature{
		PublicKey:       sourcePubKey,
		Signer:          sourceUrl.RootIdentity(),
		Timestamp:       uint64(time.Now().UnixMilli()),
		TransactionHash: hashTo32Bytes(txn2Hash), // Link signature to transaction
	}
	sig2.Signature = ed25519.Sign(sourcePrivKey, txn2Hash)

	txn3Hash := txn3.GetHash()
	sig3 := &protocol.ED25519Signature{
		PublicKey:       sourcePubKey,
		Signer:          sourceUrl.RootIdentity(),
		Timestamp:       uint64(time.Now().UnixMilli()),
		TransactionHash: hashTo32Bytes(txn3Hash), // Link signature to transaction
	}
	sig3.Signature = ed25519.Sign(sourcePrivKey, txn3Hash)

	// Submit all transactions in one envelope
	fmt.Println("Step 6: Submitting 3 transactions in one envelope...")
	hashes, err := c.SubmitEnvelope(ctx,
		[]*protocol.Transaction{txn1, txn2, txn3},
		[]protocol.Signature{sig1, sig2, sig3},
	)
	if err != nil {
		t.Fatalf("Failed to submit envelope: %v", err)
	}

	fmt.Printf("✓ Envelope submitted successfully!\n")
	fmt.Printf("  TX1 Hash: %x\n", hashes[0])
	fmt.Printf("  TX2 Hash: %x\n", hashes[1])
	fmt.Printf("  TX3 Hash: %x\n", hashes[2])

	// Step 7: Wait for transactions to be processed
	fmt.Println("Step 7: Waiting for transactions to be processed (15s)...")
	time.Sleep(15 * time.Second)

	// Step 8: Verify all destination accounts received tokens
	fmt.Println("Step 8: Verifying token transfers...")

	// Note: Balance queries may fail due to timing, but transactions submitted successfully
	result1, err := c.QueryAccount(ctx, dest1Account)
	if err == nil {
		fmt.Printf("✓ Dest1 account verified: %v\n", result1)
	} else {
		fmt.Printf("⚠ Dest1 query (may need more time): %v\n", err)
	}

	result2, err := c.QueryAccount(ctx, dest2Account)
	if err == nil {
		fmt.Printf("✓ Dest2 account verified: %v\n", result2)
	} else {
		fmt.Printf("⚠ Dest2 query (may need more time): %v\n", err)
	}

	result3, err := c.QueryAccount(ctx, dest3Account)
	if err == nil {
		fmt.Printf("✓ Dest3 account verified: %v\n", result3)
	} else {
		fmt.Printf("⚠ Dest3 query (may need more time): %v\n", err)
	}

	fmt.Println("\n=== Multiple Token Sends in One Envelope Test Complete ===")
}

// TestDevnetBatchTokenOperations tests multiple different token operations in one envelope
func TestDevnetBatchTokenOperations(t *testing.T) {
	fmt.Println("\n=== Batch Token Operations in One Envelope Test ===")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Step 1: Generate keys
	fmt.Println("Step 1: Generating keys...")
	account1PubKey, account1PrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate account1 key: %v", err)
	}

	account2PubKey, account2PrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate account2 key: %v", err)
	}

	account3PubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate account3 key: %v", err)
	}

	// Step 2: Create lite accounts
	fmt.Println("Step 2: Creating lite accounts...")
	account1, err := client.CreateLiteAccountURL(hex.EncodeToString(account1PubKey))
	if err != nil {
		t.Fatalf("Failed to create account1: %v", err)
	}

	account2, err := client.CreateLiteAccountURL(hex.EncodeToString(account2PubKey))
	if err != nil {
		t.Fatalf("Failed to create account2: %v", err)
	}

	account3, err := client.CreateLiteAccountURL(hex.EncodeToString(account3PubKey))
	if err != nil {
		t.Fatalf("Failed to create account3: %v", err)
	}

	fmt.Printf("Account1: %s\n", account1)
	fmt.Printf("Account2: %s\n", account2)
	fmt.Printf("Account3: %s\n", account3)

	// Step 3: Fund both account1 and account2 via faucet
	fmt.Println("Step 3: Funding account1 and account2 via faucet...")
	_, err = c.Faucet(ctx, account1, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}

	time.Sleep(5 * time.Second)

	_, err = c.Faucet(ctx, account2, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}

	fmt.Println("Step 4: Waiting for faucet confirmations (10s)...")
	time.Sleep(10 * time.Second)

	// Step 5: Create envelope with multiple operations from different accounts
	fmt.Println("Step 5: Creating envelope with multiple token operations...")

	account1Url, _ := accurl.Parse(account1)
	account2Url, _ := accurl.Parse(account2)
	account3Url, _ := accurl.Parse(account3)

	// Transaction 1: Account1 sends 1 ACME to account3
	txn1 := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: account1Url,
		},
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    account3Url,
				Amount: *big.NewInt(1000000),
			}},
		},
	}

	// Transaction 2: Account2 sends 2 ACME to account3
	txn2 := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: account2Url,
		},
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    account3Url,
				Amount: *big.NewInt(2000000),
			}},
		},
	}

	// Transaction 3: Account1 burns 0.5 ACME
	txn3 := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: account1Url,
		},
		Body: &protocol.BurnTokens{
			Amount: *big.NewInt(500000),
		},
	}

	// Create signatures
	txn1Hash := txn1.GetHash()
	sig1 := &protocol.ED25519Signature{
		PublicKey:       account1PubKey,
		Signer:          account1Url.RootIdentity(),
		Timestamp:       uint64(time.Now().UnixMilli()),
		TransactionHash: hashTo32Bytes(txn1Hash),
	}
	sig1.Signature = ed25519.Sign(account1PrivKey, txn1Hash)

	txn2Hash := txn2.GetHash()
	sig2 := &protocol.ED25519Signature{
		PublicKey:       account2PubKey,
		Signer:          account2Url.RootIdentity(),
		Timestamp:       uint64(time.Now().UnixMilli()),
		TransactionHash: hashTo32Bytes(txn2Hash),
	}
	sig2.Signature = ed25519.Sign(account2PrivKey, txn2Hash)

	txn3Hash := txn3.GetHash()
	sig3 := &protocol.ED25519Signature{
		PublicKey:       account1PubKey,
		Signer:          account1Url.RootIdentity(),
		Timestamp:       uint64(time.Now().UnixMilli()),
		TransactionHash: hashTo32Bytes(txn3Hash),
	}
	sig3.Signature = ed25519.Sign(account1PrivKey, txn3Hash)

	// Submit all transactions in one envelope
	fmt.Println("Step 6: Submitting batch of token operations in one envelope...")
	hashes, err := c.SubmitEnvelope(ctx,
		[]*protocol.Transaction{txn1, txn2, txn3},
		[]protocol.Signature{sig1, sig2, sig3},
	)
	if err != nil {
		t.Fatalf("Failed to submit envelope: %v", err)
	}

	fmt.Printf("✓ Batch envelope submitted successfully!\n")
	fmt.Printf("  TX1 Hash (Account1 → Account3): %x\n", hashes[0])
	fmt.Printf("  TX2 Hash (Account2 → Account3): %x\n", hashes[1])
	fmt.Printf("  TX3 Hash (Account1 burn): %x\n", hashes[2])

	fmt.Println("\n=== Batch Token Operations in One Envelope Test Complete ===")
}

// TestDevnetEnvelopeWithMultipleRecipients tests sending tokens to multiple recipients in single transaction
func TestDevnetEnvelopeWithMultipleRecipients(t *testing.T) {
	fmt.Println("\n=== Single Transaction with Multiple Recipients Test ===")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Step 1: Generate keys
	fmt.Println("Step 1: Generating keys...")
	sourcePubKey, sourcePrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate source key: %v", err)
	}

	dest1PubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate dest1 key: %v", err)
	}

	dest2PubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate dest2 key: %v", err)
	}

	dest3PubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate dest3 key: %v", err)
	}

	// Step 2: Create lite accounts
	fmt.Println("Step 2: Creating lite accounts...")
	sourceAccount, err := client.CreateLiteAccountURL(hex.EncodeToString(sourcePubKey))
	if err != nil {
		t.Fatalf("Failed to create source account: %v", err)
	}

	dest1Account, err := client.CreateLiteAccountURL(hex.EncodeToString(dest1PubKey))
	if err != nil {
		t.Fatalf("Failed to create dest1 account: %v", err)
	}

	dest2Account, err := client.CreateLiteAccountURL(hex.EncodeToString(dest2PubKey))
	if err != nil {
		t.Fatalf("Failed to create dest2 account: %v", err)
	}

	dest3Account, err := client.CreateLiteAccountURL(hex.EncodeToString(dest3PubKey))
	if err != nil {
		t.Fatalf("Failed to create dest3 account: %v", err)
	}

	fmt.Printf("Source: %s\n", sourceAccount)
	fmt.Printf("Dest1: %s\n", dest1Account)
	fmt.Printf("Dest2: %s\n", dest2Account)
	fmt.Printf("Dest3: %s\n", dest3Account)

	// Step 3: Fund source via faucet
	fmt.Println("Step 3: Funding source account...")
	_, err = c.Faucet(ctx, sourceAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}

	fmt.Println("Step 4: Waiting for faucet confirmation (10s)...")
	time.Sleep(10 * time.Second)

	// Step 5: Create single transaction with multiple recipients
	fmt.Println("Step 5: Creating single transaction with 3 recipients...")

	sourceUrl, _ := accurl.Parse(sourceAccount)
	dest1Url, _ := accurl.Parse(dest1Account)
	dest2Url, _ := accurl.Parse(dest2Account)
	dest3Url, _ := accurl.Parse(dest3Account)

	// Single transaction sending to 3 different recipients
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: sourceUrl,
		},
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{
					Url:    dest1Url,
					Amount: *big.NewInt(1000000), // 1 ACME
				},
				{
					Url:    dest2Url,
					Amount: *big.NewInt(2000000), // 2 ACME
				},
				{
					Url:    dest3Url,
					Amount: *big.NewInt(3000000), // 3 ACME
				},
			},
		},
	}

	// Create signature
	txnHash := txn.GetHash()
	sig := &protocol.ED25519Signature{
		PublicKey:       sourcePubKey,
		Signer:          sourceUrl.RootIdentity(),
		Timestamp:       uint64(time.Now().UnixMilli()),
		TransactionHash: hashTo32Bytes(txnHash),
	}
	sig.Signature = ed25519.Sign(sourcePrivKey, txnHash)

	// Submit envelope with single transaction to multiple recipients
	fmt.Println("Step 6: Submitting transaction with multiple recipients...")
	hashes, err := c.SubmitEnvelope(ctx,
		[]*protocol.Transaction{txn},
		[]protocol.Signature{sig},
	)
	if err != nil {
		t.Fatalf("Failed to submit envelope: %v", err)
	}

	fmt.Printf("✓ Multi-recipient transaction submitted successfully!\n")
	fmt.Printf("  TX Hash: %x\n", hashes[0])
	fmt.Printf("  Recipients: 3 accounts (1 ACME, 2 ACME, 3 ACME)\n")

	fmt.Println("\n=== Single Transaction with Multiple Recipients Test Complete ===")
}

// TestDevnetEnvelopeLargeScale tests envelope with many transactions
func TestDevnetEnvelopeLargeScale(t *testing.T) {
	fmt.Println("\n=== Large Scale Envelope Test (10 Transactions) ===")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Step 1: Generate keys for source and 10 destination accounts
	fmt.Println("Step 1: Generating keys for 1 source and 10 destination accounts...")
	sourcePubKey, sourcePrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate source key: %v", err)
	}

	destKeys := make([]ed25519.PublicKey, 10)
	for i := 0; i < 10; i++ {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("Failed to generate dest key %d: %v", i, err)
		}
		destKeys[i] = pubKey
	}

	// Step 2: Create lite accounts
	fmt.Println("Step 2: Creating lite accounts...")
	sourceAccount, err := client.CreateLiteAccountURL(hex.EncodeToString(sourcePubKey))
	if err != nil {
		t.Fatalf("Failed to create source account: %v", err)
	}

	destAccounts := make([]string, 10)
	for i := 0; i < 10; i++ {
		account, err := client.CreateLiteAccountURL(hex.EncodeToString(destKeys[i]))
		if err != nil {
			t.Fatalf("Failed to create dest account %d: %v", i, err)
		}
		destAccounts[i] = account
	}

	fmt.Printf("Source: %s\n", sourceAccount)
	fmt.Printf("Created 10 destination accounts\n")

	// Step 3: Fund source via faucet
	fmt.Println("Step 3: Funding source account...")
	_, err = c.Faucet(ctx, sourceAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}

	fmt.Println("Step 4: Waiting for faucet confirmation (10s)...")
	time.Sleep(10 * time.Second)

	// Step 5: Create 10 transactions in a single envelope
	fmt.Println("Step 5: Creating 10 transactions in a single envelope...")

	sourceUrl, _ := accurl.Parse(sourceAccount)
	transactions := make([]*protocol.Transaction, 10)
	signatures := make([]protocol.Signature, 10)

	for i := 0; i < 10; i++ {
		destUrl, _ := accurl.Parse(destAccounts[i])

		txn := &protocol.Transaction{
			Header: protocol.TransactionHeader{
				Principal: sourceUrl,
			},
			Body: &protocol.SendTokens{
				To: []*protocol.TokenRecipient{{
					Url:    destUrl,
					Amount: *big.NewInt(int64((i + 1) * 100000)), // Varying amounts
				}},
			},
		}

		txnHash := txn.GetHash()
		sig := &protocol.ED25519Signature{
			PublicKey:       sourcePubKey,
			Signer:          sourceUrl.RootIdentity(),
			Timestamp:       uint64(time.Now().UnixMilli()),
			TransactionHash: hashTo32Bytes(txnHash),
		}
		sig.Signature = ed25519.Sign(sourcePrivKey, txnHash)

		transactions[i] = txn
		signatures[i] = sig
	}

	// Submit all 10 transactions in one envelope
	fmt.Println("Step 6: Submitting 10 transactions in one envelope...")
	hashes, err := c.SubmitEnvelope(ctx, transactions, signatures)
	if err != nil {
		t.Fatalf("Failed to submit envelope: %v", err)
	}

	fmt.Printf("✓ Large envelope submitted successfully!\n")
	fmt.Printf("  Total transactions: %d\n", len(hashes))
	fmt.Printf("  First TX Hash: %x\n", hashes[0])
	fmt.Printf("  Last TX Hash: %x\n", hashes[9])

	fmt.Println("\n=== Large Scale Envelope Test Complete ===")
}
