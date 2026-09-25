// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	badger "github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// rebuildChains rebuilds chains from the CometBFT blocks, block by block, over
// the heights from -from to the end of the block store, and proves every block
// of every chain as it goes.
//
// A block ledger lists, in order, the chain entries its block appended: which
// account, which chain, which index. That gives each entry its block and its
// place. The block's messages give the candidate hashes. And the ledger also
// lists the root chain entries the block wrote, which are the anchors of every
// chain the block changed — so a chain rebuilt through a block is right exactly
// when its anchor is one of them. Where a block's candidates do not fit in
// order, other selections from them, and from the unused candidates of recent
// blocks (a transaction that waited for signatures), are tried against the
// anchor.
//
// Each chain starts from its state before its first entry in range, taken from
// the archive; everything after that comes from blocks. A chain stops at its
// first block that cannot be proven, and the reason is counted.
func rebuildChains(archiveArg, blockstore string) error {
	name, path, _ := strings.Cut(archiveArg, "=")
	db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
	if err != nil {
		return fmt.Errorf("archive %s: %w", name, err)
	}
	a := &archive{name: name, db: db, log: &vlog{dir: path}}
	store, err := leveldb.OpenFile(blockstore, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return err
	}
	defer store.Close()
	lu, err := url.Parse(*flagLedgerURL)
	if err != nil {
		return fmt.Errorf("-ledger-url: %w", err)
	}
	base, height := storeRange(store)
	from := base
	if *flagFrom > from {
		from = *flagFrom
	}
	if *flagTo > 0 && *flagTo < height {
		height = *flagTo
	}

	// A batch caches what it reads, so it is replaced every so often; chains
	// are looked up in the current one
	batch := coredb.New(&verifiedStore{a}, nil).Begin(false)
	defer func() { batch.Discard() }()
	root := batch.Account(lu).RootChain()

	// The root chain entries each block wrote, from the root index chain,
	// walked back from its head to the start of the range
	rootIndex := root.Index()
	rhead, err := rootIndex.Head().Get()
	if err != nil {
		return fmt.Errorf("root index chain: %w", err)
	}
	type rootSpan struct{ first, last uint64 }
	rootSpans := map[uint64]rootSpan{}
	var after *protocol.IndexEntry
	for k := rhead.Count - 1; k >= 0; k-- {
		ie := new(protocol.IndexEntry)
		if err := rootIndex.EntryAs(k, ie); err != nil {
			return fmt.Errorf("root index entry %d: %w", k, err)
		}
		if after != nil {
			rootSpans[after.BlockIndex] = rootSpan{ie.Source + 1, after.Source}
		}
		after = ie
		if ie.BlockIndex < from {
			break
		}
	}
	log.Printf("root index: %d blocks in range", len(rootSpans))

	type chainState struct {
		account *url.URL
		ok      bool
		kind    string
		state   *merkle.State
		next    uint64 // the next index to rebuild
		used    map[[32]byte]bool
		recent  [][32]byte // used, oldest first; bounds used
	}
	type tally struct {
		entries, rebuilt, equal, blocks, provenBlocks int
		gaps                                          map[string]int
	}
	chains := map[string]*chainState{}
	tallies := map[string]*tally{}
	tallyOf := func(kind string) *tally {
		t := tallies[kind]
		if t == nil {
			t = &tally{gaps: map[string]int{}}
			tallies[kind] = t
		}
		return t
	}
	var noStart int
	var debugDA int

	// What an entry the blocks did not give is, from the archive's own copy
	typeOf := func(h []byte) string {
		msg, err := batch.Message([32]byte(h)).Main().Get()
		if err != nil {
			return "(no message record)"
		}
		typ := msg.Type().String()
		switch m := msg.(type) {
		case *messaging.TransactionMessage:
			typ += "/" + m.Transaction.Body.Type().String()
		case *messaging.SignatureMessage:
			if m.Signature != nil {
				typ += "/" + m.Signature.Type().String()
			}
		}
		return typ
	}

	// The messages of the last -window blocks, oldest first
	var ring []*blockIndex

	use := func(c *chainState, h []byte) {
		k := [32]byte(h)
		c.used[k] = true
		c.recent = append(c.recent, k)
		if len(c.recent) > 4096 {
			delete(c.used, c.recent[0])
			c.recent = c.recent[1:]
		}
	}
	chainOf := func(c *chainState) *coredb.Chain2 {
		ch, _ := batch.Account(c.account).ChainByName(c.kind)
		return ch
	}
	// The window is filled from the blocks before the range, so a message
	// delivered just before it can still be matched
	first := from
	if first >= base+uint64(*flagWindow) {
		first -= uint64(*flagWindow)
	} else {
		first = base
	}
	for n := first; n <= height; n++ {
		if (n-first)%2000 == 1999 {
			batch.Discard()
			batch = coredb.New(&verifiedStore{a}, nil).Begin(false)
			root = batch.Account(lu).RootChain()
		}
		if (n-from)%100000 == 0 {
			log.Printf("block %d of %d..%d, %d chains", n, from, height, len(chains))
		}
		txs, inStore, err := cometBlock(store, n)
		if err != nil {
			return err
		}
		if bi := indexBlock(n, decodeBlock(txs)); bi != nil {
			ring = append(ring, bi)
		}
		for len(ring) > 0 && ring[0].height+uint64(*flagWindow) < n {
			ring = ring[1:]
		}
		if n < from {
			continue
		}

		var bl *protocol.BlockLedger
		if batch.Account(lu.JoinPath(fmt.Sprint(n))).Main().GetAs(&bl) != nil {
			continue
		}

		// The anchors the block wrote, and what it appended to each chain
		anchors := map[[32]byte]bool{}
		if rs, ok := rootSpans[n]; ok {
			for i := rs.first; i <= rs.last; i++ {
				if h, err := root.Entry(int64(i)); err == nil && len(h) == 32 {
					anchors[[32]byte(h)] = true
				}
			}
		}
		touched := map[string][]uint64{}
		var order []string
		for _, e := range bl.Entries {
			if e.Chain != "main" && e.Chain != "signature" && e.Chain != "scratch" {
				continue
			}
			k := e.Account.String() + "#" + e.Chain
			if _, ok := touched[k]; !ok {
				order = append(order, k)
			}
			touched[k] = append(touched[k], e.Index)
		}

		for _, k := range order {
			indices := touched[k]
			c := chains[k]
			if c == nil {
				acct, kind, _ := strings.Cut(k, "#")
				u, _ := url.Parse(acct)
				ch, err := batch.Account(u).ChainByName(kind)
				if err != nil {
					noStart++
					chains[k] = &chainState{}
					continue
				}
				c = &chainState{account: u, ok: true, kind: kind, used: map[[32]byte]bool{}}
				chains[k] = c
				// The state before the chain's first entry in range, from
				// the archive: the mark point below it, and the entries since
				c.next = indices[0]
				freq := uint64(ch.Inner().MarkFreq())
				mark := c.next / freq * freq
				c.state = new(merkle.State)
				if mark > 0 {
					s, err := ch.Inner().States(mark - 1).Get()
					if err != nil {
						noStart++
						c.ok = false
						continue
					}
					c.state = s.Copy()
				}
				for i := mark; i < c.next && c.ok; i++ {
					h, err := ch.Entry(int64(i))
					if err != nil {
						noStart++
						c.ok = false
						break
					}
					c.state.AddEntry(h)
				}
			}
			if !c.ok {
				continue
			}
			ch := chainOf(c)
			t := tallyOf(c.kind)
			t.entries += len(indices)
			t.blocks++

			// Entries appended where the ledger did not show them are taken
			// from the archive; they are not the blocks' to give
			for c.next < indices[0] {
				h, err := ch.Entry(int64(c.next))
				if err != nil {
					break
				}
				c.state.AddEntry(h)
				c.next++
			}

			var pool [][]byte
			if inStore {
				key := c.account.String()
				for _, d := range ring {
					var hs [][]byte
					if c.kind == "signature" {
						hs = append(d.bySigner[key], d.blockAnchors...)
					} else {
						hs = d.byPrincipal[key]
					}
					for _, h := range hs {
						if !c.used[[32]byte(h)] {
							pool = append(pool, h)
						}
					}
				}
			}
			pool = dedupe(pool)
			pick := fit(c.state, pool, len(indices), anchors)
			if pick != nil {
				t.provenBlocks++
				for j, i := range pick {
					c.state.AddEntry(pool[i])
					use(c, pool[i])
					t.rebuilt++
					if want, err := ch.Entry(int64(indices[j])); err == nil && bytes.Equal(want, pool[i]) {
						t.equal++
					}
				}
			} else {
				// A gap: say what the blocks did not give, then carry on from
				// the archive's state after the block
				for _, idx := range indices {
					h, err := ch.Entry(int64(idx))
					if err != nil {
						t.gaps["(archive unreadable)"]++
						continue
					}
					inPool := false
					for _, p := range pool {
						inPool = inPool || bytes.Equal(p, h)
					}
					reason := typeOf(h)
					if reason == "transaction/directoryAnchor" && debugDA < 2 {
						debugDA++
						msg, _ := batch.Message([32]byte(h)).Main().Get()
						tm := msg.(*messaging.TransactionMessage)
						ab := tm.Transaction.Body.(protocol.AnchorBody).GetPartitionAnchor()
						rec, _ := json.Marshal(tm.Transaction)
						fmt.Printf("DEBUG recorded %x %.600s\n", h[:4], rec)
						for _, d := range ring {
							for _, m := range d.anchors {
								if m.Body.(protocol.AnchorBody).GetPartitionAnchor().MinorBlockIndex == ab.MinorBlockIndex {
									del, _ := json.Marshal(m)
									fmt.Printf("DEBUG delivered in %d %x %.600s\n", d.height, m.GetHash()[:4], del)
								}
							}
						}
					}
					if !inStore {
						reason = "block not in the store"
					} else if inPool {
						reason += " (in the blocks, selection not found)"
					}
					t.gaps[reason]++
					c.state.AddEntry(h)
					use(c, h)
				}
			}
			c.next = indices[len(indices)-1] + 1
		}
	}

	fmt.Printf("%s: blocks %d..%d, window %d blocks, %d chains (%d without a starting state)\n", name, from, height, *flagWindow, len(chains), noStart)
	fmt.Printf("%-10s %9s %9s %9s %11s\n", "chain", "entries", "rebuilt", "= archive", "blocks proven")
	var kinds []string
	for k := range tallies {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		t := tallies[k]
		fmt.Printf("%-10s %9d %9d %9d %6d of %d\n", k, t.entries, t.rebuilt, t.equal, t.provenBlocks, t.blocks)
		var reasons []string
		for r := range t.gaps {
			reasons = append(reasons, r)
		}
		sort.Slice(reasons, func(i, j int) bool { return t.gaps[reasons[i]] > t.gaps[reasons[j]] })
		for _, r := range reasons {
			fmt.Printf("    not rebuilt: %7d %s\n", t.gaps[r], r)
		}
	}
	return nil
}

func dedupe(l [][]byte) [][]byte {
	seen := map[[32]byte]bool{}
	var out [][]byte
	for _, h := range l {
		if !seen[[32]byte(h)] {
			seen[[32]byte(h)] = true
			out = append(out, h)
		}
	}
	return out
}

// decodeBlock returns a block's messages.
func decodeBlock(txs [][]byte) []messaging.Message {
	var out []messaging.Message
	for _, tx := range txs {
		env := new(messaging.Envelope)
		if env.UnmarshalBinary(tx) != nil {
			continue
		}
		msgs, err := env.Normalize()
		if err != nil {
			continue
		}
		out = append(out, msgs...)
	}
	return out
}

// fit returns the positions in pool of count entries which, appended to state
// in order, give an anchor the block wrote. The newest candidates in order are
// tried first, then other in-order selections, within a bound.
func fit(state *merkle.State, pool [][]byte, count int, anchors map[[32]byte]bool) []int {
	if count > len(pool) {
		return nil
	}
	proves := func(pick []int) bool {
		// The anchor depends only on the count and the peaks, so a trial
		// leaves the hash list behind
		s := &merkle.State{Count: state.Count, Pending: append([][]byte{}, state.Pending...)}
		for _, i := range pick {
			s.AddEntry(pool[i])
		}
		a := s.Anchor()
		return len(a) == 32 && anchors[[32]byte(a)]
	}
	pick := make([]int, count)
	for j := range pick {
		pick[j] = len(pool) - count + j
	}
	if proves(pick) {
		return pick
	}
	for j := range pick {
		pick[j] = j
	}
	if proves(pick) {
		return pick
	}
	tries := 0
	var search func(start, j int) bool
	search = func(start, j int) bool {
		if j == count {
			tries++
			return proves(pick)
		}
		for i := start; i <= len(pool)-(count-j) && tries < 50000; i++ {
			pick[j] = i
			if search(i+1, j+1) {
				return true
			}
		}
		return false
	}
	if search(0, 0) {
		return pick
	}
	return nil
}

// blockIndex is a block's candidate chain entries, by account: transactions
// by principal (main, scratch), signatures by signer and the validators' block
// anchors (signature).
type blockIndex struct {
	height       uint64
	anchors      []*protocol.Transaction
	byPrincipal  map[string][][]byte
	bySigner     map[string][][]byte
	blockAnchors [][]byte
}

func indexBlock(height uint64, msgs []messaging.Message) *blockIndex {
	if len(msgs) == 0 {
		return nil
	}
	bi := &blockIndex{height: height, byPrincipal: map[string][][]byte{}, bySigner: map[string][][]byte{}}
	var visit func(messaging.Message)
	visit = func(m messaging.Message) {
		switch m := m.(type) {
		case *messaging.TransactionMessage:
			if p := m.Transaction.Header.Principal; p != nil {
				bi.byPrincipal[p.String()] = append(bi.byPrincipal[p.String()], m.Transaction.GetHash())
				// An anchor may be recorded as produced, without a header
				if _, ok := m.Transaction.Body.(protocol.AnchorBody); ok {
					bi.anchors = append(bi.anchors, m.Transaction)
					bare := &protocol.Transaction{Body: m.Transaction.Body}
					bi.byPrincipal[p.String()] = append(bi.byPrincipal[p.String()], bare.GetHash())
				}
			}
		case *messaging.SignatureMessage:
			if m.Signature != nil {
				h := m.Hash()
				s := m.Signature.GetSigner().String()
				bi.bySigner[s] = append(bi.bySigner[s], h[:])
			}
		case *messaging.SyntheticMessage:
			visit(m.Message)
		case *messaging.BadSyntheticMessage:
			visit(m.Message)
		case *messaging.SequencedMessage:
			visit(m.Message)
		case *messaging.BlockAnchor:
			h := m.Hash()
			bi.blockAnchors = append(bi.blockAnchors, h[:])
			visit(m.Anchor)
		}
	}
	for _, m := range msgs {
		visit(m)
	}
	return bi
}
