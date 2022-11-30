// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
)

var stripTxCmd = &cobra.Command{
	Use:   "strip-tx <input> <output>",
	Short: "Strip out transactions and transaction history from the snapshot",
	Args:  cobra.ExactArgs(2),
	Run:   stripSnapshot,
}

func init() { cmd.AddCommand(stripTxCmd) }

func stripSnapshot(_ *cobra.Command, args []string) {
	in, err := os.Open(args[0])
	checkf(err, "input")
	defer in.Close()

	out, err := os.Create(args[1])
	checkf(err, "output")
	defer out.Close()

	r := snapshot.NewReader(in)
	w := snapshot.NewWriter(out)
	for {
		s, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		switch s.Type() {
		case snapshot.SectionTypeTransactions,
			snapshot.SectionTypeGzTransactions,
			snapshot.SectionTypeSignatures:
			fmt.Printf("Discarding %s section at %d\n", s.Type(), s.Offset())
			continue

		case snapshot.SectionTypeAccounts:
			fmt.Printf("Stripping %s section at %d\n", s.Type(), s.Offset())

		default:
			fmt.Printf("Copying %s section at %d\n", s.Type(), s.Offset())

			sr, err := s.Open()
			checkf(err, "read section")

			sw, err := w.Open(s.Type())
			checkf(err, "write section")

			_, err = io.Copy(sw, sr)
			checkf(err, "copy section")

			check(sw.Close())
			continue
		}

		db := database.OpenInMemory(nil)
		check(db.Update(func(batch *database.Batch) error {
			sr, err := s.Open()
			checkf(err, "read section")

			var i int
			start := time.Now()
			check(pmt.ReadSnapshot(sr, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
				account := new(snapshot.Account)
				check(account.UnmarshalBinaryFrom(reader))

				// Fix up the URL
				if account.Url == nil {
					if account.Main == nil {
						fatalf("cannot determine URL of account")
					}
					account.Url = account.Main.GetUrl()
				}

				// Restore the account
				check(account.Restore(batch))

				for _, c := range account.Chains {
					// Remove mark points
					c.MarkPoints = nil

					// Restore the chain
					c2, err := account.RestoreChainHead(batch, c)
					check(err)
					check(c2.Inner().RestoreMarkPointRange(c, 0, len(c.MarkPoints)))
				}

				i++
				if i%1000 == 0 {
					since := time.Since(start)
					fmt.Printf("Processed %d accounts in %v (%.2f/s)\n", i, since, float64(i)/since.Seconds())
				}
				return nil
			}))

			since := time.Since(start)
			fmt.Printf("Processed %d accounts in %v (%.2f/s)\n", i, since, float64(i)/since.Seconds())
			return nil
		}))

		check(db.View(func(batch *database.Batch) error {
			return w.CollectAccounts(batch, snapshot.CollectOptions{
				PreserveAccountHistory: func(account *database.Account) (bool, error) { return false, nil },
			})
		}))
	}
}
