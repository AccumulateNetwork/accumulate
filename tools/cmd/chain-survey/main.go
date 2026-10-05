// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// chain-survey lists every chain of every account a stopped node's LevelDB
// database holds, with its length: one line per chain, tab separated --
// account, chain, type, count. It reads the accounts from the BPT.
//
//	chain-survey /path/to/accumulate.db > chains.tsv
package main

import (
	"bufio"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: chain-survey <accumulate.db>")
		os.Exit(2)
	}
	db, err := database.OpenLevelDB(os.Args[1], nil)
	check(err)
	defer db.Close()
	batch := db.Begin(false)
	defer batch.Discard()

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	var accounts, chains, other, bare int
	err = bpt.ForEach(batch.BPT(), func(key *record.Key, _ []byte) error {
		if key.Len() != 2 || key.Get(0) != "Account" {
			other++
			return nil
		}
		u, ok := key.Get(1).(*url.URL)
		if !ok {
			other++
			return nil
		}
		accounts++
		metas, err := batch.Account(u).Chains().Get()
		if err != nil {
			return fmt.Errorf("%v: chain index: %w", u, err)
		}
		if len(metas) == 0 {
			// An account with no chain index: name a few, and whether its
			// main state is held, so what they are can be told.
			bare++
			if bare <= 5 {
				st, err := batch.Account(u).Main().Get()
				fmt.Fprintf(os.Stderr, "no chains: %v (main: %T, %v)\n", u, st, err)
			}
		}
		for _, m := range metas {
			c, err := batch.Account(u).ChainByName(m.Name)
			if err != nil {
				return fmt.Errorf("%v#%s: %w", u, m.Name, err)
			}
			head, err := c.Inner().Head().Get()
			if err != nil {
				return fmt.Errorf("%v#%s: head: %w", u, m.Name, err)
			}
			chains++
			fmt.Fprintf(out, "%v\t%s\t%v\t%d\n", u, m.Name, m.Type, head.Count)
		}
		return nil
	})
	check(err)
	fmt.Fprintf(os.Stderr, "%d accounts (%d with no chains), %d chains, %d other BPT entries\n", accounts, bare, chains, other)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
