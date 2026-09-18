// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"context"
	"math/big"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// blockLedgerFacts is everything the block ledgers of a partition say, over
// a range of blocks: which accounts and chains each block named, and every
// chain-entry hash they refer to.
type blockLedgerFacts struct {
	// blocks that have a ledger record at all
	present map[uint64]bool
	// block -> "account;chain" it named
	named map[uint64][]string
	// every entry hash named by any block ledger's chains
	hashes map[[32]byte]uint64
}

func readBlockLedgers(t testing.TB, batch *database.Batch, ledgerURL *url.URL, from, to uint64) *blockLedgerFacts {
	t.Helper()
	f := &blockLedgerFacts{present: map[uint64]bool{}, named: map[uint64][]string{}, hashes: map[[32]byte]uint64{}}
	acct := batch.Account(ledgerURL)
	for i := from; i <= to; i++ {
		bl, err := acct.BlockLedger(i).Get()
		switch {
		case errors.Is(err, errors.NotFound):
			continue
		case err != nil:
			require.NoError(t, err)
		}
		if bl == nil || len(bl.Entries) == 0 {
			continue
		}
		f.present[i] = true
		for _, e := range bl.Entries {
			f.named[i] = append(f.named[i], strings.ToLower(e.Account.String())+";"+e.Chain)
			// and the entry hash the chain holds at that index
			chain, err := batch.Account(e.Account).ChainByName(e.Chain)
			if err != nil {
				continue
			}
			c, err := chain.Get()
			if err != nil {
				continue
			}
			h, err := c.Entry(int64(e.Index))
			if err != nil {
				continue
			}
			f.hashes[*(*[32]byte)(h)] = i
		}
	}
	return f
}

// heldOn is what one node's staging holds, by stream, as raw facts: the
// entry's sequence number, the hash a proof would prove, whether it was
// collected, and whether a validated hash stands at that number.
type heldFact struct {
	Stream    string
	Number    uint64
	Hash      [32]byte
	TxID      string
	Collected bool
	Validated bool
}

func heldOn(t testing.TB, s *execute.Staging) []heldFact {
	t.Helper()
	tx := s.Begin()
	defer tx.Discard()
	var out []heldFact
	for _, st := range tx.Streams() {
		for n := st.Delivered + 1; n <= st.Delivered+2048; n++ {
			h, ok := tx.IDOf(st.ID, n)
			if !ok {
				continue
			}
			_, v := tx.Validated(st.ID, n)
			out = append(out, heldFact{
				Stream:    st.ID.Ledger.String() + "|" + st.ID.Source.String(),
				Number:    n,
				Hash:      h.Hash,
				TxID:      h.ID.String(),
				Collected: h.Collected,
				Validated: v,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Stream != out[j].Stream {
			return out[i].Stream < out[j].Stream
		}
		return out[i].Number < out[j].Number
	})
	return out
}

// TestSyntheticIsNamedByTheBlockLedger establishes, against a running
// network, what the block ledger does and does not say about staging.
//
// Paul's proposal is that a restarting node rebuilds staging from the block
// ledger and the chains it names. This test asks the network the three
// questions that proposal turns on:
//
//  1. While a stage fills, does the block ledger name the synthetic account?
//  2. Do the held entries appear as chain entries named by any block ledger?
//  3. Can a reader tell a block that staged nothing from a block that staged
//     something it cannot find?
func TestSyntheticIsNamedByTheBlockLedger(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	// Hold the Directory's anchors back from BVN1 so what arrives from BVN0
	// is collected and held unproven: a stage with something in it.
	var hookMu sync.Mutex
	var hold atomic.Bool
	sim.S.SetNodeBlockHook("BVN1", func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		hookMu.Lock()
		defer hookMu.Unlock()
		if !hold.Load() {
			return envelopes, true
		}
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			isAnchor := false
			for _, m := range env.Messages {
				blk, ok := m.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				if seq, ok := blk.Anchor.(*messaging.SequencedMessage); ok {
					if txn, ok := seq.Message.(*messaging.TransactionMessage); ok && txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor {
						isAnchor = true
					}
				}
			}
			if isAnchor {
				continue
			}
			kept = append(kept, env)
		}
		return kept, true
	})

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// The block BVN1 stood at before anything was staged.
	before := sim.S.BlockIndex("BVN1")

	// Mixed pacing, deliberately: some blocks send one transaction to bob,
	// some send three. One synthetic per block cannot tell a record that
	// names a CHAIN once per block from one that names each ENTRY at its own
	// index — every index would be 0 and every count would agree. Blocks that
	// append several entries separate the two.
	hold.Store(true)
	var ts uint64
	for i := 0; i < 6; i++ {
		n := 1
		if i%2 == 1 {
			n = 3
		}
		for j := 0; j < n; j++ {
			ts++
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(bob, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		}
		sim.Step()
	}
	sim.StepN(30)

	after := sim.S.BlockIndex("BVN1")
	p := sim.S.Partition("BVN1")

	held := heldOn(t, p.NodeStaging(0))
	require.NotEmpty(t, held, "precondition: BVN1 node 0 holds unexecuted entries")

	batch := p.Begin(false)
	defer batch.Discard()
	ledgerURL := PartitionUrl("BVN1").JoinPath(Ledger)
	facts := readBlockLedgers(t, batch, ledgerURL, 1, after+2)

	synthAcct := strings.ToLower(PartitionUrl("BVN1").JoinPath(Synthetic).String())
	anchorAcct := strings.ToLower(PartitionUrl("BVN1").JoinPath(AnchorPool).String())

	t.Logf("BVN1 blocks %d..%d; %d block ledgers present", before, after, len(facts.present))
	t.Logf("staging holds %d entries:", len(held))
	for _, h := range held {
		t.Logf("  %s #%d collected=%v validated=%v hash=%x txid=%s",
			h.Stream, h.Number, h.Collected, h.Validated, h.Hash[:4], h.TxID)
	}

	// (1) Every held entry's hash is absent from every chain any block ledger
	//     names. Staging writes no chain, so the block ledger cannot name it.
	for _, h := range held {
		if blk, ok := facts.hashes[h.Hash]; ok {
			t.Errorf("held entry %s #%d (%x) IS named by block ledger %d", h.Stream, h.Number, h.Hash[:4], blk)
		}
	}

	// (2) During the window in which the stage filled, did any block ledger
	//     name the synthetic or anchor account at all?
	namedSynth, namedAnchor, blocksWithLedger := 0, 0, 0
	for i := before; i <= after; i++ {
		if !facts.present[i] {
			continue
		}
		blocksWithLedger++
		for _, n := range facts.named[i] {
			if strings.HasPrefix(n, synthAcct+";") {
				namedSynth++
			}
			if strings.HasPrefix(n, anchorAcct+";") {
				namedAnchor++
			}
		}
	}
	t.Logf("in blocks %d..%d: %d had a block ledger; synthetic named %d times, anchor pool named %d times",
		before, after, blocksWithLedger, namedSynth, namedAnchor)

	// (3) The distinguishability question. Collect, for each block in the
	//     window, the fingerprint a reconstructing node would see. If two
	//     blocks — one that staged something, one that staged nothing — have
	//     the same fingerprint, absence is ambiguous.
	fingerprints := map[string][]uint64{}
	for i := before; i <= after; i++ {
		key := "<no block ledger record>"
		if facts.present[i] {
			names := append([]string(nil), facts.named[i]...)
			sort.Strings(names)
			key = strings.Join(names, ",")
		}
		fingerprints[key] = append(fingerprints[key], i)
	}
	t.Logf("distinct block-ledger fingerprints over the window: %d", len(fingerprints))
	for k, v := range fingerprints {
		if len(k) > 160 {
			k = k[:160] + "..."
		}
		t.Logf("  %d blocks: %s", len(v), k)
	}

	// (4) And the synthetic ledger account itself — the one account Paul's
	//     proposal names — says nothing about the stage either. Received and
	//     Pending are fields of the record that the v2 executor no longer
	//     writes (stream_position.go flushStreams: "ONLY Delivered").
	var synth *SyntheticLedger
	require.NoError(t, batch.Account(PartitionUrl("BVN1").JoinPath(Synthetic)).Main().GetAs(&synth))
	part := synth.Partition(PartitionUrl("BVN0"))
	t.Logf("bvn1.acme/synthetic <- BVN0: delivered=%d received=%d pending=%d (staging holds %d)",
		part.Delivered, part.Received, len(part.Pending), len(held))
	require.Zero(t, part.Received, "Received is not maintained")
	require.Empty(t, part.Pending, "Pending is not maintained")
	require.Zero(t, part.Delivered, "nothing has been delivered on this stream")

	// (5) The PRODUCING side is on chain, and that half of Paul's proposal
	//     holds: BVN0's own block ledgers name its synthetic chain, and the
	//     hashes there are the hashes BVN1 is holding. What is missing is
	//     only the receiving node's record of which of them it had.
	p0 := sim.S.Partition("BVN0")
	b0 := p0.Begin(false)
	defer b0.Discard()
	f0 := readBlockLedgers(t, b0, PartitionUrl("BVN0").JoinPath(Ledger), 1, sim.S.BlockIndex("BVN0")+2)
	synth0 := strings.ToLower(PartitionUrl("BVN0").JoinPath(Synthetic).String())
	named0 := 0
	for _, names := range f0.named {
		for _, n := range names {
			if strings.HasPrefix(n, synth0+";") {
				named0++
			}
		}
	}
	matched := 0
	for _, h := range held {
		if _, ok := f0.hashes[h.Hash]; ok {
			matched++
		}
	}
	t.Logf("BVN0 block ledgers name its synthetic account %d times; %d/%d of BVN1's held hashes appear there",
		named0, matched, len(held))
	chains0 := map[string]int{}
	for _, names := range f0.named {
		for _, n := range names {
			chains0[n]++
		}
	}
	for n, c := range chains0 {
		t.Logf("  BVN0 chain named by %d block ledgers: %s", c, n)
	}

	// The producer's synthetic chain to BVN1 is named by the block ledgers,
	// and named the way every other chain is: one entry per APPENDED CHAIN
	// ENTRY, carrying that entry's own index (executor.md, "The block
	// ledger" — the list is (account, chain, index) triples, and every
	// consumer reads the chain AT that index). The block ledger is written
	// last, after anchorSynthChains has registered those entries.
	//
	// The ground truth for "which entries did block B append" is not the
	// block ledger — that is what is under test — but the synthetic chain's
	// own INDEX chain, which addChainAnchor writes per block with the last
	// entry index that block appended.
	sc := b0.Account(PartitionUrl("BVN0").JoinPath(Synthetic)).SyntheticChain("BVN1")
	head, err := sc.Inner().Head().Get()
	require.NoError(t, err)
	require.NotZero(t, head.Count, "the source did append to its synthetic chain")

	appended := map[uint64][]uint64{} // block -> entry indices it appended
	indexOfEntry := map[uint64]uint64{}
	ic, err := sc.Index().Get()
	require.NoError(t, err)
	require.NotZero(t, ic.Height(), "the synthetic chain is indexed per block")
	var next uint64
	for i := int64(0); i < ic.Height(); i++ {
		raw, err := ic.Entry(i)
		require.NoError(t, err)
		ie := new(IndexEntry)
		require.NoError(t, ie.UnmarshalBinary(raw))
		for x := next; x <= ie.Source; x++ {
			appended[ie.BlockIndex] = append(appended[ie.BlockIndex], x)
			indexOfEntry[x] = ie.BlockIndex
		}
		next = ie.Source + 1
	}
	require.Equal(t, int(head.Count), len(indexOfEntry),
		"the index chain accounts for every entry on the synthetic chain")

	// The workload must actually have put more than one synthetic for BVN1
	// in one block, or naming per chain and naming per entry are the same
	// assertion and this proves nothing.
	multi := 0
	for _, ix := range appended {
		if len(ix) > 1 {
			multi++
		}
	}
	require.NotZero(t, multi, "precondition: some block appended more than one entry to the synthetic chain")
	t.Logf("BVN0's synthetic chain to BVN1 (%s) has height %d over %d blocks, %d of which appended more than one entry",
		sc.Name(), head.Count, len(appended), multi)

	// What the block ledgers say: block -> the indices of that chain they name.
	named0map := map[uint64][]uint64{}
	acct0 := b0.Account(PartitionUrl("BVN0").JoinPath(Ledger))
	for i := uint64(1); i <= sim.S.BlockIndex("BVN0")+2; i++ {
		bl, err := acct0.BlockLedger(i).Get()
		switch {
		case errors.Is(err, errors.NotFound):
			continue
		case err != nil:
			require.NoError(t, err)
		}
		if bl == nil {
			continue
		}
		for _, e := range bl.Entries {
			if e.Account.Equal(PartitionUrl("BVN0").JoinPath(Synthetic)) && e.Chain == sc.Name() {
				named0map[i] = append(named0map[i], e.Index)
			}
		}
	}
	for b, ix := range named0map {
		sort.Slice(ix, func(i, j int) bool { return ix[i] < ix[j] })
		t.Logf("  block %d names synthetic indices %v (appended %v)", b, ix, appended[b])
	}

	// Every appended entry is named by exactly one block ledger, at its own
	// index, by the ledger of the block that appended it.
	require.Equal(t, appended, named0map,
		"each block ledger names exactly the synthetic chain entries that block appended, at their own indices")
	seen := map[uint64]uint64{}
	for b, ix := range named0map {
		for _, x := range ix {
			if first, ok := seen[x]; ok {
				t.Errorf("block %d names synthetic entry %d, already named by block %d", b, x, first)
			}
			seen[x] = b
		}
	}
	require.Equal(t, int(head.Count), len(seen), "every entry on the chain is named, exactly once")

	// And the naming RESOLVES: the production read path — queryMinorBlock,
	// which goes through loadBlockEntry (internal/api/v3/load.go) — answers
	// each block with the chain entries that block actually appended, and no
	// others.
	chainObj, err := sc.Get()
	require.NoError(t, err)
	ctx := context.Background()
	q := api.Querier2{Querier: sim.S.Services()}
	resolved := map[[32]byte]uint64{}
	for b := range appended {
		blk := b
		rec, err := q.QueryMinorBlock(ctx, PartitionUrl("BVN0"), &api.BlockQuery{Minor: &blk})
		require.NoError(t, err)
		require.NotNil(t, rec.Entries)
		var got []uint64
		for _, e := range rec.Entries.Records {
			if e.Account == nil || !e.Account.Equal(PartitionUrl("BVN0").JoinPath(Synthetic)) || e.Name != sc.Name() {
				continue
			}
			want, err := chainObj.Entry(int64(e.Index))
			require.NoError(t, err)
			require.Equal(t, *(*[32]byte)(want), e.Entry,
				"block %d, synthetic index %d: the query returned the chain's entry at that index", blk, e.Index)
			if first, ok := resolved[e.Entry]; ok {
				t.Errorf("the query reports entry %x as the content of block %d and of block %d", e.Entry[:4], first, blk)
			}
			resolved[e.Entry] = blk
			got = append(got, e.Index)
		}
		sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
		require.Equal(t, appended[blk], got, "the block query reports block %d's own synthetic entries", blk)
	}
	require.Equal(t, int(head.Count), len(resolved),
		"the block query resolves every entry on the synthetic chain, each as the content of one block")

	// The claim under test, stated as an assertion: no block ledger on the
	// DESTINATION side records what was staged and not executed. Naming the
	// producer's synthetic chain says what was sent, never what was held.
	require.Zero(t, countHeldNamed(facts, held), "no held entry is named by any block ledger")
}

func countHeldNamed(f *blockLedgerFacts, held []heldFact) int {
	n := 0
	for _, h := range held {
		if _, ok := f.hashes[h.Hash]; ok {
			n++
		}
	}
	return n
}
