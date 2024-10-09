// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/csv"
	"io"
	"math/big"
	"os"
	"slices"

	"github.com/spf13/cobra"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	sv1 "gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	cmd.AddCommand(cmdSnap)
	cmdSnap.AddCommand(cmdSnapRich)
}

var cmdSnap = &cobra.Command{
	Use:     "snapshot",
	Aliases: []string{"snap"},
	Short:   "Snapshot utilities",
}

var cmdSnapRich = &cobra.Command{
	Use:     "rich-list [snapshot]",
	Aliases: []string{"rich"},
	Short:   "Extract the most valuable accounts from a snapshot",
	Args:    cobra.ExactArgs(1),
	Run:     runSnapRich,
}

var flagSnapRich = struct {
	Top int
	Min int
}{}

func init() {
	cmdSnapRich.Flags().IntVar(&flagSnapRich.Top, "top", 0, "List the top N accounts (0 for all)")
	cmdSnapRich.Flags().IntVar(&flagSnapRich.Min, "min", 0, "Minimum balance threshold for including an account")
}

func runSnapRich(_ *cobra.Command, args []string) {
	rd := openSnapshotFile(args[0])
	if c, ok := rd.(io.Closer); ok {
		defer c.Close()
	}

	var accounts []protocol.AccountWithTokens
	min := big.NewInt(protocol.AcmePrecision)
	min.Mul(min, big.NewInt(int64(flagSnapRich.Min)))
	if min.Sign() <= 0 {
		min.SetInt64(1)
	}

	add := func(acct protocol.AccountWithTokens) {
		if !protocol.AcmeUrl().Equal(acct.GetTokenUrl()) {
			return
		}

		if acct.TokenBalance().Cmp(min) < 0 {
			return
		}

		s := protocol.FormatBigAmount(acct.TokenBalance(), protocol.AcmePrecisionPower)
		_ = s
		accounts = append(accounts, acct)
	}

	db := coredb.OpenInMemory(nil)
	scanSnapshot(rd, snapshotScanArgs{
		Record: func(value any) {
			switch value := value.(type) {
			case *sv1.Account:
				acct, ok := value.Main.(protocol.AccountWithTokens)
				if !ok {
					break
				}
				add(acct)

			case *sv2.RecordEntry:
				if value.Key.Len() == 0 || value.Key.Get(0) != "Account" {
					break
				}

				// Decode the record
				batch := db.Begin(false)
				defer batch.Discard()
				v, err := values.Resolve[database.Value](batch, value.Key)
				if err != nil {
					break
				}
				err = v.LoadBytes(value.Value, true)
				if err != nil {
					break
				}
				u, ok := v.(values.Value[protocol.Account])
				if !ok {
					break
				}

				var acct protocol.AccountWithTokens
				if u.GetAs(&acct) != nil {
					break
				}
				add(acct)
			}
		},
	})

	slices.SortFunc(accounts, func(a, b protocol.AccountWithTokens) int {
		return -a.TokenBalance().Cmp(b.TokenBalance())
	})

	if flagSnapRich.Top > 0 && len(accounts) > flagSnapRich.Top {
		accounts = accounts[:flagSnapRich.Top]
	}

	wr := csv.NewWriter(os.Stdout)
	for _, acct := range accounts {
		name := acct.GetUrl().String()
		balance := protocol.FormatBigAmount(acct.TokenBalance(), protocol.AcmePrecisionPower)
		check(wr.Write([]string{name, balance}))
	}
	wr.Flush()
}
