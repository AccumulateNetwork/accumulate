// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// modelprobe reports what an account holds in an offline database, using the
// model's own accessors rather than hand-built record keys.
//
// That distinction is the point of the tool. During the investigation in
// docs/incidents/2025-07-13-reorg-history-loss.md, hand-built keys reported the
// Directory's anchor chains as absent when they were present, and the mistake
// was caught only because a control against a chain known to exist failed the
// same way. Asking the model cannot make that error.
//
// Read-only. Badger and LevelDB are both accepted; the backend is detected from
// the directory's own markers.
//
//	modelprobe <database> <account> [partition ...]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: modelprobe <database> <account> [partition ...]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(2)
	}
	path, account := flag.Arg(0), flag.Arg(1)

	store, err := open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}

	// coredb.Close closes the store with it
	db := coredb.New(store, nil)
	defer db.Close()

	u, err := url.Parse(account)
	if err != nil {
		fmt.Fprintln(os.Stderr, "account:", err)
		os.Exit(1)
	}

	batch := db.Begin(false)
	defer batch.Discard()
	acct := batch.Account(u)

	// The registry is the account's own list of chains, so it answers "what is
	// here" without anyone having to guess a name.
	chains, err := acct.Chains().Get()
	if err != nil {
		fmt.Fprintln(os.Stderr, "chains:", err)
	} else {
		fmt.Printf("%v: %d chains\n", u, len(chains))
		for _, c := range chains {
			fmt.Printf("  %-34s %v\n", c.Name, c.Type)
		}
	}

	// Anchor chains are addressed by partition, not by their display name
	for _, part := range flag.Args()[2:] {
		head, err := acct.AnchorChain(part).Root().Head().Get()
		if err != nil {
			fmt.Printf("  anchor(%v) root: %v\n", part, err)
			continue
		}
		fmt.Printf("  anchor(%v) root: %d entries\n", part, head.Count)
	}
}

// open detects the backend from the markers each one writes, so the caller does
// not have to know which wrote the directory.
func open(path string) (keyvalue.Beginner, error) {
	switch {
	case exists(path, "CURRENT"):
		return leveldb.Open(path)
	case exists(path, "MANIFEST"), exists(path, "KEYREGISTRY"):
		return badger.OpenV1(path)
	default:
		return nil, fmt.Errorf("%s: no LevelDB or Badger marker", path)
	}
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}
