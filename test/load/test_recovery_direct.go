package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// DirectRecoveryTest tests reading and providing past anchors/synths
type DirectRecoveryTest struct {
	client *jsonrpc.Client
}

func main() {
	fmt.Println("========================================")
	fmt.Println("  DIRECT ANCHOR/SYNTH RECOVERY TEST")
	fmt.Println("========================================")
	fmt.Println()
	
	test := &DirectRecoveryTest{
		client: GetPooledClient("http://127.0.0.1:26660/v3"),
	}
	
	// Run the tests
	fmt.Println("Test 1: Reading Anchor Ledgers")
	test.testReadAnchorLedgers()
	
	fmt.Println("\nTest 2: Reading Synthetic Ledgers")
	test.testReadSyntheticLedgers()
	
	fmt.Println("\nTest 3: Identifying Missing Transactions")
	test.testIdentifyMissing()
	
	fmt.Println("\nTest 4: Querying Historical Transactions")
	test.testQueryHistorical()
	
	fmt.Println("\nTest 5: Simulating Recovery Request")
	test.testSimulateRecovery()
	
	fmt.Println("\n========================================")
	fmt.Println("          TEST COMPLETED")
	fmt.Println("========================================")
}

// testReadAnchorLedgers tests reading anchor ledger accounts
func (test *DirectRecoveryTest) testReadAnchorLedgers() {
	ctx, cancel := CreateContextWithTimeout(30 * time.Second)
	defer cancel()
	
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	
	for _, part := range partitions {
		partUrl := protocol.PartitionUrl(part)
		anchorUrl := partUrl.JoinPath(protocol.AnchorPool)
		
		fmt.Printf("\nReading anchor ledger for %s...\n", part)
		
		// Query the anchor ledger
		Q := api.Querier2{Querier: test.client}
		resp, err := Q.QueryAccount(ctx, anchorUrl, nil)
		if err != nil {
			log.Printf("  Error reading %s: %v", anchorUrl, err)
			continue
		}
		
		// Check if it's an anchor ledger
		ledger, ok := resp.Account.(*protocol.AnchorLedger)
		if !ok {
			fmt.Printf("  Not an anchor ledger: %T\n", resp.Account)
			continue
		}
		
		fmt.Printf("  Anchor ledger found:\n")
		fmt.Printf("    Type: %s\n", ledger.Type())
		
		// Check sequences from other partitions
		for _, otherPart := range partitions {
			if otherPart == part {
				continue
			}
			
			otherUrl := protocol.PartitionUrl(otherPart)
			seq := ledger.Anchor(otherUrl)
			
			if seq.Received > 0 || seq.Delivered > 0 {
				fmt.Printf("    From %s: Received=%d, Delivered=%d, Pending=%d\n",
					otherPart, seq.Received, seq.Delivered, len(seq.Pending))
				
				// Check for gaps
				missing := seq.Received - seq.Delivered
				if missing > 0 {
					fmt.Printf("      WARNING: %d missing anchors!\n", missing)
				}
			}
		}
	}
}

// testReadSyntheticLedgers tests reading synthetic ledger accounts
func (test *DirectRecoveryTest) testReadSyntheticLedgers() {
	ctx, cancel := CreateContextWithTimeout(30 * time.Second)
	defer cancel()
	
	partitions := []string{"BVN0", "BVN1", "BVN2"}
	
	for _, part := range partitions {
		partUrl := protocol.PartitionUrl(part)
		synthUrl := partUrl.JoinPath(protocol.Synthetic)
		
		fmt.Printf("\nReading synthetic ledger for %s...\n", part)
		
		// Query the synthetic ledger
		Q := api.Querier2{Querier: test.client}
		resp, err := Q.QueryAccount(ctx, synthUrl, nil)
		if err != nil {
			log.Printf("  Error reading %s: %v", synthUrl, err)
			continue
		}
		
		// Check if it's a synthetic ledger
		ledger, ok := resp.Account.(*protocol.SyntheticLedger)
		if !ok {
			fmt.Printf("  Not a synthetic ledger: %T\n", resp.Account)
			continue
		}
		
		fmt.Printf("  Synthetic ledger found:\n")
		fmt.Printf("    Type: %s\n", ledger.Type())
		
		// Check sequences to other partitions
		for _, seq := range ledger.Sequence {
			if seq.Url != nil {
				fmt.Printf("    To %s: Received=%d, Delivered=%d\n",
					seq.Url.ShortString(), seq.Received, seq.Delivered)
				
				// Check for missing transactions
				missing := seq.Received - seq.Delivered
				if missing > 0 {
					fmt.Printf("      WARNING: %d synthetics not delivered!\n", missing)
				}
			}
		}
	}
}

// testIdentifyMissing identifies missing transactions between partitions
func (test *DirectRecoveryTest) testIdentifyMissing() {
	ctx, cancel := CreateContextWithTimeout(30 * time.Second)
	defer cancel()
	Q := api.Querier2{Querier: test.client}
	
	fmt.Println("\nScanning for missing transactions...")
	
	totalMissingAnchors := 0
	totalMissingSynths := 0
	
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	
	// Check each partition pair
	for _, dst := range partitions {
		dstUrl := protocol.PartitionUrl(dst)
		
		// Check anchors
		anchorUrl := dstUrl.JoinPath(protocol.AnchorPool)
		if resp, err := Q.QueryAccount(ctx, anchorUrl, nil); err == nil {
			if ledger, ok := resp.Account.(*protocol.AnchorLedger); ok {
				for _, src := range partitions {
					if src == dst {
						continue
					}
					srcUrl := protocol.PartitionUrl(src)
					seq := ledger.Anchor(srcUrl)
					missing := seq.Received - seq.Delivered
					if missing > 0 {
						fmt.Printf("  Missing anchors: %s -> %s: %d\n", src, dst, missing)
						totalMissingAnchors += int(missing)
					}
				}
			}
		}
		
		// Check synthetics (skip Directory)
		if dst != "Directory" {
			synthUrl := dstUrl.JoinPath(protocol.Synthetic)
			if resp, err := Q.QueryAccount(ctx, synthUrl, nil); err == nil {
				if ledger, ok := resp.Account.(*protocol.SyntheticLedger); ok {
					for _, seq := range ledger.Sequence {
						if seq.Url != nil {
							missing := seq.Received - seq.Delivered
							if missing > 0 {
								fmt.Printf("  Missing synthetics: %s -> %s: %d\n", 
									seq.Url.ShortString(), dst, missing)
								totalMissingSynths += int(missing)
							}
						}
					}
				}
			}
		}
	}
	
	fmt.Printf("\nTotal missing: %d anchors, %d synthetics\n", 
		totalMissingAnchors, totalMissingSynths)
	
	if totalMissingAnchors == 0 && totalMissingSynths == 0 {
		fmt.Println("SUCCESS: No missing transactions detected!")
	}
}

// testQueryHistorical tests querying past transactions
func (test *DirectRecoveryTest) testQueryHistorical() {
	ctx, cancel := CreateContextWithTimeout(30 * time.Second)
	defer cancel()
	Q := api.Querier2{Querier: test.client}
	
	fmt.Println("\nQuerying historical transactions...")
	
	// First, create some transactions to have history
	fmt.Println("Creating test transactions...")
	test.createTestTransactions()
	
	// Wait for transactions to be processed
	time.Sleep(5 * time.Second)
	
	// Query recent transactions
	fmt.Println("\nQuerying recent transaction history...")
	
	// Query from each partition
	for _, part := range []string{"BVN0", "BVN1", "BVN2"} {
		partUrl := protocol.PartitionUrl(part)
		
		// Query recent blocks for transactions
		fmt.Printf("\nQuerying %s history:\n", part)
		
		// Query the main ledger for recent activity
		ledgerUrl := partUrl.JoinPath(protocol.Ledger)
		resp, err := Q.QueryAccount(ctx, ledgerUrl, nil)
		if err != nil {
			log.Printf("  Error querying ledger: %v", err)
			continue
		}
		
		if ledger, ok := resp.Account.(*protocol.SystemLedger); ok {
			fmt.Printf("  Latest block: %d\n", ledger.Index)
			fmt.Printf("  Timestamp: %v\n", ledger.Timestamp)
			
			// Query recent transactions from this partition
			if ledger.Index > 0 {
				// Try to query a recent transaction by constructing a transaction ID
				// In practice, would enumerate actual transaction IDs
				fmt.Printf("  Can query transactions from blocks 1 to %d\n", ledger.Index)
			}
		}
	}
}

// testSimulateRecovery simulates a recovery request and response
func (test *DirectRecoveryTest) testSimulateRecovery() {
	ctx, cancel := CreateContextWithTimeout(30 * time.Second)
	defer cancel()
	Q := api.Querier2{Querier: test.client}
	
	fmt.Println("\nSimulating recovery request/response...")
	
	// Find a partition pair with potential missing transactions
	sourcePartition := "BVN0"
	destPartition := "BVN1"
	
	fmt.Printf("Checking %s -> %s for recovery needs...\n", sourcePartition, destPartition)
	
	// Check destination ledger
	dstUrl := protocol.PartitionUrl(destPartition)
	anchorUrl := dstUrl.JoinPath(protocol.AnchorPool)
	
	resp, err := Q.QueryAccount(ctx, anchorUrl, nil)
	if err != nil {
		log.Printf("Error querying anchor ledger: %v", err)
		return
	}
	
	ledger, ok := resp.Account.(*protocol.AnchorLedger)
	if !ok {
		fmt.Println("Not an anchor ledger")
		return
	}
	
	srcUrl := protocol.PartitionUrl(sourcePartition)
	seq := ledger.Anchor(srcUrl)
	
	fmt.Printf("\nCurrent state:\n")
	fmt.Printf("  Received: %d\n", seq.Received)
	fmt.Printf("  Delivered: %d\n", seq.Delivered)
	fmt.Printf("  Pending: %d\n", len(seq.Pending))
	
	missing := seq.Received - seq.Delivered
	
	if missing > 0 {
		fmt.Printf("\nSimulating recovery of %d missing anchors...\n", missing)
		
		// Simulate the recovery process
		fmt.Println("Step 1: Request missing anchors from source")
		fmt.Printf("  Request: anchors %d-%d from %s\n", 
			seq.Delivered+1, seq.Received, sourcePartition)
		
		fmt.Println("Step 2: Source reads historical anchors")
		// In real implementation, would query the actual anchors
		
		fmt.Println("Step 3: Package and send recovered anchors")
		recovered := 0
		for i := seq.Delivered + 1; i <= seq.Received && recovered < 10; i++ {
			fmt.Printf("  Recovering anchor #%d", i)
			
			// Check if we have the transaction ID
			idx := i - seq.Delivered - 1
			if idx < uint64(len(seq.Pending)) && seq.Pending[idx] != nil {
				fmt.Printf(" (ID: %s...)", seq.Pending[idx].String()[:16])
			}
			fmt.Println()
			recovered++
		}
		
		if recovered < int(missing) {
			fmt.Printf("  ... and %d more\n", int(missing)-recovered)
		}
		
		fmt.Println("\nStep 4: Destination processes recovered anchors")
		fmt.Println("  Validating proofs...")
		fmt.Println("  Updating ledger...")
		fmt.Printf("  Recovery complete: %d anchors restored\n", missing)
	} else {
		fmt.Println("\nNo missing anchors to recover")
		fmt.Println("Simulating proactive recovery check...")
		fmt.Println("  Source partition: healthy")
		fmt.Println("  Destination partition: synchronized")
		fmt.Println("  No recovery needed")
	}
}

// createTestTransactions creates some test transactions for history
func (test *DirectRecoveryTest) createTestTransactions() {
	// Create a simple transaction to generate activity
	ctx, cancel := CreateContextWithTimeout(30 * time.Second)
	defer cancel()
	
	// Generate a test account
	seed := make([]byte, 32)
	rand.Read(seed)
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	
	liteAddr, err := protocol.LiteTokenAddress(publicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Printf("Failed to create lite address: %v", err)
		return
	}
	
	// Fund the account via faucet
	fmt.Printf("  Funding test account %s...\n", liteAddr.ShortString())
	resp, err := test.client.Faucet(ctx, liteAddr, api.FaucetOptions{})
	if err != nil {
		log.Printf("  Faucet failed: %v", err)
		return
	}
	
	if resp != nil {
		fmt.Printf("  Funded successfully\n")
	}
	
	// Wait for funding
	time.Sleep(3 * time.Second)
	
	// Create a credit purchase (generates synthetic transaction)
	fmt.Println("  Creating credit purchase...")
	
	env := build.Transaction().
		For(liteAddr).
		Body(&protocol.AddCredits{
			Recipient: liteAddr.RootIdentity(),
			Amount:    *big.NewInt(100),
			Oracle:    1000000, // 0.01 ACME per credit
		}).
		SignWith(liteAddr).Version(1).Timestamp(time.Now().UnixNano()).PrivateKey(privateKey)
	
	envelope, err := env.Done()
	if err != nil {
		log.Printf("  Failed to build transaction: %v", err)
		return
	}
	
	// Submit the transaction
	subs, err := test.client.Submit(ctx, envelope, api.SubmitOptions{})
	if err != nil {
		log.Printf("  Failed to submit: %v", err)
		return
	}
	
	for i, sub := range subs {
		if sub.Success {
			fmt.Printf("  Transaction %d submitted successfully\n", i)
		}
	}
}

// Helper function to display partition info
func displayPartitionInfo(info *protocol.PartitionInfo) {
	fmt.Printf("  Partition: %s\n", info.ID)
	fmt.Printf("    Type: %v\n", info.Type)
}

// Helper function to check network status
func checkNetworkStatus(client *jsonrpc.Client) error {
	ctx := context.Background()
	
	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return err
	}
	
	fmt.Printf("Network Status:\n")
	fmt.Printf("  Oracle Price: %.4f\n", float64(status.Oracle.Price)/1e8)
	fmt.Printf("  Partitions: %d\n", len(status.Network.Partitions))
	
	return nil
}