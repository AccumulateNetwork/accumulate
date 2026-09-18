// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
		"math/big"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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

	hold.Store(true)
	for i := uint64(1); i <= 10; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
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

	// The synthetic chain to BVN1 exists and has the ten entries on it — it
	// is simply not in any block ledger, and that is deliberate.
	// enumerateModifiedChains (block_end.go:911, called at :120, before
	// recordBlockLedger at :245) throws away everything DidAddChainEntry
	// recorded and rebuilds the list from the batch's updated accounts,
	// SKIPPING the partition's synthetic account outright:
	//
	//	// Anchoring the synthetic transaction ledger causes sadness and
	//	// despair (it breaks things but I don't know why)
	//	_, ok := protocol.ParsePartitionUrl(e.Account)
	//	if ok && e.Account.PathEqual(protocol.Synthetic) {
	//		continue
	//	}
	//
	// anchorSynthChains does call DidUpdateChain for it (block_end.go:557),
	// but that runs at :288 — after the record has been marshalled and
	// hashed. So the one account Paul's proposal names is the one account
	// the block ledger never names.
	sc := b0.Account(PartitionUrl("BVN0").JoinPath(Synthetic)).SyntheticChain("BVN1")
	head, err := sc.Inner().Head().Get()
	require.NoError(t, err)
	t.Logf("BVN0's synthetic chain to BVN1 (%s) has height %d, named by %d block ledgers",
		sc.Name(), head.Count, chains0[synth0+";"+sc.Name()])
	require.NotZero(t, head.Count, "the source did append to its synthetic chain")
	require.Equal(t, int(head.Count), chains0[synth0+";"+sc.Name()],
		"every block that appended to the synthetic chain names it in that block's ledger: "+
			"the block ledger is built last, after anchorSynthChains has registered the entries")

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

// TestAnchorsAreStagedToo answers the "is it only synthetics?" question
// empirically: an anchor below its validator-signature quorum is held in the
// anchor stream's stage, exactly as an unproven synthetic is held in the
// synthetic stream's (msg_block_anchor.go, "an entry in the anchor stream's
// stage at its number").
