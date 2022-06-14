package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/v1"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
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

func init() {
	cmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(
		snapshotListCmd,
		snapshotDumpCmd,
	)
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

		height, _, hash, _, err := database.ReadSnapshot(f)
		checkf(err, "read snapshot %s", entry.Name())

		fmt.Fprintf(wr, "%d\t%x\t%s\n", height, hash, entry.Name())
	}
}

func dumpSnapshot(_ *cobra.Command, args []string) {
	filename := args[0]
	f, err := os.Open(filename)
	checkf(err, "open snapshot %s", filename)
	defer f.Close()

	height, _, hash, rd, err := database.ReadSnapshot(f)
	checkf(err, "read snapshot %s", filename)
	fmt.Printf("Height:\t%d\n", height)
	fmt.Printf("Hash:\t%x\n", hash)

	store := memory.New(nil)
	batch := store.Begin(true)
	defer batch.Discard()
	bpt := pmt.NewBPTManager(batch)
	err = bpt.Bpt.LoadSnapshot(rd, func(_ storage.Key, _ [32]byte, reader ioutil2.SectionReader) error {
		state := new(accountState)
		err := state.UnmarshalBinaryFrom(reader)
		if err != nil {
			return err
		}

		fmt.Printf("Account %v (%v)\n", state.Main.GetUrl(), state.Main.Type())

		for _, chain := range state.Chains {
			fmt.Printf("    Chain %s (%v) height %d", chain.Name, chain.Type, chain.Count)
			if chain.Count > 0 && len(chain.Entries) == 0 {
				fmt.Printf(" (pruned)")
			}
			fmt.Println()
		}

		for _, txn := range state.Pending {
			fmt.Printf("    Pending transaction %x (%v)\n", txn.Transaction.GetHash()[:4], txn.Transaction.Body.Type())
		}

		for _, txn := range state.Transactions {
			fmt.Printf("    Other transaction %x (%v)\n", txn.Transaction.GetHash()[:4], txn.Transaction.Body.Type())
		}

		return nil
	})
	checkf(err, "load snapshot")
}
