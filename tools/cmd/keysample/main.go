// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// keysample takes a Badger DB and a list of hashes (one hex hash
// per line, sorted), and prints each key's value as hex bytes.
// Used to inspect specific keys whose hashes appear in a diff.
package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: keysample <db-path> <hashes-file>")
		os.Exit(2)
	}
	dbPath, hashesFile := os.Args[1], os.Args[2]

	want := map[[32]byte]bool{}
	f, err := os.Open(hashesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", hashesFile, err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if len(line) < 64 {
			continue
		}
		raw, err := hex.DecodeString(line[:64])
		if err != nil {
			continue
		}
		var h [32]byte
		copy(h[:], raw)
		want[h] = true
	}
	f.Close()
	fmt.Fprintf(os.Stderr, "want %d hashes\n", len(want))

	db, err := badger.OpenV1(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	cs := db.Begin(nil, false)
	defer cs.Discard()
	err = cs.ForEach(func(k *record.Key, v []byte) error {
		h := k.Hash()
		if !want[h] {
			return nil
		}
		fmt.Printf("%s | len=%d | %s\n", hex.EncodeToString(h[:]), len(v), hex.EncodeToString(v))
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "iter: %v\n", err)
	}
}
