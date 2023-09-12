// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cometbft/cometbft/types"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
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
	rd, ver := openSnapshotFile(args[0])
	if c, ok := rd.(io.Closer); ok {
		defer c.Close()
	}

	switch ver {
	case sv1.Version1:
		dumpV1(rd)
	case sv2.Version2:
		dumpV2(rd)
	default:
		fatalf("I don't know how to handle snapshot version %d", ver)
	}
}

func openSnapshotFile(filename string) (ioutil.SectionReader, uint64) {
	var rd ioutil.SectionReader
	if filepath.Ext(filename) == ".json" {
		fmt.Fprintf(os.Stderr, "Loading %s\n", filename)
		genDoc, err := types.GenesisDocFromFile(filename)
		checkf(err, "read %s", filename)

		var b []byte
		check(json.Unmarshal(genDoc.AppState, &b))
		rd = bytes.NewReader(b)

	} else {
		f, err := os.Open(filename)
		checkf(err, "open snapshot %s", filename)
		rd = f
	}

	ver, err := sv2.GetVersion(rd)
	check(err)
	return rd, ver
}

func dumpV2(f ioutil.SectionReader) {
	r, err := sv2.Open(f)
	check(err)

	fmt.Printf("%d section(s)\n", len(r.Sections))
	for i, s := range r.Sections {
		typstr := s.Type().String()
		fmt.Printf("%s%s section at %d (size %d)\n", enUsTitle.String(typstr[:1]), typstr[1:], s.Offset(), s.Size())

		if flagSnapDump.Short {
			continue
		}

		switch s.Type() {
		case sv2.SectionTypeRecords:
			rr, err := r.OpenRecords(i)
			check(err)

			for {
				re, err := rr.Read()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					check(err)
				}

				fmt.Printf("  %v\n", re.Key)
			}
		}
	}
}

func dumpV1(f ioutil.SectionReader) {
	r := sv1.NewReader(f)
	s, err := r.Next()
	checkf(err, "find header")
	sr, err := s.Open()
	checkf(err, "open header")
	header := new(sv1.Header)
	_, err = header.ReadFrom(sr)
	checkf(err, "read header")

	fmt.Printf("Header section at %d (size %d)\n", s.Offset(), s.Size())
	fmt.Printf("  Version   %d\n", header.Version)
	fmt.Printf("  Height    %d\n", header.Height)
	fmt.Printf("  Root Hash %x\n", header.RootHash)

	_, err = f.Seek(0, io.SeekStart)
	check(err)

	check(sv1.Visit(f, dumpVisitor{}))
}

type dumpVisitor struct{}

func (dumpVisitor) VisitSection(s *sv1.ReaderSection) error {
	typstr := s.Type().String()
	fmt.Printf("%s%s section at %d (size %d)\n", enUsTitle.String(typstr[:1]), typstr[1:], s.Offset(), s.Size())

	switch {
	case !flagSnapDump.Short,
		flagSnapDump.Verify && s.Type() == sv1.SectionTypeAccounts:
		return nil
	default:
		return sv1.ErrSkip
	}
}

var dumpDb = database.OpenInMemory(nil)

func (dumpVisitor) VisitAccount(acct *sv1.Account, _ int) error {
	if acct == nil {
		return nil
	}

	if !flagSnapDump.Short {
		var typ string
		if acct.Main == nil {
			typ = "<nil>"
		} else {
			typ = acct.Main.Type().String()
		}
		fmt.Printf("  Account %v (%v)\n", acct.Url, typ)

		for _, chain := range acct.Chains {
			fmt.Printf("    Chain %s (%v) height %d with %d mark points\n", chain.Name, chain.Type, chain.Head.Count, len(chain.MarkPoints))
		}
	}

	if flagSnapDump.Verify {
		batch := dumpDb.Begin(true)
		defer batch.Discard()

		err := acct.Restore(batch)
		checkf(err, "restore %v", acct.Url)

		for _, c := range acct.Chains {
			c2, err := acct.RestoreChainHead(batch, c)
			checkf(err, "restore %v %s chain", acct.Url, c.Name)
			err = c.RestoreMarkPointRange(c2.Inner(), 0, len(c.MarkPoints))
			checkf(err, "restore %v %s chain", acct.Url, c.Name)
		}

		err = batch.Account(acct.Url).VerifyHash(acct.Hash[:])
		checkf(err, "verify %v", acct.Url)
	}

	return nil
}

func (dumpVisitor) VisitTransaction(txn *sv1.Transaction, _ int) error {
	if txn == nil {
		return nil
	}
	fmt.Printf("    Transaction %x, %v, %v \n", txn.Transaction.GetHash()[:4], txn.Transaction.Body.Type(), txn.Transaction.Header.Principal)
	return nil
}

func (dumpVisitor) VisitSignature(sig *sv1.Signature, _ int) error {
	if sig == nil {
		return nil
	}
	fmt.Printf("    Signature %x (%v)\n", sig.Signature.Hash()[:4], sig.Signature.Type())
	return nil
}
