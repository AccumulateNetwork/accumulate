// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"flag"
	"log"
	"math/big"
	"time"

	api "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var serverUrl string
var targetTPS int
var durationSec int

func init() {
	flag.StringVar(&serverUrl, "s", "http://127.0.1.1:26660/v2", "Accumulate server URL")
	flag.IntVar(&targetTPS, "tps", 10, "Target transactions per second")
	flag.IntVar(&durationSec, "d", 10, "Duration in seconds")
}

func main() {
	flag.Parse()

	log.Printf("Starting load test with pre-funded accounts: %d TPS for %d seconds", targetTPS, durationSec)
	log.Printf("Server: %s", serverUrl)

	// Create client
	cl, err := client.New(serverUrl)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Use the 100 pre-generated accounts (index 0-99)
	numAccounts := 100
	accounts := make([]*account, numAccounts)
	for i := 0; i < numAccounts; i++ {
		seed := make([]byte, 32)
		for j := range seed {
			seed[j] = byte(i)
		}
		privKey := ed25519.NewKeyFromSeed(seed)
		pubKey := privKey.Public().(ed25519.PublicKey)

		acctUrl, err := protocol.LiteTokenAddress(pubKey, "ACME", protocol.SignatureTypeED25519)
		if err != nil {
			log.Fatalf("Error creating account %d: %v", i, err)
		}

		accounts[i] = &account{
			url:     acctUrl,
			privKey: privKey,
			pubKey:  pubKey,
		}
	}

	log.Printf("Using %d pre-funded accounts", numAccounts)

	start := time.Now()
	successCount := 0
	failCount := 0
	totalTxs := targetTPS * durationSec

	// Submit transactions at the target rate
	ticker := time.NewTicker(time.Second / time.Duration(targetTPS))
	defer ticker.Stop()

	ctx := context.Background()
	txCount := 0

	for txCount < totalTxs {
		<-ticker.C

		// Round-robin between accounts: send from account[i] to account[(i+1) % numAccounts]
		sender := accounts[txCount%numAccounts]
		recipient := accounts[(txCount+1)%numAccounts]

		// Create send tokens transaction
		body := &protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    recipient.url,
				Amount: *big.NewInt(100000), // 0.001 ACME
			}},
		}

		// Build transaction
		txn := &protocol.Transaction{
			Header: protocol.TransactionHeader{
				Principal: sender.url,
			},
			Body: body,
		}

		// Sign with initiator signature
		sig := new(protocol.LegacyED25519Signature)
		sig.Signer = sender.url.RootIdentity()
		sig.SignerVersion = 1
		sig.Timestamp = uint64(time.Now().UnixMilli())
		sig.PublicKey = sender.pubKey

		// Compute transaction hash
		txnHash := txn.GetHash()
		sigHash := sha256.Sum256(append(txnHash[:], sig.Metadata().Hash()...))
		sig.Signature = ed25519.Sign(sender.privKey, sigHash[:])

		// Build envelope
		env := &messaging.Envelope{
			Transaction: []*protocol.Transaction{txn},
			Signatures:  []protocol.Signature{sig},
		}

		// Submit transaction using ExecuteDirect
		resp, err := cl.ExecuteDirect(ctx, &api.ExecuteRequest{Envelope: env})
		if err != nil {
			log.Printf("Error submitting tx #%d: %v", txCount+1, err)
			failCount++
		} else if resp.Code != 0 {
			log.Printf("Tx #%d failed with code %d: %s", txCount+1, resp.Code, resp.Message)
			failCount++
		} else {
			if txCount < 5 || txCount%10 == 0 {
				log.Printf("✓ Tx #%d submitted: %x", txCount+1, resp.TransactionHash[:8])
			}
			successCount++
		}

		txCount++

		// Log progress every 10 transactions
		if txCount%10 == 0 {
			elapsed := time.Since(start).Seconds()
			currentTPS := float64(txCount) / elapsed
			log.Printf("Progress: %d/%d txs (%.1f TPS, success: %d, failed: %d)",
				txCount, totalTxs, currentTPS, successCount, failCount)
		}
	}

	elapsed := time.Since(start)
	actualTPS := float64(txCount) / elapsed.Seconds()

	log.Printf("\n========== Results ==========")
	log.Printf("Total transactions: %d", txCount)
	log.Printf("Successful: %d (%.1f%%)", successCount, 100.0*float64(successCount)/float64(txCount))
	log.Printf("Failed: %d (%.1f%%)", failCount, 100.0*float64(failCount)/float64(txCount))
	log.Printf("Duration: %.2f seconds", elapsed.Seconds())
	log.Printf("Actual TPS: %.2f", actualTPS)
	log.Printf("=============================")
}

type account struct {
	url     *url.URL
	privKey ed25519.PrivateKey
	pubKey  ed25519.PublicKey
}
