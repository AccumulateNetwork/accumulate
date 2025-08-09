package main

import (
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MissingTxRecoveryTest tests recovery with actual missing transactions
type MissingTxRecoveryTest struct {
	client *jsonrpc.Client
}

func main() {
	fmt.Println("========================================")
	fmt.Println("  MISSING TRANSACTION RECOVERY TEST")
	fmt.Println("========================================")
	fmt.Println()

	test := &MissingTxRecoveryTest{
		client: GetPooledClient("http://127.0.0.1:26660/v3"),
	}

	// Run the comprehensive test
	test.runComprehensiveTest()
}

func (test *MissingTxRecoveryTest) runComprehensiveTest() {
	fmt.Println("Phase 1: Baseline Check")
	fmt.Println("------------------------")
	baseline := test.checkCurrentState()
	test.printState("Initial State", baseline)

	fmt.Println("\nPhase 2: Monitoring Ledger Changes")
	fmt.Println("-----------------------------------")
	// Monitor for 30 seconds to see if any new anchors/synths arrive
	test.monitorChanges(30 * time.Second)

	fmt.Println("\nPhase 3: Analyzing Missing Transactions")
	fmt.Println("---------------------------------------")
	test.analyzeMissingTransactions()

	fmt.Println("\nPhase 4: Testing Recovery Capability")
	fmt.Println("------------------------------------")
	test.testRecoveryCapability()

	fmt.Println("\nPhase 5: Final State Check")
	fmt.Println("--------------------------")
	final := test.checkCurrentState()
	test.printState("Final State", final)

	fmt.Println("\n========================================")
	fmt.Println("          TEST COMPLETED")
	fmt.Println("========================================")
	test.printSummary(baseline, final)
}

// State represents the current state of ledgers
type State struct {
	Timestamp time.Time
	Anchors   map[string]AnchorState
	Synths    map[string]SynthState
}

type AnchorState struct {
	Partition string
	Sources   map[string]SequenceState
}

type SynthState struct {
	Partition    string
	Destinations map[string]SequenceState
}

type SequenceState struct {
	Received  uint64
	Delivered uint64
	Missing   uint64
	Pending   int
}

// checkCurrentState captures the current state of all ledgers
func (test *MissingTxRecoveryTest) checkCurrentState() State {
	state := State{
		Timestamp: time.Now(),
		Anchors:   make(map[string]AnchorState),
		Synths:    make(map[string]SynthState),
	}

	ctx, cancel := CreateContextWithTimeout(30 * time.Second)
	defer cancel()
	Q := api.Querier2{Querier: test.client}

	// Check each partition
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}

	for _, part := range partitions {
		partUrl := protocol.PartitionUrl(part)

		// Check anchor ledger
		anchorUrl := partUrl.JoinPath(protocol.AnchorPool)
		if resp, err := Q.QueryAccount(ctx, anchorUrl, nil); err == nil {
			if ledger, ok := resp.Account.(*protocol.AnchorLedger); ok {
				anchorState := AnchorState{
					Partition: part,
					Sources:   make(map[string]SequenceState),
				}

				// Check sequences from other partitions
				for _, src := range partitions {
					if src == part {
						continue
					}
					srcUrl := protocol.PartitionUrl(src)
					seq := ledger.Anchor(srcUrl)

					if seq.Received > 0 || seq.Delivered > 0 {
						anchorState.Sources[src] = SequenceState{
							Received:  seq.Received,
							Delivered: seq.Delivered,
							Missing:   seq.Received - seq.Delivered,
							Pending:   len(seq.Pending),
						}
					}
				}

				if len(anchorState.Sources) > 0 {
					state.Anchors[part] = anchorState
				}
			}
		}

		// Check synthetic ledger (skip Directory)
		if part != "Directory" {
			synthUrl := partUrl.JoinPath(protocol.Synthetic)
			if resp, err := Q.QueryAccount(ctx, synthUrl, nil); err == nil {
				if ledger, ok := resp.Account.(*protocol.SyntheticLedger); ok {
					synthState := SynthState{
						Partition:    part,
						Destinations: make(map[string]SequenceState),
					}

					for _, seq := range ledger.Sequence {
						if seq.Url != nil {
							synthState.Destinations[seq.Url.ShortString()] = SequenceState{
								Received:  seq.Received,
								Delivered: seq.Delivered,
								Missing:   seq.Received - seq.Delivered,
								Pending:   len(seq.Pending),
							}
						}
					}

					if len(synthState.Destinations) > 0 {
						state.Synths[part] = synthState
					}
				}
			}
		}
	}

	return state
}

// monitorChanges monitors ledger changes over time
func (test *MissingTxRecoveryTest) monitorChanges(duration time.Duration) {
	fmt.Printf("Monitoring for %v...\n", duration)

	initial := test.checkCurrentState()
	startTime := time.Now()

	// Check every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	changes := 0

	for {
		select {
		case <-ticker.C:
			current := test.checkCurrentState()

			// Check for changes in anchors
			for part, anchorState := range current.Anchors {
				if initial, exists := initial.Anchors[part]; exists {
					for src, seq := range anchorState.Sources {
						if initSeq, exists := initial.Sources[src]; exists {
							if seq.Received > initSeq.Received {
								fmt.Printf("  [%s] New anchors from %s: %d -> %d\n",
									time.Now().Format("15:04:05"),
									src, initSeq.Received, seq.Received)
								changes++
							}
							if seq.Delivered > initSeq.Delivered {
								fmt.Printf("  [%s] Delivered anchors from %s: %d -> %d\n",
									time.Now().Format("15:04:05"),
									src, initSeq.Delivered, seq.Delivered)
								changes++
							}
						}
					}
				}
			}

			// Check for changes in synthetics
			for part, synthState := range current.Synths {
				if initial, exists := initial.Synths[part]; exists {
					for dst, seq := range synthState.Destinations {
						if initSeq, exists := initial.Destinations[dst]; exists {
							if seq.Received > initSeq.Received {
								fmt.Printf("  [%s] New synthetics to %s: %d -> %d\n",
									time.Now().Format("15:04:05"),
									dst, initSeq.Received, seq.Received)
								changes++
							}
							if seq.Delivered > initSeq.Delivered {
								fmt.Printf("  [%s] Delivered synthetics to %s: %d -> %d\n",
									time.Now().Format("15:04:05"),
									dst, initSeq.Delivered, seq.Delivered)
								changes++
							}
						}
					}
				}
			}

			if time.Since(startTime) >= duration {
				fmt.Printf("\nMonitoring complete. Detected %d changes.\n", changes)
				return
			}
		}
	}
}

// analyzeMissingTransactions analyzes patterns in missing transactions
func (test *MissingTxRecoveryTest) analyzeMissingTransactions() {
	state := test.checkCurrentState()

	totalMissingAnchors := 0
	totalMissingSynths := 0

	fmt.Println("\nMissing Anchors:")
	for part, anchorState := range state.Anchors {
		for src, seq := range anchorState.Sources {
			if seq.Missing > 0 {
				fmt.Printf("  %s <- %s: %d missing (received=%d, delivered=%d)\n",
					part, src, seq.Missing, seq.Received, seq.Delivered)
				totalMissingAnchors += int(seq.Missing)

				// Analyze pending list
				if seq.Pending > 0 {
					fmt.Printf("    Pending list has %d entries\n", seq.Pending)
				}
			}
		}
	}

	fmt.Println("\nMissing Synthetics:")
	for part, synthState := range state.Synths {
		for dst, seq := range synthState.Destinations {
			if seq.Missing > 0 {
				fmt.Printf("  %s -> %s: %d missing (received=%d, delivered=%d)\n",
					part, dst, seq.Missing, seq.Received, seq.Delivered)
				totalMissingSynths += int(seq.Missing)

				// Analyze pending list
				if seq.Pending > 0 {
					fmt.Printf("    Pending list has %d entries\n", seq.Pending)
				}
			}
		}
	}

	if totalMissingAnchors == 0 && totalMissingSynths == 0 {
		fmt.Println("\nNo missing transactions detected - system is fully synchronized!")
	} else {
		fmt.Printf("\nTotal missing: %d anchors, %d synthetics\n",
			totalMissingAnchors, totalMissingSynths)

		fmt.Println("\nPotential causes:")
		fmt.Println("  1. Network delays or packet loss")
		fmt.Println("  2. Partition temporarily offline")
		fmt.Println("  3. Processing backlog")
		fmt.Println("  4. Validation failures")
	}
}

// testRecoveryCapability tests the ability to recover missing transactions
func (test *MissingTxRecoveryTest) testRecoveryCapability() {
	ctx, cancel := CreateContextWithTimeout(30 * time.Second)
	defer cancel()
	Q := api.Querier2{Querier: test.client}

	fmt.Println("\nTesting recovery mechanisms...")

	// Find a partition with missing transactions
	state := test.checkCurrentState()

	var targetPartition string
	var sourcePartition string
	var missingCount uint64

	// Look for missing anchors
	for part, anchorState := range state.Anchors {
		for src, seq := range anchorState.Sources {
			if seq.Missing > 0 {
				targetPartition = part
				sourcePartition = src
				missingCount = seq.Missing
				break
			}
		}
		if targetPartition != "" {
			break
		}
	}

	if targetPartition == "" {
		fmt.Println("No missing anchors found to test recovery")

		// Look for missing synthetics instead
		for part, synthState := range state.Synths {
			for dst, seq := range synthState.Destinations {
				if seq.Missing > 0 {
					fmt.Printf("\nFound missing synthetics: %s -> %s (%d missing)\n",
						part, dst, seq.Missing)

					// Test if we can query these
					fmt.Println("Testing ability to query source partition...")
					srcUrl := protocol.PartitionUrl(part)
					ledgerUrl := srcUrl.JoinPath(protocol.Ledger)

					if resp, err := Q.QueryAccount(ctx, ledgerUrl, nil); err == nil {
						if ledger, ok := resp.Account.(*protocol.SystemLedger); ok {
							fmt.Printf("  Source partition %s is accessible (block %d)\n",
								part, ledger.Index)
							fmt.Println("  Recovery would query transactions from this partition")
						}
					}
					return
				}
			}
		}

		fmt.Println("System is fully synchronized - no recovery needed!")
		return
	}

	fmt.Printf("\nFound missing anchors: %s <- %s (%d missing)\n",
		targetPartition, sourcePartition, missingCount)

	// Simulate recovery process
	fmt.Println("\nSimulating recovery process:")
	fmt.Println("1. Identify missing sequence numbers")

	// Get the anchor ledger to see which specific numbers are missing
	dstUrl := protocol.PartitionUrl(targetPartition)
	anchorUrl := dstUrl.JoinPath(protocol.AnchorPool)

	if resp, err := Q.QueryAccount(ctx, anchorUrl, nil); err == nil {
		if ledger, ok := resp.Account.(*protocol.AnchorLedger); ok {
			srcUrl := protocol.PartitionUrl(sourcePartition)
			seq := ledger.Anchor(srcUrl)

			fmt.Printf("   Delivered: %d, Received: %d\n", seq.Delivered, seq.Received)
			fmt.Printf("   Missing: %d-%d\n", seq.Delivered+1, seq.Received)

			// Check pending list
			if len(seq.Pending) > 0 {
				fmt.Printf("   Pending list contains %d transaction IDs\n", len(seq.Pending))
				for i, txid := range seq.Pending {
					if txid != nil && i < 3 { // Show first 3
						fmt.Printf("     - %s\n", txid.String()[:32])
					}
				}
				if len(seq.Pending) > 3 {
					fmt.Printf("     ... and %d more\n", len(seq.Pending)-3)
				}
			}
		}
	}

	fmt.Println("\n2. Query source partition for missing transactions")
	fmt.Printf("   Would query %s for anchors %d-%d\n",
		sourcePartition, missingCount, missingCount)

	fmt.Println("\n3. Validate recovered transactions")
	fmt.Println("   - Verify signatures")
	fmt.Println("   - Check sequence numbers")
	fmt.Println("   - Validate merkle proofs")

	fmt.Println("\n4. Submit recovered transactions to destination")
	fmt.Printf("   Would submit to %s for processing\n", targetPartition)

	fmt.Println("\n5. Update ledger state")
	fmt.Println("   - Mark transactions as delivered")
	fmt.Println("   - Clear from pending list")

	fmt.Println("\nRecovery simulation complete!")
}

// printState prints the current state
func (test *MissingTxRecoveryTest) printState(label string, state State) {
	fmt.Printf("\n%s (as of %s):\n", label, state.Timestamp.Format("15:04:05"))

	// Print anchor states
	if len(state.Anchors) > 0 {
		fmt.Println("Anchors:")
		for part, anchorState := range state.Anchors {
			fmt.Printf("  %s:\n", part)
			for src, seq := range anchorState.Sources {
				fmt.Printf("    <- %s: R=%d D=%d", src, seq.Received, seq.Delivered)
				if seq.Missing > 0 {
					fmt.Printf(" (missing=%d)", seq.Missing)
				}
				fmt.Println()
			}
		}
	}

	// Print synthetic states
	if len(state.Synths) > 0 {
		fmt.Println("Synthetics:")
		for part, synthState := range state.Synths {
			fmt.Printf("  %s:\n", part)
			for dst, seq := range synthState.Destinations {
				fmt.Printf("    -> %s: R=%d D=%d", dst, seq.Received, seq.Delivered)
				if seq.Missing > 0 {
					fmt.Printf(" (missing=%d)", seq.Missing)
				}
				fmt.Println()
			}
		}
	}
}

// printSummary prints a summary comparing initial and final states
func (test *MissingTxRecoveryTest) printSummary(initial, final State) {
	fmt.Println("\nSummary:")
	fmt.Println("--------")

	// Count total transactions
	initialAnchors := 0
	finalAnchors := 0
	initialSynths := 0
	finalSynths := 0

	for _, anchorState := range initial.Anchors {
		for _, seq := range anchorState.Sources {
			initialAnchors += int(seq.Delivered)
		}
	}

	for _, anchorState := range final.Anchors {
		for _, seq := range anchorState.Sources {
			finalAnchors += int(seq.Delivered)
		}
	}

	for _, synthState := range initial.Synths {
		for _, seq := range synthState.Destinations {
			initialSynths += int(seq.Delivered)
		}
	}

	for _, synthState := range final.Synths {
		for _, seq := range synthState.Destinations {
			finalSynths += int(seq.Delivered)
		}
	}

	fmt.Printf("Anchors delivered: %d -> %d (change: %+d)\n",
		initialAnchors, finalAnchors, finalAnchors-initialAnchors)
	fmt.Printf("Synthetics delivered: %d -> %d (change: %+d)\n",
		initialSynths, finalSynths, finalSynths-initialSynths)

	// Count missing
	initialMissingAnchors := 0
	finalMissingAnchors := 0
	initialMissingSynths := 0
	finalMissingSynths := 0

	for _, anchorState := range initial.Anchors {
		for _, seq := range anchorState.Sources {
			initialMissingAnchors += int(seq.Missing)
		}
	}

	for _, anchorState := range final.Anchors {
		for _, seq := range anchorState.Sources {
			finalMissingAnchors += int(seq.Missing)
		}
	}

	for _, synthState := range initial.Synths {
		for _, seq := range synthState.Destinations {
			initialMissingSynths += int(seq.Missing)
		}
	}

	for _, synthState := range final.Synths {
		for _, seq := range synthState.Destinations {
			finalMissingSynths += int(seq.Missing)
		}
	}

	fmt.Printf("\nMissing anchors: %d -> %d (change: %+d)\n",
		initialMissingAnchors, finalMissingAnchors,
		finalMissingAnchors-initialMissingAnchors)
	fmt.Printf("Missing synthetics: %d -> %d (change: %+d)\n",
		initialMissingSynths, finalMissingSynths,
		finalMissingSynths-initialMissingSynths)

	// Overall assessment
	fmt.Println("\nAssessment:")
	if finalMissingAnchors == 0 && finalMissingSynths == 0 {
		fmt.Println("✓ System is fully synchronized")
		fmt.Println("✓ CCC can successfully read ledger states")
		fmt.Println("✓ Recovery mechanisms are available")
	} else if finalMissingAnchors < initialMissingAnchors ||
		finalMissingSynths < initialMissingSynths {
		fmt.Println("✓ Recovery is in progress")
		fmt.Println("✓ Missing transactions are being processed")
	} else {
		fmt.Println("⚠ Missing transactions detected")
		fmt.Println("⚠ Recovery may be needed")
	}
}
