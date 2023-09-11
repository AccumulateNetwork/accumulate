// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdSnapCollect = &cobra.Command{
	Use:   "collect [database] [snapshot]",
	Short: "Collect a snapshot",
	Args:  cobra.ExactArgs(2),
	Run:   collectSnapshot,
}

func init() {
	cmdSnap.AddCommand(cmdSnapCollect)
}

func collectSnapshot(_ *cobra.Command, args []string) {
	// Open the database
	db, err := coredb.OpenBadger(args[0], nil)
	check(err)

	// Scan for the partition account
	partUrl := protocol.PartitionUrl(getPartition(db, args[0]))

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
	check(db.Collect(f, partUrl, &coredb.CollectOptions{
		// BuildIndex: true,
		Metrics: &metrics,
		Predicate: func(r database.Record) (bool, error) {
			select {
			case <-tick.C:
			default:
				return true, nil
			}

			// The sole purpose of this function is to print progress
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
	}))
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
