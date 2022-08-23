package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Analyze snapshots",
}

var snapshotListCmd = &cobra.Command{
	Use:   "list <directory>",
	Short: "List snapshots",
	Args:  cobra.ExactArgs(1),
	Run:   listSnapshots,
}

var snapshotDumpCmd = &cobra.Command{
	Use:   "dump <snapshot>",
	Short: "Dump the contents of a snapshot",
	Args:  cobra.ExactArgs(1),
	Run:   dumpSnapshot,
}

var snapshotDumpFlag = struct {
	Short bool
}{}

func init() {
	cmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(
		snapshotListCmd,
		snapshotDumpCmd,
	)
	snapshotDumpCmd.Flags().BoolVarP(&snapshotDumpFlag.Short, "short", "s", false, "Short output")
}

func listSnapshots(_ *cobra.Command, args []string) {
	snapDir := args[0]
	entries, err := os.ReadDir(snapDir)
	checkf(err, "read directory")

	wr := tabwriter.NewWriter(os.Stdout, 3, 4, 2, ' ', 0)
	defer wr.Flush()

	fmt.Fprint(wr, "HEIGHT\tHASH\tFILE\n")
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !core.SnapshotMajorRegexp.MatchString(entry.Name()) {
			continue
		}

		filename := filepath.Join(snapDir, entry.Name())
		f, err := os.Open(filename)
		checkf(err, "open snapshot %s", entry.Name())
		defer f.Close()

		header, _, err := database.OpenSnapshot(f)
		checkf(err, "oepn snapshot %s", entry.Name())

		fmt.Fprintf(wr, "%d\t%x\t%s\n", header.Height, header.RootHash, entry.Name())
	}
}

var enUsTitle = cases.Title(language.AmericanEnglish)

func dumpSnapshot(_ *cobra.Command, args []string) {
	filename := args[0]
	f, err := os.Open(filename)
	checkf(err, "open snapshot %s", filename)
	defer f.Close()

	header, rd, err := database.OpenSnapshot(f)
	checkf(err, "open snapshot %s", filename)
	fmt.Printf("Height:\t%d\n", header.Height)
	fmt.Printf("Hash:\t%x\n", header.RootHash)

	for {
		s, err := rd.Next()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, io.EOF):
			return
		default:
			checkf(err, "read section header")
		}

		typstr := s.Type().String()
		fmt.Printf("%s%s section at %d (size %d)\n", enUsTitle.String(typstr[:1]), typstr[1:], s.Offset(), s.Size())
		if snapshotDumpFlag.Short {
			continue
		}

		sr, err := s.Open()
		checkf(err, "open section")

		switch s.Type() {
		case snapshot.SectionTypeAccounts:
			store := memory.New(nil)
			batch := store.Begin(true)
			defer batch.Discard()
			bpt := pmt.NewBPTManager(batch)
			err = bpt.Bpt.LoadSnapshot(sr, func(_ storage.Key, _ [32]byte, reader ioutil2.SectionReader) error {
				state := new(accountState)
				err := state.UnmarshalBinaryFrom(reader)
				if err != nil {
					return err
				}

				fmt.Printf("  Account %v (%v)\n", state.Main.GetUrl(), state.Main.Type())

				for _, chain := range state.Chains {
					fmt.Printf("    Chain %s (%v) height %d", chain.Name, chain.Type, chain.Count)
					if chain.Count > 0 && len(chain.Entries) == 0 {
						fmt.Printf(" (pruned)")
					}
					fmt.Println()
				}
				return nil
			})
			checkf(err, "load snapshot")

		case snapshot.SectionTypeTransactions:
			s := new(transactionSection)
			err = s.UnmarshalBinaryFrom(sr)
			checkf(err, "unmarshal transaction section")

			for _, txn := range s.Transactions {
				fmt.Printf("    Transaction %x (%v)\n", txn.Transaction.GetHash()[:4], txn.Transaction.Body.Type())
			}

		case snapshot.SectionTypeSignatures:
			s := new(signatureSection)
			err = s.UnmarshalBinaryFrom(sr)
			checkf(err, "unmarshal transaction section")

			for _, sig := range s.Signatures {
				fmt.Printf("    Signature %x (%v)\n", sig.Hash()[:4], sig.Type())
			}

		default:
			fmt.Printf("  (do not know how to handle %v)\n", s.Type())
		}
	}
}
