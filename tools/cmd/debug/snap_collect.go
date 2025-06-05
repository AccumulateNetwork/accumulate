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
	// Open the database
	var store keyvalue.Beginner
	store, remoteAddr := openDbUrl(args[0], false)
	if remoteAddr != nil {
		store = remote.Connect(func() (io.ReadWriteCloser, error) {
			return net.Dial(remoteAddr.Network(), remoteAddr.String())
		})
	}
	db := coredb.New(store, nil)

	// Scan for the partition account
	var partUrl *url.URL
	if flagSnapCollect.Partition.V != nil {
		partUrl = flagSnapCollect.Partition.V.RootIdentity()
	} else {
		fmt.Println("Scanning the database for a partition's system ledger...")
		partUrl = protocol.PartitionUrl(getPartition(db, args[0]))
	}

	// Create the snapshot file
	f, err := os.Create(args[1])
	check(err)
	defer f.Close()

	// Timer for updating progress
	tick := time.NewTicker(time.Second / 2)
	defer tick.Stop()

	// Collect a snapshot
	var metrics coredb.CollectMetrics
	fmt.Println("Collecting...")
	_, err = db.Collect(f, partUrl, &coredb.CollectOptions{
		BuildIndex:      flagSnapCollect.Indexed,
		Metrics:         &metrics,
		SkipMessageRefs: flagSnapCollect.SkipSigs,
		Predicate: func(r database.Record) (bool, error) {
			// Skip the BPT
			if flagSnapCollect.SkipBPT && r.Key().Get(0) == "BPT" {
				return false, nil
			}

			// Skip system accounts
			if flagSnapCollect.SkipSystem && r.Key().Get(0) == "Account" {
				_, ok := protocol.ParsePartitionUrl(r.Key().Get(1).(*url.URL))
				if ok {
					return false, nil
				}
			}

			select {
			case <-tick.C:
			default:
				return true, nil
			}

			// Print progress
			switch r.Key().Get(0) {
			case "Account":
				k := r.Key().SliceJ(2)
				h := k.Hash()
				fmt.Printf("\033[A\r\033[KCollecting [%x] (%d) %v\n", h[:4], metrics.Messages.Count, k.Get(1))

			case "Message", "Transaction":
				fmt.Printf("\033[A\r\033[KCollecting (%d/%d) %x\n", metrics.Messages.Collecting, metrics.Messages.Count, r.Key().Get(1).([32]byte))
			}

			// Retain everything
			return true, nil
		},
		KeepMessage: func(msg messaging.Message) (bool, error) {
			switch msg := msg.(type) {
			case *messaging.SignatureMessage:
				return !flagSnapCollect.SkipSigs, nil
			case *messaging.TransactionMessage:
				if !flagSnapCollect.SkipTokens {
					break
				}
				switch msg.Transaction.Body.Type() {
				case protocol.TransactionTypeSendTokens,
					protocol.TransactionTypeBurnTokens,
					protocol.TransactionTypeAddCredits,
					protocol.TransactionTypeSyntheticDepositTokens,
					protocol.TransactionTypeSyntheticDepositCredits:
					return false, nil
				}
			}
			return true, nil
		},
	})
	check(err)
}

func getPartition(db *coredb.Database, path string) string {
	// Scan for the partition account
	var thePart string
	batch := db.Begin(false)
	defer batch.Discard()
	Check(batch.ForEachAccount(func(account *coredb.Account, _ [32]byte) error {
		if !account.Url().IsRootIdentity() {
			return nil
		}

		part, ok := protocol.ParsePartitionUrl(account.Url())
		if !ok {
			return nil
		}

		fmt.Printf("Found %v in %s\n", account.Url(), path)

		if thePart == "" {
			thePart = part
			return nil
		}

		Fatalf("%s has multiple partition accounts", path)
		panic("not reached")
	}))
	return thePart
}
