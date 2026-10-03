// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// tear-store reproduces, on purpose, the tear a kill left on two validators in
// run 20261002T145702Z (#4507, BlockchainDB#99): it shortens the Tail chunk
// of a partition's ledger root chain that starts the open mark set, so the
// chain's head names hashes the chunk no longer holds. The
// node whose store it is must find the tear at start (join.CheckStore) and
// join instead of resuming. Run it on a stopped node's store only.
//
//	tear-store <path/to/accumulate.db> <partition, e.g. BVN2>
package main

import (
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: tear-store <accumulate.db> <partition>")
		os.Exit(2)
	}
	store, err := bcdb.Open(os.Args[1])
	check(err)
	defer func() { check(store.Close()) }()
	db := database.New(store, nil)

	batch := db.Begin(true)
	c := batch.Account(protocol.PartitionUrl(os.Args[2]).JoinPath(protocol.Ledger)).RootChain().Inner()
	head, err := c.Head().Get()
	check(err)
	open := head.Count & c.MarkMask()
	if open < 2 {
		fmt.Fprintf(os.Stderr, "the open mark set holds %d entries; nothing to tear\n", open)
		os.Exit(1)
	}
	// The open mark set's first chunk, which holds at least two of its
	// entries, cut to one hash.
	k := uint64(0)
	chunk, err := c.Tail(k).Get()
	check(err)
	was := len(chunk.Hashes)
	chunk.Hashes = chunk.Hashes[:1]
	check(c.Tail(k).Put(chunk))
	check(batch.Commit())
	fmt.Printf("tore %s ledger RootChain Tail(%d): %d hashes -> 1 (head count %d)\n", os.Args[2], k, was, head.Count)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
