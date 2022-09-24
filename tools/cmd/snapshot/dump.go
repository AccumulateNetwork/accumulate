package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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
	Short  bool
	Verify bool
}{}

func init() {
	cmd.AddCommand(dumpCmd)
	dumpCmd.Flags().BoolVarP(&dumpFlag.Short, "short", "s", false, "Short output")
	dumpCmd.Flags().BoolVar(&dumpFlag.Verify, "verify", false, "Verify the hash of accounts")
}

func dumpSnapshot(_ *cobra.Command, args []string) {
	filename := args[0]
	f, err := os.Open(filename)
	checkf(err, "open snapshot %s", filename)
	defer f.Close()

	r := snapshot.NewReader(f)
	s, err := r.Next()
	checkf(err, "find header")
	sr, err := s.Open()
	checkf(err, "open header")
	header := new(snapshot.Header)
	_, err = header.ReadFrom(sr)
	checkf(err, "read header")

	fmt.Printf("Header section at %d (size %d)\n", s.Offset(), s.Size())
	fmt.Printf("  Version   %d\n", header.Version)
	fmt.Printf("  Height    %d\n", header.Height)
	fmt.Printf("  Root Hash %x\n", header.RootHash)

	_, err = f.Seek(0, io.SeekStart)
	check(err)

	check(snapshot.Visit(f, dumpVisitor{}))
}

type dumpVisitor struct{}

func (dumpVisitor) VisitSection(s *snapshot.ReaderSection) error {
	typstr := s.Type().String()
	fmt.Printf("%s%s section at %d (size %d)\n", enUsTitle.String(typstr[:1]), typstr[1:], s.Offset(), s.Size())

	switch {
	case !dumpFlag.Short,
		dumpFlag.Verify && s.Type() == snapshot.SectionTypeAccounts:
		return nil
	default:
		return snapshot.ErrSkip
	}
}

var dumpDb = database.OpenInMemory(nil)

func (dumpVisitor) VisitAccount(acct *snapshot.Account, _ int) error {
	if acct == nil {
		return nil
	}

	if !dumpFlag.Short {
		fmt.Printf("  Account %v (%v)\n", acct.Url, acct.Main.Type())

		for _, chain := range acct.Chains {
			fmt.Printf("    Chain %s (%v) height %d with %d entries\n", chain.Name, chain.Type, chain.Count, len(chain.Entries))
		}
	}

	if dumpFlag.Verify {
		batch := dumpDb.Begin(true)
		defer batch.Discard()

		err := acct.Restore(batch)
		checkf(err, "restore %v", acct.Url)

		for _, c := range acct.Chains {
			err = c.Restore(batch.Account(acct.Url))
			checkf(err, "restore %v %s chain", acct.Url, c.Name)
		}

		err = batch.Account(acct.Url).VerifyHash(acct.Hash[:])
		checkf(err, "verify %v", acct.Url)
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
