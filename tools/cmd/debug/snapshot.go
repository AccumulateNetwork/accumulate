package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
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

var snapshotConcatCmd = &cobra.Command{
	Use:   "concat <output> <inputs>",
	Short: "Concatenate multiple snapshots into one",
	Args:  cobra.MinimumNArgs(2),
	Run:   concatSnapshots,
}

var snapshotDumpFlag = struct {
	Short bool
}{}

func init() {
	cmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(
		snapshotListCmd,
		snapshotDumpCmd,
		snapshotConcatCmd,
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
		fInfo, err := f.Stat()
		checkf(err, "open snapshot info %s", entry.Name())
		defer f.Close()

		/*		header, _, err := snapshot.Open(f)
				checkf(err, "open snapshot %s", entry.Name())
		*/
		buf := make([]byte, fInfo.Size())
		f.Read(buf)
		tmSnap := new(snapshot.TmStateSnapshot)
		tmSnap.UnmarshalBinary(buf)
		if tmSnap.Height > 0 {
			fmt.Fprintf(wr, "%d\t%x\t%s\n", tmSnap.Height, tmSnap.Blocks[1].SignedHeader.Header.AppHash, entry.Name())
		}
	}
}

var enUsTitle = cases.Title(language.AmericanEnglish)

func dumpSnapshot(_ *cobra.Command, args []string) {
	filename := args[0]
	f, err := os.Open(filename)
	checkf(err, "open snapshot %s", filename)
	defer f.Close()

	check(snapshot.Visit(f, dumpVisitor{}))
}

type dumpVisitor struct{}

func (dumpVisitor) VisitSection(s *snapshot.ReaderSection) error {
	typstr := s.Type().String()
	fmt.Printf("%s%s section at %d (size %d)\n", enUsTitle.String(typstr[:1]), typstr[1:], s.Offset(), s.Size())
	if snapshotDumpFlag.Short {
		return snapshot.ErrSkip
	}
	return nil
}

func (dumpVisitor) VisitAccount(acct *snapshot.Account, _ int) error {
	if acct == nil {
		return nil
	}

	fmt.Printf("  Account %v (%v)\n", acct.Main.GetUrl(), acct.Main.Type())

	for _, chain := range acct.Chains {
		fmt.Printf("    Chain %s (%v) height %d", chain.Name, chain.Type, chain.Count+uint64(len(chain.Entries)))
		if chain.Count > 0 && len(chain.Entries) == 0 {
			fmt.Printf(" (pruned)")
		}
		fmt.Println()
	}
	return nil
}

func (dumpVisitor) VisitTransaction(txn *snapshot.Transaction, _ int) error {
	if txn == nil {
		return nil
	}
	fmt.Printf("    Transaction %x, %v, %v \n", txn.Transaction.GetHash()[:4], txn.Transaction.Body.Type(), txn.Transaction.Header.Principal)
	return nil
}

func (dumpVisitor) VisitSignature(sig *snapshot.Signature, _ int) error {
	if sig == nil {
		return nil
	}
	fmt.Printf("    Signature %x (%v)\n", sig.Signature.Hash()[:4], sig.Signature.Type())
	return nil
}

func concatSnapshots(_ *cobra.Command, args []string) {
	wf, err := os.Create(args[0])
	checkf(err, "output file")
	w := snapshot.NewWriter(wf)

	for i, filename := range args[1:] {
		rf, err := os.Open(filename)
		checkf(err, "input file %d", i)
		defer rf.Close()

		r := snapshot.NewReader(rf)
		for {
			s, err := r.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			checkf(err, "next section")
			fmt.Printf("Copying %s section of %s\n", s.Type(), filepath.Base(filename))

			sr, err := s.Open()
			checkf(err, "read section")

			sw, err := w.Open(s.Type())
			checkf(err, "write section")

			_, err = io.Copy(sw, sr)
			checkf(err, "copy section")

			check(sw.Close())
		}
	}
	check(wf.Close())
}
