// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// keydump dumps every key/value pair in a Badger DB. By default
// the underlying badger uses hash-keyed format so we can't recover
// the original record path from a hash; we still emit the hash and
// value summary, which is enough to set-diff two DBs.
//
// Usage:
//   keydump <badger-db-path>           prints "<hash> | len=<n> sha=<8>"
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: keydump <badger-db-path>")
		os.Exit(2)
	}
	dbPath := os.Args[1]

	db, err := badger.OpenV1(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	type entry struct {
		keyHash [32]byte
		valLen  int
		valSha  [32]byte
	}
	var entries []entry

	cs := db.Begin(nil, false)
	defer cs.Discard()
	err = cs.ForEach(func(k *record.Key, v []byte) error {
		entries = append(entries, entry{
			keyHash: k.Hash(),
			valLen:  len(v),
			valSha:  sha256.Sum256(v),
		})
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "iter: %v\n", err)
		os.Exit(1)
	}
	sort.Slice(entries, func(i, j int) bool {
		hi, hj := entries[i].keyHash[:], entries[j].keyHash[:]
		for x := range 32 {
			if hi[x] != hj[x] {
				return hi[x] < hj[x]
			}
		}
		return false
	})

	for _, e := range entries {
		fmt.Printf("%s | len=%d sha=%s\n", hex.EncodeToString(e.keyHash[:]), e.valLen, hex.EncodeToString(e.valSha[:8]))
	}
	fmt.Fprintf(os.Stderr, "%d entries\n", len(entries))
}
