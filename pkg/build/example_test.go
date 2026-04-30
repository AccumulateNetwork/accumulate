// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Example_createIdentity demonstrates how to create an ADI (Accumulate Digital
// Identity) using the build package. This is the recommended approach for
// building and signing transactions.
func Example_createIdentity() {
	// Generate a new key pair
	_, privateKey, _ := ed25519.GenerateKey(nil)
	publicKey := privateKey[32:]

	// Derive the lite account URL from the public key
	liteAccount := protocol.LiteAuthorityForKey(publicKey, protocol.SignatureTypeED25519)
	liteTokenAccount := liteAccount.JoinPath("ACME")

	// Create the identity URL and key book URL
	identityUrl, _ := url.Parse("acc://my-identity.acme")
	keyBookUrl := identityUrl.JoinPath("book")

	// Compute the key hash (SHA256 of public key)
	keyHash := sha256.Sum256(publicKey)

	// Build and sign the CreateIdentity transaction
	env, err := build.Transaction().
		For(liteTokenAccount).                   // Sponsor/principal account
		CreateIdentity(identityUrl).             // Create identity at this URL
		WithKeyHash(keyHash[:]).                 // Initial key hash
		WithKeyBook(keyBookUrl).                 // Key book URL
		SignWith(liteAccount).                   // Sign with lite identity
		Version(1).                              // Signer version
		Timestamp(time.Now().UnixMilli()).       // Timestamp (required for initiator)
		PrivateKey(privateKey).                  // 64-byte ED25519 private key
		Done()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Transaction type: %v\n", env.Transaction[0].Body.Type())
	fmt.Printf("Signature count: %d\n", len(env.Signatures))
	// Output:
	// Transaction type: createIdentity
	// Signature count: 1
}

// Example_sendTokens demonstrates how to send ACME tokens from one account to
// another.
func Example_sendTokens() {
	_, privateKey, _ := ed25519.GenerateKey(nil)
	publicKey := privateKey[32:]

	// Sender's lite token account
	sender := protocol.LiteAuthorityForKey(publicKey, protocol.SignatureTypeED25519)
	senderTokens := sender.JoinPath("ACME")

	// Recipient URL (any valid ACME token account)
	recipient, _ := url.Parse("acc://recipient.acme/tokens")

	// Amount to send: 10.5 ACME (amount is in smallest units, precision=8)
	// 10.5 ACME = 1050000000 units (10.5 * 10^8)
	amount := uint64(1050000000)

	// Build the transaction
	env, err := build.Transaction().
		For(senderTokens).                       // Sender's token account
		SendTokens(amount, 0).To(recipient).     // Send tokens (precision=0 means use raw amount)
		SignWith(sender).                        // Sign with sender's lite identity
		Version(1).
		Timestamp(time.Now().UnixMilli()).
		PrivateKey(privateKey).
		Done()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Transaction type: %v\n", env.Transaction[0].Body.Type())
	// Output:
	// Transaction type: sendTokens
}

// Example_multipleSignatures demonstrates how to create a transaction with
// multiple signatures, which is required for multi-sig accounts.
func Example_multipleSignatures() {
	// Generate two key pairs
	_, key1, _ := ed25519.GenerateKey(nil)
	_, key2, _ := ed25519.GenerateKey(nil)

	identityUrl, _ := url.Parse("acc://multi-sig.acme")
	tokenAccount := identityUrl.JoinPath("tokens")
	keyPage := identityUrl.JoinPath("book/1")

	recipient, _ := url.Parse("acc://recipient.acme/tokens")

	// Use a timestamp variable - the builder will increment it automatically
	// to ensure each signature has a unique timestamp
	var timestamp uint64 = uint64(time.Now().UnixMilli())

	env, err := build.Transaction().
		For(tokenAccount).
		SendTokens(100, 0).To(recipient).
		// First signature
		SignWith(keyPage).Version(1).Timestamp(&timestamp).PrivateKey(key1).
		// Second signature (timestamp auto-increments)
		SignWith(keyPage).Version(1).Timestamp(&timestamp).PrivateKey(key2).
		Done()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Transaction type: %v\n", env.Transaction[0].Body.Type())
	fmt.Printf("Signature count: %d\n", len(env.Signatures))
	// Output:
	// Transaction type: sendTokens
	// Signature count: 2
}

// Example_delegatedSignature demonstrates how to sign a transaction using a
// delegated authority.
func Example_delegatedSignature() {
	_, privateKey, _ := ed25519.GenerateKey(nil)

	// The account being delegated to sign
	delegatedSigner, _ := url.Parse("acc://delegate.acme/book/1")

	// The authority that delegated signing rights
	delegator, _ := url.Parse("acc://principal.acme/book")

	// The principal account for the transaction
	principal, _ := url.Parse("acc://principal.acme/tokens")
	recipient, _ := url.Parse("acc://recipient.acme/tokens")

	env, err := build.Transaction().
		For(principal).
		SendTokens(100, 0).To(recipient).
		SignWith(delegatedSigner).
		Delegator(delegator).            // Add the delegation chain
		Version(1).
		Timestamp(time.Now().UnixMilli()).
		PrivateKey(privateKey).
		Done()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// The signature should be a DelegatedSignature
	sig := env.Signatures[0]
	fmt.Printf("Signature type: %v\n", sig.Type())
	// Output:
	// Signature type: delegated
}

// Example_writeData demonstrates how to write data to a data account.
func Example_writeData() {
	_, privateKey, _ := ed25519.GenerateKey(nil)

	dataAccount, _ := url.Parse("acc://my-identity.acme/data")
	keyPage, _ := url.Parse("acc://my-identity.acme/book/1")

	env, err := build.Transaction().
		For(dataAccount).
		WriteData([]byte("entry1"), []byte("entry2"), []byte("entry3")).
		SignWith(keyPage).
		Version(1).
		Timestamp(time.Now().UnixMilli()).
		PrivateKey(privateKey).
		Done()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Transaction type: %v\n", env.Transaction[0].Body.Type())
	// Output:
	// Transaction type: writeData
}

// Example_submitTransaction demonstrates the complete flow of building,
// signing, and submitting a transaction to the network.
func Example_submitTransaction() {
	// This example shows the full submission flow but doesn't actually submit
	// since we don't have a live network connection in tests.

	_, privateKey, _ := ed25519.GenerateKey(nil)
	publicKey := privateKey[32:]

	liteAccount := protocol.LiteAuthorityForKey(publicKey, protocol.SignatureTypeED25519)
	liteTokenAccount := liteAccount.JoinPath("ACME")

	// Step 1: Build the envelope
	env, err := build.Transaction().
		For(liteTokenAccount).
		SendTokens(100, 0).To(liteTokenAccount). // Send to self
		SignWith(liteAccount).
		Version(1).
		Timestamp(time.Now().UnixMilli()).
		PrivateKey(privateKey).
		Done()

	if err != nil {
		fmt.Printf("Build error: %v\n", err)
		return
	}

	// Step 2: Create API client
	client := jsonrpc.NewClient("https://testnet.accumulatenetwork.io/v3")
	ctx := context.Background()

	// Step 3: Submit the envelope
	// (Commented out to avoid actual network calls in tests)
	_ = client
	_ = ctx
	_ = env
	/*
		submissions, err := client.Submit(ctx, env, api.SubmitOptions{})
		if err != nil {
			fmt.Printf("Submit error: %v\n", err)
			return
		}

		// Step 4: Check submission results
		for i, sub := range submissions {
			if sub.Status.Failed() {
				fmt.Printf("Submission %d failed: %v\n", i, sub.Status.AsError())
			} else {
				fmt.Printf("Submission %d succeeded: %v\n", i, sub.Status.TxID)
			}
		}
	*/

	fmt.Printf("Envelope built successfully\n")
	// Output:
	// Envelope built successfully
}

// Example_useFaucet demonstrates how to request tokens from the testnet faucet.
func Example_useFaucet() {
	_, privateKey, _ := ed25519.GenerateKey(nil)
	publicKey := privateKey[32:]

	// Get the lite token account URL
	liteAccount := protocol.LiteAuthorityForKey(publicKey, protocol.SignatureTypeED25519)
	liteTokenAccount := liteAccount.JoinPath("ACME")

	// Build a faucet request
	env, err := build.Faucet(liteTokenAccount).Done()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Transaction type: %v\n", env.Transaction[0].Body.Type())
	// Output:
	// Transaction type: acmeFaucet
}

// Example_signWithBuilder demonstrates using the lower-level signing.Builder
// for more control over signature construction.
func Example_signWithBuilder() {
	_, privateKey, _ := ed25519.GenerateKey(nil)
	publicKey := privateKey[32:]

	signerUrl := protocol.LiteAuthorityForKey(publicKey, protocol.SignatureTypeED25519)
	liteTokenAccount := signerUrl.JoinPath("ACME")

	// Create the transaction manually
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: liteTokenAccount,
		},
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    liteTokenAccount,
				Amount: *big.NewInt(100),
			}},
		},
	}

	// Use signing.Builder for more control
	builder := &signing.Builder{
		Type:      protocol.SignatureTypeED25519,
		Url:       signerUrl,
		Version:   1,
		Timestamp: signing.TimestampFromValue(uint64(time.Now().UnixMilli())),
		Signer:    signing.PrivateKey(privateKey),
	}

	// Initiate sets the transaction's initiator hash and signs
	sig, err := builder.Initiate(txn)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Create the envelope
	env := &messaging.Envelope{
		Transaction: []*protocol.Transaction{txn},
		Signatures:  []protocol.Signature{sig},
	}

	fmt.Printf("Transaction initiated: %v\n", txn.Header.Initiator != [32]byte{})
	fmt.Printf("Envelope ready: %v\n", env != nil)
	// Output:
	// Transaction initiated: true
	// Envelope ready: true
}

// Example_wrongWayToSign demonstrates the INCORRECT way to sign a transaction.
// This is included to show what NOT to do.
func Example_wrongWayToSign() {
	_, privateKey, _ := ed25519.GenerateKey(nil)
	publicKey := privateKey[32:]

	signerUrl := protocol.LiteAuthorityForKey(publicKey, protocol.SignatureTypeED25519)
	liteTokenAccount := signerUrl.JoinPath("ACME")

	// Create the transaction
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: liteTokenAccount,
		},
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    liteTokenAccount,
				Amount: *big.NewInt(100),
			}},
		},
	}

	// WRONG: Manually creating signature and signing just the transaction hash
	// This will be REJECTED by the network!
	sig := &protocol.ED25519Signature{
		PublicKey:     publicKey,
		Signer:        signerUrl,
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().UnixMilli()),
	}

	// This is WRONG - do not do this!
	// The network expects: SHA256(signature_metadata_hash || transaction_hash)
	// Not just: transaction_hash
	wrongSignature := ed25519.Sign(privateKey, txn.GetHash())
	_ = wrongSignature // Don't actually use this!

	// CORRECT: Use protocol.SignED25519 which properly computes the signing hash
	protocol.SignED25519(sig, privateKey, nil, txn.GetHash())

	// Or better yet, use the build package as shown in other examples
	fmt.Printf("Use build.Transaction() or signing.Builder instead of manual signing\n")
	// Output:
	// Use build.Transaction() or signing.Builder instead of manual signing
}

// Example_addCredits demonstrates how to purchase credits by burning ACME tokens.
func Example_addCredits() {
	_, privateKey, _ := ed25519.GenerateKey(nil)
	publicKey := privateKey[32:]

	// Source ACME token account (must have ACME balance)
	liteAccount := protocol.LiteAuthorityForKey(publicKey, protocol.SignatureTypeED25519)
	liteTokenAccount := liteAccount.JoinPath("ACME")

	// Credit recipient (can be the lite identity itself or any key page)
	creditRecipient := liteAccount

	// Oracle price (credits per ACME as a float, typically fetched from network)
	// For example: 500 credits per ACME
	oracle := 500.0

	// Amount of credits to purchase (as a float)
	creditAmount := 1000.0 // 1000 credits

	env, err := build.Transaction().
		For(liteTokenAccount).
		AddCredits().
		To(creditRecipient).
		WithOracle(oracle).
		Purchase(creditAmount).
		SignWith(liteAccount).
		Version(1).
		Timestamp(time.Now().UnixMilli()).
		PrivateKey(privateKey).
		Done()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Transaction type: %v\n", env.Transaction[0].Body.Type())
	// Output:
	// Transaction type: addCredits
}

// Example_waitForTransaction demonstrates how to wait for a transaction to be
// confirmed after submission.
func Example_waitForTransaction() {
	// This example shows how to poll for transaction status
	// In practice, you would use the actual API client

	client := jsonrpc.NewClient("https://testnet.accumulatenetwork.io/v3")
	ctx := context.Background()

	// After submitting, you get a transaction ID
	// txID := submissions[0].Status.TxID

	// Poll for the transaction status
	_ = client
	_ = ctx
	/*
		for {
			resp, err := client.Query(ctx, txID, nil)
			if err != nil {
				fmt.Printf("Query error: %v\n", err)
				time.Sleep(time.Second)
				continue
			}

			status := resp.(*api.TransactionRecord).Status
			if status.Delivered() {
				if status.Failed() {
					fmt.Printf("Transaction failed: %v\n", status.AsError())
				} else {
					fmt.Printf("Transaction succeeded!\n")
				}
				break
			}

			time.Sleep(time.Second)
		}
	*/

	fmt.Printf("Poll for transaction status after submission\n")
	// Output:
	// Poll for transaction status after submission
}

// Example_signatureTypes demonstrates different signature types supported by
// Accumulate.
func Example_signatureTypes() {
	_, ed25519Key, _ := ed25519.GenerateKey(nil)
	publicKey := ed25519Key[32:]

	liteAccount := protocol.LiteAuthorityForKey(publicKey, protocol.SignatureTypeED25519)
	liteTokenAccount := liteAccount.JoinPath("ACME")

	// ED25519 (default)
	env1, _ := build.Transaction().
		For(liteTokenAccount).
		SendTokens(100, 0).To(liteTokenAccount).
		SignWith(liteAccount).
		Type(protocol.SignatureTypeED25519). // This is the default
		Version(1).
		Timestamp(time.Now().UnixMilli()).
		PrivateKey(ed25519Key).
		Done()

	// Legacy ED25519 (for backwards compatibility)
	env2, _ := build.Transaction().
		For(liteTokenAccount).
		SendTokens(100, 0).To(liteTokenAccount).
		SignWith(liteAccount).
		Type(protocol.SignatureTypeLegacyED25519).
		Version(1).
		Timestamp(time.Now().UnixMilli()).
		PrivateKey(ed25519Key).
		Done()

	// Note: BTC, ETH, and RCD1 signatures require different key formats
	// and are typically used with hardware wallets or specific integrations

	fmt.Printf("ED25519 signature type: %v\n", env1.Signatures[0].Type())
	fmt.Printf("LegacyED25519 signature type: %v\n", env2.Signatures[0].Type())
	// Output:
	// ED25519 signature type: ed25519
	// LegacyED25519 signature type: legacyED25519
}

// The following are stub implementations to make the examples compile
var _ api.Submitter = (*jsonrpc.Client)(nil)
