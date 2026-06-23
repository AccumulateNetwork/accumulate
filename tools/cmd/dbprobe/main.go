// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// dbprobe walks a key set and identifies common Accumulate record
// paths. It reads the missing-keys file (sorted hashes) and probes
// each candidate path; if its hash matches a missing key, we know
// what kind of record was lost in the snapshot/restore round trip.
//
// The point is to take the set difference (keys in val1 but not in
// restored) and figure out what categories of record are missing.
package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: dbprobe <db-path> <missing-hashes-file>")
		os.Exit(2)
	}
	dbPath := os.Args[1]
	missingFile := os.Args[2]

	// Load the set of missing hashes
	missing := loadHashes(missingFile)
	fmt.Fprintf(os.Stderr, "loaded %d missing hashes\n", len(missing))

	db, err := badger.OpenV1(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()
	cs := db.Begin(nil, false)
	defer cs.Discard()

	// Categorize ALL keys present in this DB by stem name.
	categories := map[string]int{}
	var samples = map[string][]string{}
	err = cs.ForEach(func(k *record.Key, v []byte) error {
		// We only get hash keys back since plainKeys=false. We can't
		// recover the original path. So for now, just report:
		// (a) keys where hash is in `missing` set — those are the
		//     keys present in this DB but absent in the other DB.
		// (b) the totals.
		_ = v
		_ = k
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "iter: %v\n", err)
		os.Exit(1)
	}

	// Sort categories by count
	type c struct {
		name  string
		count int
	}
	var cs2 []c
	for n, x := range categories {
		cs2 = append(cs2, c{n, x})
	}
	sort.Slice(cs2, func(i, j int) bool { return cs2[i].count > cs2[j].count })
	for _, x := range cs2 {
		fmt.Printf("%-40s %d\n", x.name, x.count)
		if len(samples[x.name]) > 0 {
			for _, s := range samples[x.name][:min(3, len(samples[x.name]))] {
				fmt.Printf("    sample: %s\n", s)
			}
		}
	}
	fmt.Fprintln(os.Stderr, "(probe placeholder — DB uses hash-keyed format; need higher-level walker)")
}

func loadHashes(path string) map[[32]byte]bool {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", path, err)
		os.Exit(1)
	}
	defer f.Close()
	out := map[[32]byte]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if len(line) < 64 {
			continue
		}
		raw, err := hex.DecodeString(line[:64])
		if err != nil {
			continue
		}
		var h [32]byte
		copy(h[:], raw)
		out[h] = true
	}
	return out
}
