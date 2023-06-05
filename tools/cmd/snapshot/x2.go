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

	check(db.Collect(f, protocol.PartitionUrl(args[2]), &database.CollectOptions{
		Predicate: func(r record.Record) (bool, error) {
			switch r := r.(type) {
			case *bpt.BPT:
				// Skip the BPT
				return false, nil

			case *database.Account:
				// Skip system accounts
				_, ok := protocol.ParsePartitionUrl(r.Url())
				if ok {
					return false, nil
				}

				fmt.Println("Collecting", r.Url())

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
					return false, errors.UnknownError.WithFormat("load message: %w", err)
				}
				_, ok := msg.(*messaging.TransactionMessage)
				if !ok {
					return false, nil
				}

				// Skip failed and pending transactions
				st, err := r.TxnStatus().Get()
				if err != nil {
					return false, errors.UnknownError.WithFormat("load transaction status: %w", err)
				}
				if st.Code != errors.Delivered {
					return false, nil
				}

				fmt.Println("Collecting", msg.ID())
			}

			return true, nil
		},
	}))
}
