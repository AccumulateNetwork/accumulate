package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var enUsTitle = cases.Title(language.AmericanEnglish)

var dumpCmd = &cobra.Command{
	Use:   "dump <snapshot>",
	Short: "Dump the contents of a snapshot",
	Args:  cobra.ExactArgs(1),
	Run:   dumpSnapshot,
}

var dumpFlag = struct {
	Short bool
}{}

func init() {
	cmd.AddCommand(dumpCmd)
	dumpCmd.Flags().BoolVarP(&dumpFlag.Short, "short", "s", false, "Short output")
}

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
	if dumpFlag.Short {
		return snapshot.ErrSkip
	}
	return nil
}

func (dumpVisitor) VisitAccount(acct *snapshot.Account, _ int) error {
	if acct == nil {
		return nil
	}

	fmt.Printf("  Account %v (%v)\n", acct.Url, acct.Main.Type())

	for _, chain := range acct.Chains {
		fmt.Printf("    Chain %s (%v) height %d with %d entries\n", chain.Name, chain.Type, chain.Count, len(chain.Entries))
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
