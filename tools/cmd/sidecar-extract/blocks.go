// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	badger "github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"google.golang.org/protobuf/encoding/protowire"
)

// A pre-reorg partition's block ledger (ledger/<N>) lists, in order, every
// chain entry its block N appended; its CometBFT block N holds the messages
// submitted in that block. Where an entry's hash is one of those messages, the
// chain can be rebuilt from blocks. blockSurvey measures how often that holds,
// by chain, over a sample of block ledgers whose entries the archive still has.

// cometBlock returns the transactions of a CometBFT block, read from the block
// store's parts. ok is false when the store does not hold the height.
func cometBlock(store *leveldb.DB, height uint64) (txs [][]byte, ok bool, err error) {
	var block []byte
	for i := 0; ; i++ {
		part, err := store.Get([]byte(fmt.Sprintf("P:%d:%d", height, i)), nil)
		if err == leveldb.ErrNotFound {
			if i == 0 {
				return nil, false, nil
			}
			break
		}
		if err != nil {
			return nil, false, err
		}
		b, ok := field(part, 2) // Part.bytes
		if !ok {
			return nil, false, fmt.Errorf("height %d part %d has no bytes", height, i)
		}
		block = append(block, b...)
	}
	data, _ := field(block, 2) // Block.data
	for b := data; len(b) > 0; {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, false, fmt.Errorf("height %d: bad data", height)
		}
		b = b[n:]
		if num == 1 && typ == protowire.BytesType { // Data.txs
			v, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return nil, false, fmt.Errorf("height %d: bad tx", height)
			}
			txs = append(txs, v)
			b = b[m:]
			continue
		}
		m := protowire.ConsumeFieldValue(num, typ, b)
		if m < 0 {
			return nil, false, fmt.Errorf("height %d: bad field", height)
		}
		b = b[m:]
	}
	return txs, true, nil
}

// field returns the first length-delimited field num of a protobuf message.
func field(b []byte, num protowire.Number) ([]byte, bool) {
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		if l < 0 {
			return nil, false
		}
		b = b[l:]
		if n == num && typ == protowire.BytesType {
			v, m := protowire.ConsumeBytes(b)
			return v, m >= 0
		}
		m := protowire.ConsumeFieldValue(n, typ, b)
		if m < 0 {
			return nil, false
		}
		b = b[m:]
	}
	return nil, false
}

// messageHashes returns every hash a block's transactions carry: each message,
// the messages inside wrappers, transactions and signatures.
func messageHashes(txs [][]byte) (map[[32]byte]bool, int) {
	hashes := map[[32]byte]bool{}
	var bad int
	var add func(messaging.Message)
	add = func(m messaging.Message) {
		if m == nil {
			return
		}
		hashes[m.Hash()] = true
		switch m := m.(type) {
		case *messaging.TransactionMessage:
			hashes[*(*[32]byte)(m.Transaction.GetHash())] = true
			// An anchor's anchor chain entries are the roots it carries,
			// its own and those it forwards
			if body, ok := m.Transaction.Body.(protocol.AnchorBody); ok {
				a := body.GetPartitionAnchor()
				hashes[a.RootChainAnchor] = true
				hashes[a.StateTreeAnchor] = true

				// The sender's anchor-sequence chain records the anchor as
				// it was produced, before it was addressed: no header
				bare := &protocol.Transaction{Body: m.Transaction.Body}
				hashes[*(*[32]byte)(bare.GetHash())] = true
			}
			if body, ok := m.Transaction.Body.(*protocol.DirectoryAnchor); ok {
				for _, r := range body.Receipts {
					if r.Anchor != nil {
						hashes[r.Anchor.RootChainAnchor] = true
						hashes[r.Anchor.StateTreeAnchor] = true
					}
				}
			}
		case *messaging.SignatureMessage:
			if m.Signature != nil {
				hashes[*(*[32]byte)(m.Signature.Hash())] = true
			}
		case *messaging.SyntheticMessage:
			add(m.Message)
		case *messaging.BadSyntheticMessage:
			add(m.Message)
		case *messaging.SequencedMessage:
			add(m.Message)
		case *messaging.BlockAnchor:
			add(m.Anchor)
		}
	}
	for _, tx := range txs {
		env := new(messaging.Envelope)
		if err := env.UnmarshalBinary(tx); err != nil {
			bad++
			continue
		}
		msgs, err := env.Normalize()
		if err != nil {
			bad++
			continue
		}
		for _, m := range msgs {
			add(m)
		}
	}
	return hashes, bad
}

// blockTime returns a CometBFT block's header time, in Unix seconds.
func blockTime(store *leveldb.DB, height uint64) (int64, bool) {
	meta, err := store.Get([]byte(fmt.Sprintf("H:%d", height)), nil)
	if err != nil {
		return 0, false
	}
	header, ok := field(meta, 3) // BlockMeta.header
	if !ok {
		return 0, false
	}
	ts, ok := field(header, 4) // Header.time
	if !ok {
		return 0, false
	}
	num, _, n := protowire.ConsumeTag(ts)
	if n < 0 || num != 1 {
		return 0, true // zero seconds
	}
	v, m := protowire.ConsumeVarint(ts[n:])
	if m < 0 {
		return 0, false
	}
	return int64(v), true
}

// heightAt returns the first height of a block store at or after a time.
func heightAt(store *leveldb.DB, t int64) uint64 {
	lo, hi := storeRange(store)
	for lo < hi {
		mid := (lo + hi) / 2
		if bt, ok := blockTime(store, mid); ok && bt < t {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func storeRange(store *leveldb.DB) (base, height uint64) {
	v, _ := store.Get([]byte("blockStore"), nil)
	for len(v) > 0 {
		num, _, n := protowire.ConsumeTag(v)
		if n < 0 {
			break
		}
		x, m := protowire.ConsumeVarint(v[n:])
		if m < 0 {
			break
		}
		v = v[n+m:]
		switch num {
		case 1:
			base = x
		case 2:
			height = x
		}
	}
	return
}

var chainKind = regexp.MustCompile(`\(.*?\)`)

// blockSurvey samples block ledgers from an account list and reports, per
// chain kind, how many of the entries they list are messages of their block.
func blockSurvey(archiveArg, blockstore, accounts string, sample int) error {
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
	// Where this partition's outgoing anchors land
	var dests []*leveldb.DB
	for _, p := range flagDest {
		d, err := leveldb.OpenFile(p, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
		if err != nil {
			return err
		}
		defer d.Close()
		dests = append(dests, d)
	}

	// Block ledgers, evenly spaced through the account list or, without one,
	// through the block store's range: the first block with a ledger at or
	// after each of sample evenly spaced heights
	var ledgers []string
	if accounts != "" {
		f, err := os.Open(accounts)
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if strings.Contains(sc.Text(), "/ledger/") {
				ledgers = append(ledgers, sc.Text())
			}
		}
		f.Close()
		sort.Strings(ledgers)
	}
	step := len(ledgers) / sample
	if step == 0 {
		step = 1
	}
	if accounts == "" {
		lu, err := url.Parse(*flagLedgerURL)
		if err != nil {
			return fmt.Errorf("-ledger-url: %w", err)
		}
		base, height := storeRange(store)
		b := coredb.New(&verifiedStore{a}, nil).Begin(false)
		for i := 0; i < sample; i++ {
			h := base + uint64(i)*(height-base)/uint64(sample)
			for j := uint64(0); j < 200 && h+j <= height; j++ {
				u := lu.JoinPath(fmt.Sprint(h + j))
				if _, err := b.Account(u).Main().Get(); err == nil {
					ledgers = append(ledgers, u.String())
					break
				}
			}
		}
		b.Discard()
		step = 1
	}

	type tally struct {
		missTypes              map[string]int
		entries, read, inBlock int
		misses                 []string
		searched, earlier      int
		dnSearched, dnFound    int
		distances              []uint64
	}
	// Where an entry is not in its block, look back through earlier blocks for
	// it: a transaction that waited for signatures executes after it arrived
	earlier := func(h [32]byte, height uint64) (uint64, bool, error) {
		for d := uint64(1); d <= uint64(*flagWindow) && d < height; d++ {
			txs, ok, err := cometBlock(store, height-d)
			if err != nil || !ok {
				return 0, false, err
			}
			if hs, _ := messageHashes(txs); hs[h] {
				return d, true, nil
			}
		}
		return 0, false, nil
	}
	byKind := map[string]*tally{}
	var blocks, noBlock, badTx, noLedger int
	batch := coredb.New(&verifiedStore{a}, nil).Begin(false)
	defer batch.Discard()
	for i := 0; i < len(ledgers); i += step {
		u, err := url.Parse(ledgers[i])
		if err != nil {
			return err
		}
		var bl *protocol.BlockLedger
		if err := batch.Account(u).Main().GetAs(&bl); err != nil {
			noLedger++
			continue
		}
		txs, ok, err := cometBlock(store, bl.Index)
		if err != nil {
			return err
		}
		if !ok {
			noBlock++
			continue
		}
		blocks++
		hashes, bad := messageHashes(txs)
		badTx += bad
		for _, e := range bl.Entries {
			kind := chainKind.ReplaceAllString(e.Chain, "()")
			t := byKind[kind]
			if t == nil {
				t = new(tally)
				byKind[kind] = t
			}
			t.entries++
			chain, err := batch.Account(e.Account).ChainByName(e.Chain)
			if err != nil {
				continue
			}
			h, err := chain.Entry(int64(e.Index))
			if err != nil || len(h) != 32 {
				continue
			}
			t.read++
			if hashes[[32]byte(h)] {
				t.inBlock++
				continue
			}
			// What the missing entry is, from the archive's own copy
			if t.missTypes == nil {
				t.missTypes = map[string]int{}
			}
			if msg, err := batch.Message([32]byte(h)).Main().Get(); err == nil {
				typ := msg.Type().String()
				if tx, ok := msg.(*messaging.TransactionMessage); ok {
					typ += "/" + tx.Transaction.Body.Type().String()
				}
				if sig, ok := msg.(*messaging.SignatureMessage); ok && sig.Signature != nil {
					typ += "/" + sig.Signature.Type().String()
				}
				t.missTypes[typ]++
			} else {
				t.missTypes["(no message record)"]++
			}
			if len(t.misses) < 4 {
				t.misses = append(t.misses, fmt.Sprintf("block %d %v %s[%d] %x", bl.Index, e.Account, e.Chain, e.Index, h[:8]))
			}
			if len(dests) > 0 && strings.HasPrefix(e.Chain, "anchor-sequence") && t.dnSearched < *flagSearch {
				// An outgoing anchor is a message of its destinations' blocks
				t.dnSearched++
			dests:
				for _, d := range dests {
					start := heightAt(d, bl.Time.Unix())
					for h2 := start; h2 < start+200; h2++ {
						txs, ok, err := cometBlock(d, h2)
						if err != nil {
							return err
						}
						if !ok {
							break
						}
						if hs, _ := messageHashes(txs); hs[[32]byte(h)] {
							t.dnFound++
							break dests
						}
					}
				}
			}
			if *flagWindow > 0 && t.searched < *flagSearch {
				t.searched++
				d, ok, err := earlier([32]byte(h), bl.Index)
				if err != nil {
					return err
				}
				if ok {
					t.earlier++
					t.distances = append(t.distances, d)
				}
			}
		}
	}

	fmt.Printf("%s: %d block ledgers sampled, %d blocks read, %d not in the block store, %d ledgers unreadable, %d txs undecodable\n",
		name, blocks+noBlock+noLedger, blocks, noBlock, noLedger, badTx)
	var kinds []string
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return byKind[kinds[i]].entries > byKind[kinds[j]].entries })
	fmt.Printf("%-40s %10s %10s %10s\n", "chain", "entries", "readable", "in block")
	for _, k := range kinds {
		t := byKind[k]
		fmt.Printf("%-40s %10d %10d %10d\n", k, t.entries, t.read, t.inBlock)
		for _, m := range t.misses {
			fmt.Printf("    miss: %s\n", m)
		}
		if t.dnSearched > 0 {
			fmt.Printf("    of %d misses searched in the destinations' blocks, %d found\n", t.dnSearched, t.dnFound)
		}
		if len(t.missTypes) > 0 {
			fmt.Printf("    missing entries by type: %v\n", t.missTypes)
		}
		if t.searched > 0 {
			sort.Slice(t.distances, func(i, j int) bool { return t.distances[i] < t.distances[j] })
			fmt.Printf("    of %d misses searched, %d found up to %d blocks earlier; distances %v\n", t.searched, t.earlier, *flagWindow, t.distances)
		}
	}
	return nil
}
