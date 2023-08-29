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
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

var cmdDb = &cobra.Command{
	Use:     "database",
	Aliases: []string{"db"},
	Short:   "Database utilities",
}

var cmdDbSnapshot = &cobra.Command{
	Use:   "snapshot [partition] [database] [snapshot]",
	Short: "Collect a snapshot",
	Args:  cobra.ExactArgs(3),
	Run:   collectSnapshot,
}

func init() {
	cmd.AddCommand(cmdDb)
	cmdDb.AddCommand(cmdDbSnapshot)
}

func collectSnapshot(_ *cobra.Command, args []string) {
	partUrl, err := url.Parse(args[0])
	check(err)

	db, err := coredb.OpenBadger(args[1], nil)
	check(err)

	f, err := os.Create(args[2])
	check(err)
	defer f.Close()

	tick := time.NewTicker(time.Second / 2)
	defer tick.Stop()

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
