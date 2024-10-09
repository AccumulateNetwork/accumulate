// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	sv1 "gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var cmdSnapDump = &cobra.Command{
	Use:   "dump [snapshot or genesis.json]",
	Short: "Dumps a snapshot",
	Args:  cobra.ExactArgs(1),
	Run:   dumpSnapshot,
}

var flagSnapDump = struct {
	Short  bool
	Verify bool
}{}

func init() {
	cmdSnap.AddCommand(cmdSnapDump)

	cmdSnapDump.Flags().BoolVarP(&flagSnapDump.Short, "short", "s", false, "Short output")
	cmdSnapDump.Flags().BoolVar(&flagSnapDump.Verify, "verify", false, "Verify the hash of accounts")
}

var enUsTitle = cases.Title(language.AmericanEnglish)

func dumpSnapshot(_ *cobra.Command, args []string) {
	rd := openSnapshotFile(args[0])
	if c, ok := rd.(io.Closer); ok {
		defer c.Close()
	}

	db := database.OpenInMemory(nil)
	scanSnapshot(rd, snapshotScanArgs{
		Section: func(typ sv2.SectionType, size, offset int64) bool {
			typstr := typ.String()
			fmt.Printf("%s%s section at %d (size %d)\n", enUsTitle.String(typstr[:1]), typstr[1:], offset, size)
			return typ == sv2.SectionTypeHeader ||
				!flagSnapDump.Short ||
				flagSnapDump.Verify && typ == sv1.SectionTypeAccounts
		},
		Header: func(version, height uint64, timestamp time.Time, rootHash [32]byte) {
			if flagSnapDump.Verify && version != 1 {
				fatalf("--verify not supported for version %d snapshots", version)
			}
			{
				fmt.Printf("  Version    %d\n", version)
			}
			if height > 0 {
				fmt.Printf("  Height     %d\n", height)
			}
			if timestamp != (time.Time{}) {
				fmt.Printf("  Time       %v\n", timestamp)
			}
			if rootHash != [32]byte{} {
				fmt.Printf("  State hash %x\n", rootHash)
			}
		},
		Record: func(value any) {
			switch value := value.(type) {
			case *sv2.RecordEntry:
				h := sha256.Sum256(value.Value)
				fmt.Printf("  %x  %v\n", h[:4], value.Key)

			case *sv2.RecordIndexEntry:
				fmt.Printf("  %x in section %d offset %d\n", value.Key[:8], value.Section, value.Offset)

			case *sv1.Transaction:
				fmt.Printf("    Transaction %x, %v, %v \n", value.Transaction.GetHash()[:4], value.Transaction.Body.Type(), value.Transaction.Header.Principal)

			case *sv1.Signature:
				fmt.Printf("    Signature %x (%v)\n", value.Signature.Hash()[:4], value.Signature.Type())

			case *sv1.Account:
				if !flagSnapDump.Short {
					var typ string
					if value.Main == nil {
						typ = "<nil>"
					} else {
						typ = value.Main.Type().String()
					}
					fmt.Printf("  Account %v (%v)\n", value.Url, typ)

					for _, chain := range value.Chains {
						fmt.Printf("    Chain %s (%v) height %d with %d mark points\n", chain.Name, chain.Type, chain.Head.Count, len(chain.MarkPoints))
					}
				}

				if flagSnapDump.Verify {
					batch := db.Begin(true)
					defer batch.Discard()

					err := value.Restore(batch)
					checkf(err, "restore %v", value.Url)

					for _, c := range value.Chains {
						c2, err := value.RestoreChainHead(batch, c)
						checkf(err, "restore %v %s chain", value.Url, c.Name)
						err = c.RestoreMarkPointRange(c2.Inner(), 0, len(c.MarkPoints))
						checkf(err, "restore %v %s chain", value.Url, c.Name)
					}

					err = batch.Account(value.Url).VerifyHash(value.Hash[:])
					checkf(err, "verify %v", value.Url)
				}

			default:
				fmt.Printf("  (unknown record: %T)\n", value)
			}
		},
	})
}
