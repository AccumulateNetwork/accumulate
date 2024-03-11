// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var extractCmd = &cobra.Command{
	Use:   "extract <database> <snapshot>",
	Short: "Extracts the non-system accounts from a snapshot for use as a Genesis snapshot of a new network",
	Args:  cobra.ExactArgs(2),
	Run:   extractSnapshot,
}

var extractFlag = struct {
	IncludeLDA bool
}{}

func init() {
	cmd.AddCommand(extractCmd)
	extractCmd.Flags().BoolVar(&extractFlag.IncludeLDA, "include-ldas", false, "Include lite data accounts")
}

func extractSnapshot(_ *cobra.Command, args []string) {
	// pprof
	s := new(http.Server)
	s.Addr = ":8080"
	s.ReadHeaderTimeout = time.Minute
	go func() { check(s.ListenAndServe()) }() //nolint:gosec

	db, err := database.OpenBadger(args[0], nil)
	check(err)

	f, err := os.Create(args[1])
	check(err)
	defer f.Close()

	fmt.Println("Collecting...")
	var metrics database.CollectMetrics
	check(db.Collect(f, nil, &database.CollectOptions{
		BuildIndex: true,
		Metrics:    &metrics,
		Predicate: func(r record.Record) (bool, error) {
			if r.Key().Len() == 3 && r.Key().Get(0) == "Account" && r.Key().Get(2) == "Pending" {
				// Don't retain pending transactions
				return false, nil
			}

			switch r := r.(type) {
			case *bpt.BPT:
				// Skip the BPT
				return false, nil

			case *database.Account:
				h := r.Key().Hash()

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

				if !extractFlag.IncludeLDA {
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
				}

				fmt.Printf("\033[A\rCollecting [%x] (%d) %v\n", h[:4], metrics.Messages.Count, r.Url())

			case *database.MerkleManager:
				// Skip all chains except the main chain
				if !(r.Type() == merkle.ChainTypeTransaction &&
					strings.EqualFold(r.Name(), "main")) {
					return false, nil
				}

			case *database.Message:
				// Attempting to read the message and/or status will cause the
				// batch to cache values. So instead we're rely on the chain
				// processing to skip anything we don't want.
				if metrics.Messages.Collecting%1000 == 0 {
					fmt.Printf("\033[A\rCollecting (%d/%d) %x\n", metrics.Messages.Collecting, metrics.Messages.Count, r.Key().Get(1).([32]byte))
				}
			}

			return true, nil
		},
	}))
}
