// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/spf13/cobra"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/remote"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdSnapCollect = &cobra.Command{
	Use:   "collect [database] [snapshot]",
	Short: "Collect a snapshot",
	Args:  cobra.ExactArgs(2),
	Run:   collectSnapshot,
}

var flagSnapCollect = struct {
	SkipBPT    bool
	SkipSystem bool
	SkipSigs   bool
	SkipTokens bool
	Indexed    bool
	Partition  UrlFlag
}{}

func init() {
	cmdSnap.AddCommand(cmdSnapCollect)
	cmdSnapCollect.Flags().BoolVar(&flagSnapCollect.SkipBPT, "skip-bpt", false, "Skip the BPT")
	cmdSnapCollect.Flags().BoolVar(&flagSnapCollect.SkipSystem, "skip-system", false, "Skip system accounts")
	cmdSnapCollect.Flags().BoolVar(&flagSnapCollect.SkipSigs, "skip-signatures", false, "Skip signatures")
	cmdSnapCollect.Flags().BoolVar(&flagSnapCollect.SkipTokens, "skip-token-txns", false, "Skip token transactions")
	cmdSnapCollect.Flags().BoolVar(&flagSnapCollect.Indexed, "indexed", false, "Make an indexed snapshot")
	cmdSnapCollect.Flags().Var(&flagSnapCollect.Partition, "partition", "Specify the partition instead of determining it from the database")
}

func collectSnapshot(_ *cobra.Command, args []string) {
	//AI: Print a summary of user parameters and the state of each flag for this call.
	fmt.Println("User Parameters:")
	fmt.Printf("  Database Path: %s\n", args[0])
	fmt.Printf("  Snapshot Path: %s\n", args[1])
	fmt.Println("Flags:")
	fmt.Printf("  Skip BPT: %v\n", flagSnapCollect.SkipBPT)
	fmt.Printf("  Skip System: %v\n", flagSnapCollect.SkipSystem)
	fmt.Printf("  Skip Signatures: %v\n", flagSnapCollect.SkipSigs)
	fmt.Printf("  Skip Token Transactions: %v\n", flagSnapCollect.SkipTokens)
	fmt.Printf("  Indexed: %v\n", flagSnapCollect.Indexed)
	if flagSnapCollect.Partition.V != nil {
		fmt.Printf("  Partition: %v\n", flagSnapCollect.Partition.V)
	} else {
		fmt.Println("  Partition: not set")
	}

	//AI: Open the database connection using the provided path (args[0]). If the DB is remote, connect over the network.
	// Open the database
	var store keyvalue.Beginner
	store, remoteAddr := openDbUrl(args[0], false)
	//AI: openDbUrl returns a store and (optionally) a remote address if the DB is remote.
	if remoteAddr != nil {
		//AI: If a remote address is detected, wrap the store with a network connection.
		store = remote.Connect(func() (io.ReadWriteCloser, error) {
			return net.Dial(remoteAddr.Network(), remoteAddr.String())
		})
	}
	db := coredb.New(store, nil)
	//AI: Create a new coredb.Database instance using the store.

	// Scan for the partition account
	var partUrl *url.URL
	//AI: Determine the partition to snapshot. Use the flag if provided, otherwise scan the DB for a partition account.
	if flagSnapCollect.Partition.V != nil {
		//AI: Use the partition flag if provided by the user.
		partUrl = flagSnapCollect.Partition.V.RootIdentity()
	} else {
		fmt.Println("Scanning the database for a partition's system ledger")
		fmt.Println("This is super slow.  Use the --partition flag (\"--partition [bvn-Apollo.acme or bvn-dn.acme])\" to avoid.")
		//AI: If no partition flag, scan the DB to find a partition account (system ledger).
		partUrl = protocol.PartitionUrl(getPartition(db, args[0]))
	}

	// Create the snapshot file
	f, err := os.Create(args[1])
	//AI: Create the output file for the snapshot using the path from args[1].
	check(err)
	defer f.Close()

	// Timer for updating progress
	tick := time.NewTicker(time.Second / 2)
	//AI: Set up a ticker to periodically update progress during snapshot collection.
	defer tick.Stop()

	// Collect a snapshot
	var metrics coredb.CollectMetrics
	//AI: Prepare to collect metrics during the snapshot operation.
	fmt.Println("Collecting...")

	_, err = db.Collect(f, partUrl, &coredb.CollectOptions{
		//AI: Call Collect to write the snapshot, passing options and filters for what to include.
		BuildIndex:      flagSnapCollect.Indexed,
		Metrics:         &metrics,
		SkipMessageRefs: flagSnapCollect.SkipSigs,
		Predicate: func(r database.Record) (bool, error) {
			//AI: Predicate function determines which records to include in the snapshot.
			// Skip the BPT
			if flagSnapCollect.SkipBPT && r.Key().Get(0) == "BPT" {
				//AI: Optionally skip the BPT (Binary Patricia Tree) records if the flag is set.
				return false, nil
			}

			// Skip system accounts
			if flagSnapCollect.SkipSystem && r.Key().Get(0) == "Account" {
				//AI: Optionally skip system accounts if the flag is set.
				_, ok := protocol.ParsePartitionUrl(r.Key().Get(1).(*url.URL))
				if ok {
					return false, nil
				}
			}

			select {
			//AI: Throttle progress updates to once per tick interval.
			case <-tick.C:
			default:
				return true, nil
			}

			//AI: Print progress updates for account, message, or transaction records.
			// Print progress
			switch r.Key().Get(0) {
			case "Account":
				k := r.Key().SliceJ(2)
				h := k.Hash()
				fmt.Printf("\033[A\r\033[KCollecting [%x] (%d) %v\n", h[:4], metrics.Messages.Count, k.Get(1))

			case "Message", "Transaction":
				fmt.Printf("\033[A\r\033[KCollecting (%d/%d) %x\n", metrics.Messages.Collecting, metrics.Messages.Count, r.Key().Get(1).([32]byte))
			}

			//AI: By default, retain all records unless filtered above.
			//AI: Retain all other messages by default.
			return true, nil
		},
		KeepMessage: func(msg messaging.Message) (bool, error) {
			//AI: Decide which messages to keep in the snapshot based on type and flags.
			switch msg := msg.(type) {
			case *messaging.SignatureMessage:
				//AI: Optionally skip signature messages if the skip-signatures flag is set.
				return !flagSnapCollect.SkipSigs, nil
			case *messaging.TransactionMessage:
				//AI: Handle transaction messages; optionally skip token-related transactions if the flag is set.
				if !flagSnapCollect.SkipTokens {
					break
				}
				switch msg.Transaction.Body.Type() {
				case protocol.TransactionTypeSendTokens,
					protocol.TransactionTypeBurnTokens,
					protocol.TransactionTypeAddCredits,
					protocol.TransactionTypeSyntheticDepositTokens,
					protocol.TransactionTypeSyntheticDepositCredits:
					//AI: Exclude these token-related transaction types from the snapshot if the skip-tokens flag is set.
					return false, nil
				}
			}
			//AI: If no filtering conditions match, retain the message by default.
			return true, nil
		},
	})
	check(err)
	//AI: CollectSnapshot function completes; any errors encountered during snapshot collection are handled here.
}

func getPartition(db *coredb.Database, path string) string {
	//AI: Scan the database for the partition account (root identity) and return its partition name.

	// Scan for the partition account
	var thePart string
	//AI: Will store the name of the found partition, if any.
	batch := db.Begin(false)
	//AI: Start a read-only database batch for scanning accounts.
	defer batch.Discard()
	Check(batch.ForEachAccount(func(account *coredb.Account, _ [32]byte) error {
		//AI: Iterate over all accounts in the database.
		if !account.Url().IsRootIdentity() {
			//AI: Skip accounts that are not root identities (i.e., not partition accounts).
			return nil
		}

		part, ok := protocol.ParsePartitionUrl(account.Url())
		//AI: Attempt to parse the account URL as a partition URL.
		if !ok {
			//AI: If parsing fails, skip this account.
			return nil
		}

		fmt.Printf("Found %v in %s\n", account.Url(), path)
		//AI: Log the found partition account for debugging.

		if thePart == "" {
			//AI: If this is the first partition found, record it.
			thePart = part
			return nil
		}

		Fatalf("%s has multiple partition accounts", path)
		//AI: Error out if more than one partition account is found in the database.
		panic("not reached")
	}))
	return thePart
}
