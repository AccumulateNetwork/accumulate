// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

// Head-to-head of the two REAL storage backends behind the keyvalue interface:
// pkg/database/keyvalue/leveldb (today's default) and
// pkg/database/keyvalue/block (the append-only record-file store with an
// on-disk index tree — the "major block file" design). Same workload, same
// interface, so the comparison isolates the storage layout.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/block"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// runReal drives mode=real-leveldb or mode=real-blockdb.
func runReal(which string) {
	os.RemoveAll(*dir)
	check(os.MkdirAll(*dir, 0o755))

	var db keyvalue.Beginner
	var closer func() error
	switch which {
	case "real-leveldb":
		d, err := leveldb.Open(filepath.Join(*dir, "leveldb"))
		check(err)
		db, closer = d, d.Close
	case "real-blockdb":
		d, err := block.Open(filepath.Join(*dir, "blockdb"))
		check(err)
		db, closer = d, d.Close
	}

	payload := make([]byte, 4096)
	r := &rng{s: 42}
	for i := range payload {
		payload[i] = byte(r.next())
	}

	var (
		userBytes    int64
		peakHeap     uint64
		readImmNs    int64
		readImmCount int64
		readBptNs    int64
		readBptCount int64
		ms           runtime.MemStats
	)
	bptWritten := make([]bool, *bptKeys)

	// Keys are built the same way in both backends: a record.Key whose hash is
	// what the store indexes.
	immRecKey := func(tx, slot int) *record.Key { return record.NewKey("Imm", uint64(tx), uint64(slot)) }
	bptRecKey := func(i int) *record.Key { return record.NewKey("BPT", uint64(i)) }

	start := time.Now()
	blocks := *txs / *blockTxs
	for b := 0; b < blocks; b++ {
		// One changeset per block — block-per-commit, as the node does.
		cs := db.Begin(nil, true)
		for t := 0; t < *blockTxs; t++ {
			tx := b**blockTxs + t
			for slot, size := range immSizes {
				check(cs.Put(immRecKey(tx, slot), payload[:size]))
				userBytes += int64(size)
			}
			for j := 0; j < bptWritesPerTx; j++ {
				i := r.intn(*bptKeys)
				bptWritten[i] = true
				check(cs.Put(bptRecKey(i), payload[:bptValSize]))
				userBytes += bptValSize
			}
		}
		check(cs.Commit())

		// Reads go through a fresh read-only view, like a query handler.
		rv := db.Begin(nil, false)
		maxTx := (b + 1) * *blockTxs
		for t := 0; t < *blockTxs; t++ {
			for j := 0; j < *readsImm; j++ {
				k := immRecKey(r.intn(maxTx), r.intn(len(immSizes)))
				t0 := time.Now()
				_, err := rv.Get(k)
				check(err)
				readImmNs += time.Since(t0).Nanoseconds()
				readImmCount++
			}
			for j := 0; j < *readsBpt; j++ {
				i := r.intn(*bptKeys)
				if !bptWritten[i] {
					continue
				}
				t0 := time.Now()
				_, err := rv.Get(bptRecKey(i))
				check(err)
				readBptNs += time.Since(t0).Nanoseconds()
				readBptCount++
			}
		}
		rv.Discard()

		if b%50 == 0 || b == blocks-1 {
			runtime.ReadMemStats(&ms)
			if ms.HeapInuse > peakHeap {
				peakHeap = ms.HeapInuse
			}
			fmt.Printf("block %4d/%d  heapInuse=%4dMB  disk=%5dMB\n",
				b+1, blocks, ms.HeapInuse>>20, dirSize(*dir)>>20)
		}
	}
	elapsed := time.Since(start)

	runtime.GC()
	runtime.ReadMemStats(&ms)
	fmt.Printf(`
=== %s%s: %d txs, %d tx/block, %.0f tx/s wall ===
user bytes written        %6d MB
on disk                   %6d MB
heap in use (post-GC)     %6d MB   (peak during run: %d MB)
read imm  p_avg           %6.1f µs  (n=%d)
read bpt  p_avg           %6.1f µs  (n=%d)
`,
		which, *label, *txs, *blockTxs, float64(*txs)/elapsed.Seconds(),
		userBytes>>20, dirSize(*dir)>>20,
		ms.HeapInuse>>20, peakHeap>>20,
		float64(readImmNs)/1e3/float64(max64(readImmCount, 1)), readImmCount,
		float64(readBptNs)/1e3/float64(max64(readBptCount, 1)), readBptCount)

	check(closer())
}
