// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var extract2Cmd = &cobra.Command{
	Use:   "extract2 <database> <snapshot> <partition>",
	Short: "Extracts the non-system accounts from a snapshot for use as a Genesis snapshot of a new network",
	Args:  cobra.ExactArgs(3),
	Run:   extract2Snapshot,
}

func init() { cmd.AddCommand(extract2Cmd) }

func extract2Snapshot(_ *cobra.Command, args []string) {
	db, err := database.OpenBadger(args[0], nil)
	check(err)

	f, err := os.Create(args[1])
	check(err)
	defer f.Close()

	fmt.Println("Collecting...")
	var metrics database.CollectMetrics
	check(db.Collect(f, protocol.PartitionUrl(args[2]), &database.CollectOptions{
		Metrics: &metrics,
		Predicate: func(r record.Record) (bool, error) {
			switch r := r.(type) {
			case *bpt.BPT:
				// Skip the BPT
				return false, nil

			case *database.Account:
				h := r.Key().Hash()

				if r.Url().ShortString() == "c1d729874cf20f80b74af5cb4f9f843320302057918dca94/ACME" {
					print("")
				}

				// Skip system accounts
				_, ok := protocol.ParsePartitionUrl(r.Url())
				if ok {
					fmt.Printf("\033[A\rSkipping   [%x] (%d) %v\n", h[:4], metrics.Messages.Count, r.Url())
					return false, nil
				}

				// Skip ACME
				if protocol.AcmeUrl().Equal(r.Url()) {
					fmt.Printf("\033[A\rSkipping   [%x] (%d) %v\n", h[:4], metrics.Messages.Count, r.Url())
					return false, nil
				}

				// Skip light data accounts
				acct, err := r.Main().Get()
				if err != nil {
					return false, nil
					// return false, errors.UnknownError.WithFormat("load message: %w", err)
				}
				_, ok = acct.(*protocol.LiteDataAccount)
				if ok {
					return false, nil
				}

				fmt.Printf("\033[A\rCollecting [%x] (%d) %v\n", h[:4], metrics.Messages.Count, r.Url())

			case *database.MerkleManager:
				// Skip all chains except the main chain
				if !(r.Type() == merkle.ChainTypeTransaction &&
					strings.EqualFold(r.Name(), "main")) {
					return false, nil
				}

			case *database.Message:
				// Skip non-transactions
				msg, err := r.Main().Get()
				if err != nil {
					return false, nil
					// return false, errors.UnknownError.WithFormat("load message: %w", err)
				}
				_, ok := msg.(*messaging.TransactionMessage)
				if !ok {
					return false, nil
				}

				// Skip failed and pending transactions
				st, err := r.TxnStatus().Get()
				if err != nil {
					return false, nil
					// return false, errors.UnknownError.WithFormat("load transaction status: %w", err)
				}
				if st.Code != errors.Delivered {
					return false, nil
				}

				fmt.Printf("\033[A\rCollecting (%d/%d) %v\n", metrics.Messages.Collecting, metrics.Messages.Count, msg.ID())
			}

			return true, nil
		},
	}))
}
