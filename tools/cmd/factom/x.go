// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdX = &cobra.Command{
	Use:   "x",
	Short: "For experiments and stuff",
	Run:   X,
}

func init() {
	cmd.AddCommand(cmdX)
}

func X(_ *cobra.Command, args []string) {
	input, err := os.Open(args[0])
	check(err)
	x := new(xVisitor)
	check(snapshot.Visit(input, x))

	sort.Slice(x.accounts, func(i, j int) bool {
		return x.accounts[i].Chains[0].Head.Count > x.accounts[j].Chains[0].Head.Count
	})

	logger := newLogger()
	store := memory.New(logger.With("module", "storage"))
	batch := store.Begin(true)
	defer batch.Discard()
	bpt := pmt.NewBPTManager(batch)

	hasher := make(hash.Hasher, 4)
	lookup := map[[32]byte]*snapshot.Account{}
	for _, a := range x.accounts[20:25] {
		fmt.Printf("Account %v has %d entries\n", a.Url, a.Chains[0].Head.Count)
		hasher = hasher[:0]
		b, _ := a.Main.MarshalBinary()
		hasher.AddBytes(b)
		hasher.AddValue(hashSecondaryState(a))
		hasher.AddValue(hashChains(a))
		hasher.AddValue(hashTransactions(a))
		chain, err := a.Main.(*protocol.LiteDataAccount).AccountId()
		check(err)
		lookup[*(*[32]byte)(chain)] = a
		bpt.InsertKV(*(*[32]byte)(chain), *(*[32]byte)(hasher.MerkleHash()))
	}
	check(bpt.Bpt.Update())

	f, err := os.Create(args[1])
	checkf(err, "create snapshot")
	w, err := snapshot.Create(f, new(snapshot.Header))
	checkf(err, "initialize snapshot")
	sw, err := w.Open(snapshot.SectionTypeAccounts)
	checkf(err, "open accounts snapshot")
	check(bpt.Bpt.SaveSnapshot(sw, func(key storage.Key, _ [32]byte) ([]byte, error) {
		b, err := lookup[key].MarshalBinary()
		if err != nil {
			return nil, errors.Format(errors.StatusEncodingError, "marshal account: %w", err)
		}
		return b, nil
	}))
	check(sw.Close())
}

type xVisitor struct {
	accounts []*snapshot.Account
}

func (x *xVisitor) VisitAccount(a *snapshot.Account, _ int) error {
	if a != nil {
		x.accounts = append(x.accounts, a)
	}
	return nil
}
