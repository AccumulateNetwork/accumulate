// snap-bptdiff compares the BPT entries in a snapshot file (the
// "source" set) against the BPT entries in the restored DB after
// running database.Restore on that same snapshot. Reports keys missing
// on restore, keys extra on restore, and keys present in both but with
// different values. Used to localize "BPT root does not match"
// regressions in snapshot round-trip.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	psnap "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: snap-bptdiff <snapshot>")
		os.Exit(1)
	}
	f, err := os.Open(os.Args[1])
	must(err)
	defer f.Close()

	rd, err := psnap.Open(f)
	must(err)
	fmt.Printf("snapshot: version=%d sections=%d root=%x\n",
		rd.Header.Version, len(rd.Sections), rd.Header.RootHash)

	// Read source BPT entries from the snapshot.
	if _, err := f.Seek(0, 0); err != nil {
		must(err)
	}
	rd2, err := psnap.Open(f)
	must(err)
	bptSec, err := rd2.OpenBPT(-1)
	must(err)
	source := map[[32]byte]*entry{}
	for {
		r, err := bptSec.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			must(err)
		}
		var v [32]byte
		copy(v[:], r.Value)
		source[r.Key.Hash()] = &entry{key: r.Key.String(), value: v}
	}
	fmt.Printf("snapshot has %d BPT entries\n", len(source))

	// Restore.
	if _, err := f.Seek(0, 0); err != nil {
		must(err)
	}
	db := database.OpenInMemory(nil)
	defer db.Close()
	fmt.Println("restoring (with current Restore code path)...")
	must(database.Restore(db, f, &database.RestoreOptions{SkipHashCheck: true}))

	// Read restored BPT.
	batch := db.Begin(false)
	defer batch.Discard()
	root, err := batch.GetBptRootHash()
	must(err)
	fmt.Printf("restored BPT root=%x  (source: %x)\n",
		root, rd.Header.RootHash)
	fmt.Printf("equal? %v\n", root == rd.Header.RootHash)

	restored := map[[32]byte]*entry{}
	it := batch.BPT().Iterate(1000)
	for it.Next() {
		for _, e := range it.Value() {
			var v [32]byte
			copy(v[:], e.Value[:])
			restored[e.Key.Hash()] = &entry{key: e.Key.String(), value: v}
		}
	}
	fmt.Printf("restored has %d BPT entries\n", len(restored))

	// Diff.
	var (
		onlySrc  []*entry
		onlyRst  []*entry
		valDiff  []*pairDiff
	)
	for h, s := range source {
		r, ok := restored[h]
		if !ok {
			onlySrc = append(onlySrc, s)
			continue
		}
		if s.value != r.value {
			valDiff = append(valDiff, &pairDiff{key: s.key, src: s.value, rst: r.value})
		}
	}
	for h, r := range restored {
		if _, ok := source[h]; !ok {
			onlyRst = append(onlyRst, r)
		}
	}

	fmt.Println()
	fmt.Printf("only in SOURCE (missing on restore): %d\n", len(onlySrc))
	fmt.Printf("only in RESTORED (extra after restore): %d\n", len(onlyRst))
	fmt.Printf("value differs: %d\n", len(valDiff))

	if n := min(20, len(onlySrc)); n > 0 {
		fmt.Println()
		fmt.Printf("--- first %d SOURCE-only ---\n", n)
		sort.Slice(onlySrc, func(i, j int) bool { return onlySrc[i].key < onlySrc[j].key })
		for _, e := range onlySrc[:n] {
			fmt.Printf("  %s -> %x\n", e.key, e.value[:8])
		}
	}
	if n := min(20, len(onlyRst)); n > 0 {
		fmt.Println()
		fmt.Printf("--- first %d RESTORED-only ---\n", n)
		sort.Slice(onlyRst, func(i, j int) bool { return onlyRst[i].key < onlyRst[j].key })
		for _, e := range onlyRst[:n] {
			fmt.Printf("  %s -> %x\n", e.key, e.value[:8])
		}
	}
	if n := len(valDiff); n > 0 {
		fmt.Println()
		fmt.Printf("--- ALL %d VALUE diffs ---\n", n)
		sort.Slice(valDiff, func(i, j int) bool { return valDiff[i].key < valDiff[j].key })
		for _, d := range valDiff {
			fmt.Printf("  %s\n    src: %x\n    rst: %x\n", d.key, d.src, d.rst)
		}
	}
}

type entry struct {
	key   string
	value [32]byte
}

type pairDiff struct {
	key      string
	src, rst [32]byte
}

var _ = bytes.Equal // imports tickle

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

var _ bpt.BPT // tickle import
