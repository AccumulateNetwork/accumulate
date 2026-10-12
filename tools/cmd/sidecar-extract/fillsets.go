// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"google.golang.org/protobuf/encoding/protowire"
)

// fillSets rebuilds mark sets of the Directory's chains whose mark point and
// some entries are lost, from the Directory's CometBFT blocks.
//
// Each chain's entries come from a stream built from the blocks, in chain
// order: the roots each Directory anchor carries (anchor(directory)-root, the
// ledger's bpt), the anchors as produced (anchor-sequence), the validators'
// block anchors (anchors' signature), index entries built from each anchor's
// block and root index (the ledger's root-index), and for an account chain's
// index chain, the root chain entries that are that chain's anchor after one
// of its entries. A run of lost entries is filled with the stream's items
// between its known neighbours.
//
// Nothing is written unless it is proven: the set's 256 entries on the mark
// point before it, followed by the next set's 256 known entries, must give the
// next stored mark point exactly. The encodings of an element and of a mark
// point are first proven on records that exist.
func fillSets(archiveArgs []string, blockstore, sidecar, out string, targets []string) error {
	var copies []*archive
	for _, arg := range archiveArgs {
		n, path, _ := strings.Cut(arg, "=")
		db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
		if err != nil {
			return fmt.Errorf("archive %s: %w", n, err)
		}
		copies = append(copies, &archive{name: n, db: db, log: &vlog{dir: path}})
	}
	store := &verifiedStore{archives: copies}
	side, err := leveldb.OpenFile(sidecar, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return err
	}
	defer side.Close()
	store.extra = append(store.extra, side)
	batch := coredb.New(store, nil).Begin(false)
	defer batch.Discard()
	blocks, err := leveldb.OpenFile(blockstore, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return err
	}
	defer blocks.Close()
	dst, err := openOutput(out)
	if err != nil {
		return err
	}
	defer dst.Close()

	// The Directory's own anchors, and the validators' block anchors, from
	// the blocks of the range
	anchors := map[uint64]*protocol.DirectoryAnchor{}
	var blockAnchors [][]byte
	seenBA := map[[32]byte]bool{}
	for n := *flagFrom; n <= *flagTo; n++ {
		txs, ok, err := cometBlock(blocks, n)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		var visit func(messaging.Message)
		visit = func(m messaging.Message) {
			switch m := m.(type) {
			case *messaging.TransactionMessage:
				if a, ok := m.Transaction.Body.(*protocol.DirectoryAnchor); ok && a.Source.Equal(protocol.DnUrl()) {
					if _, ok := anchors[a.MinorBlockIndex]; !ok {
						anchors[a.MinorBlockIndex] = a
					}
				}
			case *messaging.SequencedMessage:
				visit(m.Message)
			case *messaging.SyntheticMessage:
				visit(m.Message)
			case *messaging.BlockAnchor:
				h := m.Hash()
				if !seenBA[h] {
					seenBA[h] = true
					blockAnchors = append(blockAnchors, h[:])
				}
				visit(m.Anchor)
			}
		}
		for _, m := range decodeBlock(txs) {
			visit(m)
		}
	}
	var minors []uint64
	for n := range anchors {
		minors = append(minors, n)
	}
	sort.Slice(minors, func(i, j int) bool { return minors[i] < minors[j] })
	log.Printf("%d Directory anchors, %d block anchors in blocks %d..%d", len(anchors), len(blockAnchors), *flagFrom, *flagTo)

	encodeElement := func(entry []byte) []byte {
		return append(binary.AppendUvarint(nil, uint64(len(entry))), entry...)
	}
	raw := func(key [32]byte) ([]byte, bool) {
		for _, a := range copies {
			txn := a.db.NewTransaction(false)
			moves := a.newMoves(txn)
			item, err := txn.Get(key[:])
			var v []byte
			if err == nil {
				v, err = a.resolve(item, moves)
			}
			moves.Close()
			txn.Discard()
			if err == nil {
				return v, true
			}
		}
		if v, err := side.Get(key[:], nil); err == nil {
			return v, true
		}
		return nil, false
	}

	var rootAt map[[32]byte]uint64
	for _, target := range targets {
		// "<account> <chain> <set>"
		f := strings.Fields(target)
		acct, err := url.Parse(f[0])
		if err != nil {
			return err
		}
		name := f[1]
		set, _ := strconv.ParseUint(f[2], 10, 64)
		ch, err := batch.Account(acct).ChainByName(name)
		if err != nil {
			return err
		}
		m := ch.Inner()
		freq := uint64(m.MarkFreq())
		first, last := set*freq, set*freq+freq-1
		report := func(format string, args ...any) {
			fmt.Printf("%v %s set %d: %s\n", acct, name, set, fmt.Sprintf(format, args...))
		}

		// What is known: the set's entries that are readable, and the
		// entries on either side of it
		entries := make([][]byte, freq)
		var missing []uint64
		for i := first; i <= last; i++ {
			if e, err := ch.Entry(int64(i)); err == nil {
				entries[i-first] = e
			} else {
				missing = append(missing, i)
			}
		}
		prev, err := m.States(first - 1).Get()
		if err != nil {
			report("no mark point before it: %v", err)
			continue
		}
		next, err := m.States(last + freq).Get()
		if err != nil {
			report("no mark point after it: %v", err)
			continue
		}
		before, after := prev.HashList[len(prev.HashList)-1], next.HashList[0]

		// The chain's stream
		var stream [][]byte
		var rootIndexSuffix []byte
		switch {
		case name == "bpt" && acct.Equal(protocol.DnUrl().JoinPath(protocol.Ledger)):
			for _, n := range minors {
				stream = append(stream, anchors[n].StateTreeAnchor[:])
			}
		case name == "anchor(directory)-root":
			for _, n := range minors {
				stream = append(stream, anchors[n].RootChainAnchor[:])
			}
		case name == "anchor(directory)-bpt":
			for _, n := range minors {
				stream = append(stream, anchors[n].StateTreeAnchor[:])
			}
		case name == "anchor-sequence":
			for _, n := range minors {
				bare := &protocol.Transaction{Body: anchors[n]}
				stream = append(stream, bare.GetHash())
			}
		case name == "signature":
			stream = blockAnchors
		case name == "root-index":
			// One entry per block that changed a chain: the root chain grows
			// by one anchor per chain changed, and the time is the ledger's.
			// The running root index is re-taken from every known entry.
			known := map[uint64]uint64{} // block -> root index
			knownEntry := map[uint64][]byte{}
			var lo, hi protocol.IndexEntry
			for _, e := range append(append([][]byte{before}, entries...), after) {
				var ie protocol.IndexEntry
				if e != nil && ie.UnmarshalBinary(e) == nil {
					known[ie.BlockIndex] = ie.Source
					knownEntry[ie.BlockIndex] = e
				}
			}
			if lo.UnmarshalBinary(before) != nil || hi.UnmarshalBinary(after) != nil {
				report("cannot read the neighbouring entries")
				continue
			}
			// The stored entries carry bytes the type does not decode; they
			// must be the same on every known entry to be carried over
			var suffix []byte
			consistent := true
			for i, e := range append(append([][]byte{before}, entries...), after) {
				var ie protocol.IndexEntry
				if e == nil || ie.UnmarshalBinary(e) != nil {
					continue
				}
				// A decoded entry keeps what it did not decode and writes it
				// back; a fresh one from its fields does not
				fresh := &protocol.IndexEntry{Source: ie.Source, Anchor: ie.Anchor, BlockIndex: ie.BlockIndex, BlockTime: ie.BlockTime, RootIndexIndex: ie.RootIndexIndex}
				b, err := fresh.MarshalBinary()
				if err != nil || !bytes.HasPrefix(e, b) {
					consistent = false
					break
				}
				if i == 0 {
					suffix = e[len(b):]
				} else if !bytes.Equal(suffix, e[len(b):]) {
					consistent = false
					break
				}
			}
			if !consistent {
				report("the known entries do not share one encoding")
				continue
			}
			rootIndexSuffix = suffix
			src := lo.Source
			for n := lo.BlockIndex; n <= hi.BlockIndex; n++ {
				var bl *protocol.BlockLedger
				if err := batch.Account(protocol.DnUrl().JoinPath(protocol.Ledger, fmt.Sprint(n))).Main().GetAs(&bl); err != nil {
					// No ledger to count from; a known entry still stands
					if s, ok := known[n]; ok {
						src = s
						stream = append(stream, knownEntry[n])
					}
					continue
				}
				chains := map[string]bool{}
				for _, e := range bl.Entries {
					chains[e.Account.String()+"#"+e.Chain] = true
				}
				if n > lo.BlockIndex {
					src += uint64(len(chains))
				}
				if s, ok := known[n]; ok {
					// A known entry stands as it is
					src = s
					stream = append(stream, knownEntry[n])
					continue
				}
				if len(chains) == 0 {
					continue
				}
				t := bl.Time.UTC()
				b, err := (&protocol.IndexEntry{Source: src, BlockIndex: n, BlockTime: &t}).MarshalBinary()
				if err != nil {
					return err
				}
				stream = append(stream, append(b, suffix...))
			}
		case strings.HasSuffix(name, "-index"):
			// The indexed chain's anchor after each of its entries, matched
			// against the root chain entries, with the block of each from the
			// root index chain, carrying what the known entries carry
			suffix, ok := learnSuffix(append(append([][]byte{before}, entries...), after))
			if !ok {
				report("the known entries do not share one encoding")
				continue
			}
			stream, err = indexStream(batch, acct, strings.TrimSuffix(name, "-index"), before, after, suffix)
			if err != nil {
				report("index stream: %v", err)
				continue
			}
		default:
			report("no stream for this chain")
			continue
		}
		pos := map[string]int{}
		for i, s := range stream {
			if _, ok := pos[string(s)]; !ok {
				pos[string(s)] = i
			}
		}

		// Check the stream reproduces the known entries in order
		var known, inOrder int
		lastPos := -1
		for _, e := range append(append([][]byte{before}, entries...), after) {
			if e == nil {
				continue
			}
			known++
			if p, ok := pos[string(e)]; ok && p > lastPos {
				inOrder++
				lastPos = p
			}
		}
		if inOrder != known && *flagDebug && name != "root-index" && strings.HasSuffix(name, "-index") {
			for x := 0; x < 3; x++ {
				var ie protocol.IndexEntry
				if entries[x] != nil && ie.UnmarshalBinary(entries[x]) == nil {
					fmt.Printf("DEBUG known %d: %x source %d anchor %d block %d\n", first+uint64(x), entries[x], ie.Source, ie.Anchor, ie.BlockIndex)
				}
			}
			for x := 0; x < 3 && x < len(stream); x++ {
				var ie protocol.IndexEntry
				_ = ie.UnmarshalBinary(stream[x])
				fmt.Printf("DEBUG stream %d: %x source %d anchor %d\n", x, stream[x], ie.Source, ie.Anchor)
			}
			fmt.Printf("DEBUG stream length %d\n", len(stream))
		}
		if inOrder != known && *flagDebug && name == "root-index" {
			shown := 0
			for _, e := range entries {
				if e == nil || shown >= 4 {
					continue
				}
				if _, ok := pos[string(e)]; ok {
					continue
				}
				var ie protocol.IndexEntry
				_ = ie.UnmarshalBinary(e)
				for _, st := range stream {
					var se protocol.IndexEntry
					if se.UnmarshalBinary(st) == nil && se.BlockIndex == ie.BlockIndex {
						fmt.Printf("DEBUG block %d want source %d time %v; mine source %d time %v\n", ie.BlockIndex, ie.Source, ie.BlockTime, se.Source, se.BlockTime)
					}
				}
				shown++
			}
		}
		if inOrder != known && *flagDebug {
			for i := 0; i < 3; i++ {
				e := entries[i]
				if e == nil {
					continue
				}
				var ie protocol.IndexEntry
				if ie.UnmarshalBinary(e) == nil && name == "root-index" {
					t, _ := headerTime(blocks, ie.BlockIndex)
					a := anchors[ie.BlockIndex]
					ri := uint64(0)
					if a != nil {
						ri = a.RootChainIndex
					}
					fmt.Printf("DEBUG known root-index %d: source %d block %d time %v; header time %v; anchor root index %d (anchor present %v)\n", first+uint64(i), ie.Source, ie.BlockIndex, ie.BlockTime, t, ri, a != nil)
					if a == nil {
						// nearest anchors
						for _, n := range minors {
							if n > ie.BlockIndex-5 && n < ie.BlockIndex+5 {
								fmt.Printf("DEBUG   anchor minor %d root index %d\n", n, anchors[n].RootChainIndex)
							}
						}
					}
				} else {
					fmt.Printf("DEBUG known %s %d: %x (in stream %v)\n", name, first+uint64(i), e, pos[string(e)] > 0)
				}
			}
		}
		// The ledger's bpt gets an entry every block, the anchors carry only
		// some; every carried root is a candidate, and the root chain decides
		sparse := name == "bpt" && acct.Equal(protocol.DnUrl().JoinPath(protocol.Ledger))
		if inOrder != known && !sparse {
			report("the stream holds %d of %d known entries in order; not used", inOrder, known)
			continue
		}

		// Fill each run of lost entries from between its neighbours
		neighbour := func(i int64) []byte {
			switch {
			case i < int64(first):
				return before
			case i > int64(last):
				return after
			}
			return entries[uint64(i)-first]
		}
		// Each run's candidate fillings: the stream's items between its
		// neighbours, all of them when they fit exactly, else every in-order
		// choice; for root-index with nothing between, every block and root
		// index between the neighbours
		type run struct {
			a, b  uint64
			cands [][][]byte
		}
		var runs []run
		ok := true
		for k := 0; k < len(missing); {
			a := missing[k]
			b := a
			for k+1 < len(missing) && missing[k+1] == b+1 {
				k++
				b = missing[k]
			}
			k++
			n := int(b - a + 1)
			r := run{a: a, b: b}
			if sparse {
				if n != 1 {
					report("entries %d..%d: more than one lost entry in a row", a, b)
					ok = false
					break
				}
				for _, c := range stream {
					r.cands = append(r.cands, [][]byte{c})
				}
				runs = append(runs, r)
				continue
			}
			p, q := pos[string(neighbour(int64(a)-1))], pos[string(neighbour(int64(b)+1))]
			between := stream[p+1 : q]
			switch {
			case len(between) == n:
				r.cands = [][][]byte{between}
			case len(between) > n && len(between) <= 40:
				pick := make([]int, n)
				var choose func(start, j int)
				choose = func(start, j int) {
					if len(r.cands) > 20000 {
						return
					}
					if j == n {
						c := make([][]byte, n)
						for x, i := range pick {
							c[x] = between[i]
						}
						r.cands = append(r.cands, c)
						return
					}
					for i := start; i <= len(between)-(n-j); i++ {
						pick[j] = i
						choose(i+1, j+1)
					}
				}
				choose(0, 0)
			case name == "root-index" && n == 1:
				var lo, hi protocol.IndexEntry
				if lo.UnmarshalBinary(neighbour(int64(a)-1)) == nil && hi.UnmarshalBinary(neighbour(int64(b)+1)) == nil {
					for blk := lo.BlockIndex + 1; blk < hi.BlockIndex; blk++ {
						var t time.Time
						var bl *protocol.BlockLedger
						if batch.Account(protocol.DnUrl().JoinPath(protocol.Ledger, fmt.Sprint(blk))).Main().GetAs(&bl) == nil {
							t = bl.Time.UTC()
						} else if ht, ok := headerTime(blocks, blk); ok {
							t = ht.Truncate(time.Second)
						} else {
							continue
						}
						for src := lo.Source; src <= hi.Source; src++ {
							e, err := (&protocol.IndexEntry{Source: src, BlockIndex: blk, BlockTime: &t}).MarshalBinary()
							if err != nil {
								return err
							}
							r.cands = append(r.cands, [][]byte{append(e, rootIndexSuffix...)})
						}
					}
				}
			}
			if len(r.cands) == 0 && *flagDebug && name == "root-index" {
				var lo, hi protocol.IndexEntry
				_ = lo.UnmarshalBinary(neighbour(int64(a) - 1))
				_ = hi.UnmarshalBinary(neighbour(int64(b) + 1))
				fmt.Printf("DEBUG neighbours: %d source %d block %d time %v | %d source %d block %d time %v\n", a-1, lo.Source, lo.BlockIndex, lo.BlockTime, b+1, hi.Source, hi.BlockIndex, hi.BlockTime)
			}
			if len(r.cands) == 0 {
				report("entries %d..%d: no candidate (%d stream items between their neighbours)", a, b, len(between))
				ok = false
				break
			}
			runs = append(runs, r)
		}
		if !ok {
			continue
		}

		// Where the chain has an index chain, choose each block's candidates
		// by the chain's anchor at the block's end, recorded in the root
		// chain, before the set as a whole is proven
		if len(runs) > 0 && sparse && *flagRootTo == 0 {
			// One entry per block? Known entries carried by an anchor give
			// entry -> block pairs; they must advance together
			blockOf := map[string]uint64{}
			for _, n := range minors {
				blockOf[string(anchors[n].StateTreeAnchor[:])] = n
			}
			type pair struct{ idx, blk uint64 }
			var pairs []pair
			for x, e := range entries {
				if e == nil {
					continue
				}
				if b, ok := blockOf[string(e)]; ok {
					pairs = append(pairs, pair{first + uint64(x), b})
				}
			}
			// A lost entry's block lies between the carried entries around
			// it, at least one block per entry apart; each anchor carried in
			// that span is a candidate
			total := 1
			failed := false
			for x := range runs {
				i := runs[x].a
				var below, above *pair
				for p := range pairs {
					if pairs[p].idx < i && (below == nil || pairs[p].idx > below.idx) {
						below = &pairs[p]
					}
					if pairs[p].idx > i && (above == nil || pairs[p].idx < above.idx) {
						above = &pairs[p]
					}
				}
				if below == nil || above == nil {
					report("entry %d: no carried entry on both sides", i)
					failed = true
					break
				}
				lo, hi := below.blk+(i-below.idx), above.blk-(above.idx-i)
				var cands [][][]byte
				for _, n := range minors {
					if n >= lo && n <= hi {
						cands = append(cands, [][]byte{anchors[n].StateTreeAnchor[:]})
					}
				}
				if len(cands) == 0 {
					report("entry %d: no anchor in blocks %d..%d", i, lo, hi)
					failed = true
					break
				}
				runs[x].cands = cands
				total *= len(cands)
			}
			if failed {
				continue
			}
			report("%d combinations to prove", total)
		}
		if len(runs) > 0 && sparse && *flagRootTo > 0 {
			// No index chain: each lost entry is the candidate whose anchor
			// is a root chain entry between where the anchors of the known
			// entries on either side went
			if rootAt == nil {
				rootAt = map[[32]byte]uint64{}
				root := batch.Account(protocol.DnUrl().JoinPath(protocol.Ledger)).RootChain()
				for r := *flagRootFrom; r <= *flagRootTo; r++ {
					if e, err := root.Entry(int64(r)); err == nil && len(e) == 32 {
						rootAt[[32]byte(e)] = r
					}
				}
				log.Printf("%d root chain entries loaded", len(rootAt))
			}
			if *flagDebug {
				st := prev.Copy()
				var in, total int
				for _, e := range entries {
					if e == nil {
						break
					}
					st.AddEntry(e)
					total++
					if _, ok := rootAt[[32]byte(st.Anchor())]; ok {
						in++
					}
				}
				inStream := 0
				for _, e := range entries {
					if e != nil {
						if _, ok := pos[string(e)]; ok {
							inStream++
						}
					}
				}
				fmt.Printf("DEBUG anchors after known entries recorded in the root chain: %d of %d; known entries carried by an anchor: %d\n", in, total, inStream)
			}
			failed := false
			state := prev.Copy()
			for idx := uint64(0); idx < uint64(len(entries)); idx++ {
				if entries[idx] != nil {
					state.AddEntry(entries[idx])
					continue
				}
				var x int
				for x = range runs {
					if runs[x].a == first+idx {
						break
					}
				}
				var hits [][]byte
				for _, c := range runs[x].cands {
					t := &merkle.State{Count: state.Count, Pending: append([][]byte{}, state.Pending...)}
					t.AddEntry(c[0])
					if _, ok := rootAt[[32]byte(t.Anchor())]; ok {
						hits = append(hits, c[0])
					}
				}
				if len(hits) != 1 {
					report("entry %d: %d candidates give a recorded anchor", first+idx, len(hits))
					failed = true
					break
				}
				entries[idx] = hits[0]
				runs[x].cands = [][][]byte{{hits[0]}}
				state.AddEntry(hits[0])
			}
			if failed {
				continue
			}
		}
		if len(runs) > 0 && name == "signature" {
			if err := chooseByBlock(batch, ch, prev, first, entries, func(a uint64) (int, [][][]byte) {
				for x, r := range runs {
					if r.a == a {
						return x, r.cands
					}
				}
				return -1, nil
			}, func(x int, c [][]byte) { runs[x].cands = [][][]byte{c} }); err != nil {
				report("by block: %v", err)
				continue
			}
		}

		// Prove it: this set, then the next, must give the next mark point.
		// The combination of the runs' candidates is searched.
		var tries int
		var mark *merkle.State
		choice := make([]int, len(runs))
		var search func(r int) bool
		search = func(r int) bool {
			if r == len(runs) {
				tries++
				for x, run := range runs {
					for j, e := range run.cands[choice[x]] {
						entries[run.a-first+uint64(j)] = e
					}
				}
				state := prev.Copy()
				for _, e := range entries {
					state.AddEntry(e)
				}
				m := state.Copy()
				for _, e := range next.HashList {
					state.AddEntry(e)
				}
				if state.Count == next.Count && bytes.Equal(state.Anchor(), next.Anchor()) {
					mark = m
					return true
				}
				return false
			}
			for c := range runs[r].cands {
				if tries > 5_000_000 {
					return false
				}
				choice[r] = c
				if search(r + 1) {
					return true
				}
			}
			return false
		}
		if !search(0) {
			report("not proven after %d combinations", tries)
			continue
		}

		// The mark point's encoding, proven on the next one: rebuilt from
		// its predecessor, it must be the stored bytes
		nextStored, has := raw(m.Key().Append("States", last+freq).Hash())
		nextBytes, err := next.MarshalBinary()
		if !has || err != nil || !bytes.Equal(nextStored, nextBytes) {
			report("proven, but the mark point encoding is not: not written")
			continue
		}
		mark.HashList = entries
		markBytes, err := mark.MarshalBinary()
		if err != nil {
			return err
		}
		kh := m.Key().Append("States", last).Hash()
		if err := dst.Put(kh[:], markBytes, nil); err != nil {
			return err
		}
		for _, i := range missing {
			kh := m.Key().Append("Element", i).Hash()
			if err := dst.Put(kh[:], encodeElement(entries[i-first]), nil); err != nil {
				return err
			}
		}
		report("proven: %d lost entries and the mark point rebuilt", len(missing))
	}
	return nil
}

// indexStream returns, in root chain order, the index entries an account
// chain's index chain records: for each root chain entry that is the chain's
// anchor after one of its entries, {Source: that entry, Anchor: the root
// index}. The chain's entries around the span come from the store.
func indexStream(batch *coredb.Batch, acct *url.URL, name string, before, after, suffix []byte) ([][]byte, error) {
	var lo, hi protocol.IndexEntry
	if err := lo.UnmarshalBinary(before); err != nil {
		return nil, err
	}
	if err := hi.UnmarshalBinary(after); err != nil {
		return nil, err
	}
	ch, err := batch.Account(acct).ChainByName(name)
	if err != nil {
		return nil, err
	}
	m := ch.Inner()
	freq := uint64(m.MarkFreq())

	// The chain's anchor after each entry from lo to hi
	start := lo.Source / freq * freq
	state := new(merkle.State)
	if start > 0 {
		s, err := m.States(start - 1).Get()
		if err != nil {
			return nil, err
		}
		state = s.Copy()
	}
	anchorAt := map[[32]byte]uint64{}
	for i := start; i <= hi.Source; i++ {
		e, err := ch.Entry(int64(i))
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		state.AddEntry(e)
		if i >= lo.Source {
			anchorAt[[32]byte(state.Anchor())] = i
		}
	}

	root := batch.Account(protocol.DnUrl().JoinPath(protocol.Ledger)).RootChain()
	rootIndex := root.Index()
	rhead, err := rootIndex.Head().Get()
	if err != nil {
		return nil, err
	}
	// The block of a root index: the first root index entry at or past it
	blockOf := func(r uint64) (uint64, error) {
		lo, hi := int64(0), rhead.Count-1
		for lo < hi {
			mid := (lo + hi) / 2
			var e protocol.IndexEntry
			if err := rootIndex.EntryAs(mid, &e); err != nil {
				return 0, err
			}
			if e.Source < r {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		var e protocol.IndexEntry
		if err := rootIndex.EntryAs(lo, &e); err != nil {
			return 0, err
		}
		return e.BlockIndex, nil
	}
	var out [][]byte
	for r := lo.Anchor; r <= hi.Anchor; r++ {
		e, err := root.Entry(int64(r))
		if err != nil {
			return nil, fmt.Errorf("root entry %d: %w", r, err)
		}
		if len(e) != 32 {
			continue
		}
		if i, ok := anchorAt[[32]byte(e)]; ok {
			blk, err := blockOf(r)
			if err != nil {
				return nil, err
			}
			b, err := (&protocol.IndexEntry{Source: i, Anchor: r, BlockIndex: blk}).MarshalBinary()
			if err != nil {
				return nil, err
			}
			out = append(out, append(b, suffix...))
		}
	}
	return out, nil
}

// headerTime returns a CometBFT block's header time.
func headerTime(store *leveldb.DB, height uint64) (time.Time, bool) {
	meta, err := store.Get([]byte(fmt.Sprintf("H:%d", height)), nil)
	if err != nil {
		return time.Time{}, false
	}
	header, ok := field(meta, 3)
	if !ok {
		return time.Time{}, false
	}
	ts, ok := field(header, 4)
	if !ok {
		return time.Time{}, false
	}
	var sec, nsec int64
	for b := ts; len(b) > 0; {
		num, _, n := protowire.ConsumeTag(b)
		if n < 0 {
			return time.Time{}, false
		}
		v, m := protowire.ConsumeVarint(b[n:])
		if m < 0 {
			return time.Time{}, false
		}
		b = b[n+m:]
		switch num {
		case 1:
			sec = int64(v)
		case 2:
			nsec = int64(v)
		}
	}
	return time.Unix(sec, nsec).UTC(), true
}

// learnSuffix returns the bytes stored index entries carry past their fields,
// if every decodable entry carries the same.
func learnSuffix(entries [][]byte) ([]byte, bool) {
	var suffix []byte
	seen := false
	for _, e := range entries {
		var ie protocol.IndexEntry
		if e == nil || ie.UnmarshalBinary(e) != nil {
			continue
		}
		// A decoded entry keeps what it did not decode and writes it back;
		// a fresh one from its fields does not
		fresh := &protocol.IndexEntry{Source: ie.Source, Anchor: ie.Anchor, BlockIndex: ie.BlockIndex, BlockTime: ie.BlockTime, RootIndexIndex: ie.RootIndexIndex}
		b, err := fresh.MarshalBinary()
		if err != nil || !bytes.HasPrefix(e, b) {
			return nil, false
		}
		if !seen {
			suffix, seen = e[len(b):], true
		} else if !bytes.Equal(suffix, e[len(b):]) {
			return nil, false
		}
	}
	return suffix, seen
}

// chooseByBlock narrows each run's candidates to the one that gives the
// chain's recorded anchor at the end of the block the run is in. The chain's
// index chain gives each block's last entry and where in the root chain its
// anchor went.
func chooseByBlock(batch *coredb.Batch, ch *coredb.Chain2, prev *merkle.State, first uint64, entries [][]byte, runAt func(uint64) (int, [][][]byte), choose func(int, [][]byte)) error {
	index := ch.Index()
	ihead, err := index.Head().Get()
	if err != nil {
		return err
	}
	root := batch.Account(protocol.DnUrl().JoinPath(protocol.Ledger)).RootChain()
	last := first + uint64(len(entries)) - 1

	// Block ends within the set: index entries with Source in [first, last+]
	lo, hi := int64(0), ihead.Count-1
	for lo < hi {
		mid := (lo + hi) / 2
		var e protocol.IndexEntry
		if err := index.EntryAs(mid, &e); err != nil {
			// An unreadable entry: step past it
			lo = mid + 1
			continue
		}
		if e.Source < first {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	type end struct{ source, anchor uint64 }
	var ends []end
	for k := lo; k < ihead.Count; k++ {
		var e protocol.IndexEntry
		if err := index.EntryAs(k, &e); err != nil {
			continue // lost; the next block end covers it
		}
		ends = append(ends, end{e.Source, e.Anchor})
		if e.Source >= last {
			break
		}
	}

	state := prev.Copy()
	i := first
	for _, en := range ends {
		// The runs in this block
		type pending struct {
			x     int
			a     uint64
			cands [][][]byte
		}
		var runs []pending
		for j := i; j <= en.source && j <= last; j++ {
			if entries[j-first] == nil {
				if x, c := runAt(j); x >= 0 {
					runs = append(runs, pending{x, j, c})
				}
			}
		}
		want, err := root.Entry(int64(en.anchor))
		if err != nil {
			return fmt.Errorf("root entry %d: %w", en.anchor, err)
		}
		// The block's entries past the set are the next set's, known
		upTo := en.source
		if upTo > last {
			upTo = last
		}
		choice := make([]int, len(runs))
		var found bool
		var try func(r int)
		try = func(r int) {
			if found {
				return
			}
			if r == len(runs) {
				for y, p := range runs {
					for z, e := range p.cands[choice[y]] {
						entries[p.a-first+uint64(z)] = e
					}
				}
				s := state.Copy()
				for j := i; j <= upTo; j++ {
					s.AddEntry(entries[j-first])
				}
				if en.source <= last && bytes.Equal(s.Anchor(), want) {
					found = true
				}
				if en.source > last {
					found = true // proven with the set as a whole
				}
				return
			}
			for c := range runs[r].cands {
				choice[r] = c
				try(r + 1)
				if found {
					return
				}
			}
		}
		try(0)
		if !found {
			return fmt.Errorf("no candidate gives the recorded anchor at block end %d", en.source)
		}
		for y, p := range runs {
			choose(p.x, p.cands[choice[y]])
		}
		for j := i; j <= upTo; j++ {
			state.AddEntry(entries[j-first])
		}
		i = upTo + 1
		if i > last {
			break
		}
	}
	return nil
}
