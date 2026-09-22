// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// anchored is a source that has verified the producer's anchors for the
// given blocks, each carrying root(block), signed by a quorum. The anchors
// say nothing about the root chain, so only equality can prove anything.
func anchored(t *testing.T, f *netFixture, blocks ...byte) *Source {
	t.Helper()
	var entries []*api.MessageRecord[messaging.Message]
	for _, b := range blocks {
		entries = append(entries, f.anchor(t, anchorOpts{
			source: bvn0(), destination: dn(), block: uint64(b), root: root(b), signers: []int{0, 1, 2},
		}))
	}
	return sourceOver(t, f.authority(t), dn().JoinPath(protocol.AnchorPool), bvn0(), entries...)
}

// A root is proven by equality with the StateTreeAnchor of an anchor a
// quorum signed. The anchor of block N carries the root block N committed, so
// a state a peer served at block N is proven when that anchor is verified,
// and no peer is asked for anything.
func TestARootIsProvenByEqualityWithAVerifiedAnchorsStateTreeAnchor(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	s := anchored(t, f, 3, 5)

	for _, b := range []byte{3, 5} {
		ok, err := s.ProveRoot(ctx, failingPeer{}, root(b), uint64(b))
		require.NoError(t, err)
		require.True(t, ok, "block %d's root is what its verified anchor carries", b)
	}

	// The root a peer is current at after the latest anchored block: the
	// next anchor may carry it, so it waits, and no peer is asked.
	ok, err := s.ProveRoot(ctx, failingPeer{}, root(6), 6)
	require.NoError(t, err)
	require.False(t, ok, "a root served after the latest verified anchor waits for the next one")

	// And when that anchor arrives, it is proven.
	pool := s.Query.(*poolQuerier)
	pool.entries = append(pool.entries, f.anchor(t, anchorOpts{
		source: bvn0(), destination: dn(), block: 6, root: root(6), signers: []int{0, 1, 2},
	}))
	ok, err = s.ProveRoot(ctx, failingPeer{}, root(6), 6)
	require.NoError(t, err)
	require.True(t, ok, "the anchor of the block the state was served at is verified")
}

// producer is a partition's store with the chains a block end writes, built
// the way block_end.go builds them: an account's main chain, and the ledger's
// bpt chain recording the PREVIOUS block's root, each anchored into the root
// chain and indexed, and the root chain indexed by block. It is served by the
// production querier, and it keeps what the anchor of each block signs about
// the root chain.
type producer struct {
	t       *testing.T
	db      *database.Database
	account *url.URL
	blocks  uint64
	signed  map[uint64]rootChainAt
}

// rootChainAt is the root chain as it stood at the end of a block: the index
// of its last entry and its anchor, which that block's anchor signs.
type rootChainAt struct {
	index  uint64
	anchor [32]byte
}

func newProducer(t *testing.T) *producer {
	p := &producer{t: t, db: database.OpenInMemory(nil), account: bvn0().JoinPath("alice"), signed: map[uint64]rootChainAt{}}
	batch := p.db.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Account(bvn0().JoinPath(protocol.Ledger)).Main().Put(
		&protocol.SystemLedger{Url: bvn0().JoinPath(protocol.Ledger)}))
	require.NoError(t, batch.Account(p.account).Main().Put(&protocol.UnknownAccount{Url: p.account}))
	require.NoError(t, batch.Commit())
	return p
}

// block ends the next block: a transaction on the account's main chain, the
// previous block's root on the bpt chain, both anchored into the root chain.
func (p *producer) block() {
	p.blocks++
	batch := p.db.Begin(true)
	defer batch.Discard()
	ledger := batch.Account(bvn0().JoinPath(protocol.Ledger))
	rootChain, err := ledger.RootChain().Get()
	require.NoError(p.t, err)

	anchor := func(c *database.Chain2, entry []byte) {
		chain, err := c.Get()
		require.NoError(p.t, err)
		require.NoError(p.t, chain.AddEntry(entry, false))
		require.NoError(p.t, rootChain.AddEntry(chain.Anchor(), false))
		p.index(c.Index(), &protocol.IndexEntry{
			BlockIndex: p.blocks,
			Source:     uint64(chain.Height() - 1),
			Anchor:     uint64(rootChain.Height() - 1),
		})
	}
	anchor(batch.Account(p.account).MainChain(), entryHash("txn", int(p.blocks)))
	previous := root(byte(p.blocks - 1))
	anchor(ledger.BptChain(), previous[:])
	p.index(ledger.RootChain().Index(), &protocol.IndexEntry{
		BlockIndex: p.blocks,
		Source:     uint64(rootChain.Height() - 1),
	})
	require.NoError(p.t, batch.Commit())
	p.signed[p.blocks] = rootChainAt{uint64(rootChain.Height() - 1), *(*[32]byte)(rootChain.Anchor())}
}

func (p *producer) index(c *database.Chain2, e *protocol.IndexEntry) {
	b, err := e.MarshalBinary()
	require.NoError(p.t, err)
	chain, err := c.Get()
	require.NoError(p.t, err)
	require.NoError(p.t, chain.AddEntry(b, false))
}

// anchorOf is the anchor block sends: the root it committed, and the root
// chain as it stood when the block ended.
func (p *producer) anchorOf(f *netFixture, block uint64, signers ...int) *api.MessageRecord[messaging.Message] {
	rc, ok := p.signed[block]
	require.True(p.t, ok, "block %d has not ended", block)
	return f.anchor(p.t, anchorOpts{source: bvn0(), destination: dn(), block: block, root: root(byte(block)),
		signers: signers, rootChainIndex: rc.index, rootChainAnchor: rc.anchor})
}

func (p *producer) peer() api.Querier {
	return apiimpl.NewQuerier(apiimpl.QuerierParams{Database: p.db, Partition: "BVN0"})
}

// verified is a source that has verified the producer's anchors for the given
// blocks, each signed by signers.
func (p *producer) verified(f *netFixture, blocks []uint64, signers ...int) *Source {
	var entries []*api.MessageRecord[messaging.Message]
	for _, b := range blocks {
		entries = append(entries, p.anchorOf(f, b, signers...))
	}
	return sourceOver(p.t, f.authority(p.t), dn().JoinPath(protocol.AnchorPool), bvn0(), entries...)
}

// A root no anchor carries -- the root a peer's state is current at between
// two anchored blocks -- is proven by the history: the bpt chain from a
// verified anchor's root to it, held to the root chain anchor a later verified
// anchor signs. A root the chain does not record is a wait if the peer named
// no block, and an error if it named one the history has passed; a root
// recorded after the latest verified anchor waits for the next one.
func TestARootIsProvenByTheHistoryBetweenTwoVerifiedAnchors(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := 0; i < 11; i++ {
		p.block()
	}
	s := p.verified(f, []uint64{4, 11}, 0, 1, 2)

	for b := byte(5); b <= 10; b++ {
		ok, err := s.ProveRoot(ctx, p.peer(), root(b), uint64(b))
		require.NoError(t, err, "block %d", b)
		require.True(t, ok, "block %d's root is on the bpt chain between the roots of blocks 4 and 11", b)
	}
	ok, err := s.ProveRoot(ctx, p.peer(), root(7), 0)
	require.NoError(t, err)
	require.True(t, ok, "the block the root was served at is not needed for the history")
	for _, b := range []byte{4, 11} {
		ok, err := s.ProveRoot(ctx, failingPeer{}, root(b), uint64(b))
		require.NoError(t, err)
		require.True(t, ok, "block %d's root is what its verified anchor carries", b)
	}

	ok, err = s.ProveRoot(ctx, p.peer(), root(0x44), 0)
	require.NoError(t, err)
	require.False(t, ok, "a root the chain does not record, served at no block the peer named, is a wait")

	_, err = s.ProveRoot(ctx, p.peer(), root(0x44), 7)
	require.Error(t, err, "served at block 7, the anchor of block 11 is verified, and the chain does not record it")
	require.True(t, errors.Is(err, errors.NotFound))

	// Block 12 records block 11's root. Block 12's own root is under no
	// verified anchor yet.
	p.block()
	ok, err = s.ProveRoot(ctx, p.peer(), root(12), 12)
	require.NoError(t, err)
	require.False(t, ok, "a root served after the latest verified anchor waits for the next one")
}

// The base is the latest verified anchor below the served block, whichever
// that is. Served below every verified anchor there is no history to read,
// and that is not a wait.
func TestTheHistoryStartsAtTheLatestVerifiedAnchorBelowTheServedBlock(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := 0; i < 11; i++ {
		p.block()
	}
	s := p.verified(f, []uint64{2, 4, 11}, 0, 1, 2)

	for b := byte(3); b <= 10; b++ {
		ok, err := s.ProveRoot(ctx, p.peer(), root(b), uint64(b))
		require.NoError(t, err, "block %d", b)
		require.True(t, ok, "block %d", b)
	}

	_, err := s.ProveRoot(ctx, p.peer(), root(1), 1)
	require.Error(t, err, "served below every verified anchor: no history reaches it, and no anchor to come will")
	require.True(t, errors.Is(err, errors.NotFound))
}

// lyingPeer answers a query for a bpt entry with a receipt and an index of its
// own choosing, serves that entry at that index of the bpt chain, and answers
// everything else honestly.
type lyingPeer struct {
	api.Querier
	entry   [32]byte
	index   uint64
	receipt *merkle.Receipt
}

func (l *lyingPeer) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	cq, ok := q.(*api.ChainQuery)
	if !ok || cq.Name != "bpt" {
		return l.Querier.Query(ctx, scope, q)
	}
	switch {
	case cq.Entry != nil && string(cq.Entry) == string(l.entry[:]):
		return &api.ChainEntryRecord[api.Record]{Name: "bpt", Index: l.index, Entry: l.entry,
			Receipt: &api.Receipt{Receipt: *l.receipt}}, nil
	case cq.Range != nil:
		return l.rangeWithEntry(ctx, scope, cq)
	}
	return l.Querier.Query(ctx, scope, q)
}

// rangeWithEntry serves the honest range with the lie in its place: the entry
// at its claimed index, appended if the real chain ends before it.
func (l *lyingPeer) rangeWithEntry(ctx context.Context, scope *url.URL, cq *api.ChainQuery) (api.Record, error) {
	rec, err := l.Querier.Query(ctx, scope, cq)
	if err != nil {
		return nil, err
	}
	rr, ok := rec.(*api.RecordRange[api.Record])
	if !ok {
		return rec, nil
	}
	if l.index < cq.Range.Start || l.index >= cq.Range.Start+*cq.Range.Count {
		return rr, nil
	}
	for _, r := range rr.Records {
		if e, ok := r.(*api.ChainEntryRecord[api.Record]); ok && e.Index == l.index {
			e.Entry = l.entry
			return rr, nil
		}
	}
	for i := rr.Start + uint64(len(rr.Records)); i <= l.index; i++ {
		entry := l.entry
		if i < l.index {
			entry = root(byte(i)) // Whatever is true for the entries between
		}
		rr.Records = append(rr.Records, &api.ChainEntryRecord[api.Record]{Name: "bpt", Index: i, Entry: entry})
	}
	return rr, nil
}

// Everything a partition anchors hangs under its root chain, so a valid
// receipt to a signed root chain anchor proves only that the partition
// recorded the hash somewhere. A root is proven by the BPT CHAIN's history:
// a receipt that starts at another chain's anchor, or at another chain's
// entry, is refused though every hash in it is true and the peer serves the
// entry as the bpt chain's (#4301).
func TestAReceiptThatIsNotTheBptChainsIsRefused(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := 0; i < 11; i++ {
		p.block()
	}
	s := p.verified(f, []uint64{4, 11}, 0, 1, 2)
	height := p.signed[11].index

	batch := p.db.Begin(false)
	defer batch.Discard()
	main, err := batch.Account(p.account).MainChain().Get()
	require.NoError(t, err)
	rootChain, err := batch.Account(bvn0().JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)

	// Each block adds two root chain entries, the main chain's anchor and
	// then the bpt chain's; the main chain's anchor at block 8 is root chain
	// entry 14: a root chain leaf, and not the bpt chain's. Block 8's
	// transaction is main chain entry 7.
	var leaf [32]byte
	e, err := rootChain.Entry(14)
	require.NoError(t, err)
	copy(leaf[:], e)
	fromLeaf, err := rootChain.Receipt(14, int64(height))
	require.NoError(t, err)
	require.True(t, fromLeaf.Validate(nil))

	ok, err := s.ProveRoot(ctx, &lyingPeer{Querier: p.peer(), entry: leaf, index: 8, receipt: fromLeaf}, leaf, 8)
	require.Error(t, err, "a receipt that starts at a root chain leaf the bpt chain did not write is refused")
	require.False(t, ok)

	var txn [32]byte
	copy(txn[:], entryHash("txn", 8))
	fromTxn, err := main.Receipt(7, 7)
	require.NoError(t, err)
	fromTxn, err = fromTxn.Combine(fromLeaf)
	require.NoError(t, err)
	require.True(t, fromTxn.Validate(nil))

	// Served as bpt entry 7: three steps below the root chain, which is what
	// entry 7 of a bpt chain has, so only the history can refuse it.
	ok, err = s.ProveRoot(ctx, &lyingPeer{Querier: p.peer(), entry: txn, index: 7, receipt: fromTxn}, txn, 8)
	require.Error(t, err, "a transaction's receipt is refused as a root's")
	require.True(t, errors.Is(err, errors.Conflict))
	require.False(t, ok)
}

// A receipt to an anchor no verified anchor carries is refused.
func TestAReceiptToAnUnverifiedAnchorIsRefused(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := 0; i < 5; i++ {
		p.block()
	}
	s := p.verified(f, []uint64{2, 5}, 0, 1, 2)

	batch := p.db.Begin(false)
	defer batch.Discard()
	bpt, err := batch.Account(bvn0().JoinPath(protocol.Ledger)).BptChain().Get()
	require.NoError(t, err)
	short, err := bpt.Receipt(3, 4) // Ends at the bpt chain's anchor, which nobody signs
	require.NoError(t, err)

	ok, err := s.ProveRoot(ctx, &lyingPeer{Querier: p.peer(), entry: root(3), index: 3, receipt: short}, root(3), 3)
	require.Error(t, err)
	require.False(t, ok)
}

// Waiting is for a root no verified anchor reaches yet. Peers that do not
// answer, a state served with no receipt, and a root the history has passed
// are not waits: waiting on them is waiting for nothing, and the state is
// fetched again.
func TestWhatIsNotAWaitIsAnError(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := 0; i < 5; i++ {
		p.block()
	}
	s := p.verified(f, []uint64{2, 5}, 0, 1, 2) // The anchors of blocks 2 and 5

	_, err := s.ProveRoot(ctx, failingPeer{}, root(3), 3)
	require.Error(t, err, "no peer answered for the history")

	_, err = s.ProveRoot(ctx, p.peer(), [32]byte{}, 0)
	require.Error(t, err, "a zero root is a state served without a receipt")
	require.True(t, errors.Is(err, errors.BadRequest))

	ok, err := s.ProveRoot(ctx, failingPeer{}, root(0x44), 5)
	require.NoError(t, err)
	require.False(t, ok, "served at the latest anchored block: no verified anchor reaches its root yet, and it waits without asking a peer")

	ok, err = s.ProveRoot(ctx, p.peer(), root(0x44), 0)
	require.NoError(t, err)
	require.False(t, ok, "served at no block the peer named, and not recorded: a wait")

	_, err = s.ProveRoot(ctx, p.peer(), root(0x44), 4)
	require.Error(t, err, "served at block 4, the anchor of block 5 is verified, and the chain does not record it")
	require.True(t, errors.Is(err, errors.NotFound))
}

// Nothing is proven before an anchor is verified: an unsigned anchor's
// StateTreeAnchor and root chain anchor are a peer's word.
func TestNoRootIsProvenWithoutAVerifiedAnchor(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := 0; i < 5; i++ {
		p.block()
	}
	s := p.verified(f, []uint64{2, 5}) // Nobody signed them

	ok, err := s.ProveRoot(ctx, p.peer(), root(3), 3)
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = s.ProveRoot(ctx, p.peer(), root(5), 5)
	require.NoError(t, err)
	require.False(t, ok)

	// A pool that cannot be read is an error, and the join fetches again;
	// what was verified before the peers went away stays proven.
	s.Query = failingPeer{}
	_, err = s.ProveRoot(ctx, p.peer(), root(5), 5)
	require.Error(t, err)

	proven := p.verified(f, []uint64{2, 5}, 0, 1, 2)
	ok, err = proven.ProveRoot(ctx, p.peer(), root(5), 5)
	require.NoError(t, err)
	require.True(t, ok)
	proven.Query = failingPeer{}
	ok, err = proven.ProveRoot(ctx, p.peer(), root(5), 5)
	require.NoError(t, err, "a root verified already is proven whether or not the pool answers this round")
	require.True(t, ok)
}

// anythingPeer serves the pool honestly and answers every other query with a
// record of its own making: whatever ProveRoot asks a peer besides the pool,
// the peer is glad to supply.
type anythingPeer struct {
	*poolQuerier
	asked int
}

func (p *anythingPeer) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	if scope.Equal(p.pool) {
		return p.poolQuerier.Query(ctx, scope, q)
	}
	p.asked++
	return &api.ChainEntryRecord[api.Record]{Name: "bpt", Entry: root(0x44), Receipt: &api.Receipt{}}, nil
}

// A root only a peer vouches for is never proven: not by an anchor nobody
// signed, not by an anchor one validator signed, and not by a chain entry
// with a receipt that hashes to nothing a quorum signed. The peer is asked
// for the history, and what it answers is refused.
func TestARootOnlyAPeerVouchesForIsNeverProven(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := 0; i < 5; i++ {
		p.block()
	}
	forged := root(0x44)
	pool := &poolQuerier{pool: dn().JoinPath(protocol.AnchorPool), entries: []*api.MessageRecord[messaging.Message]{
		// The anchor of block 2, real.
		p.anchorOf(f, 2, 0, 1, 2),
		// The anchor of block 4 as one peer would like it: unsigned.
		f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 4, root: forged}),
		// The same, with one validator's signature.
		f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 4, root: forged, signers: []int{3}}),
		// And the anchor of block 5, real.
		p.anchorOf(f, 5, 0, 1, 2),
	}}
	peer := &anythingPeer{poolQuerier: pool}
	s, err := New(pool, pool.pool, bvn0(), f.authority(t))
	require.NoError(t, err)

	ok, err := s.ProveRoot(ctx, peer, forged, 4)
	require.Error(t, err, "a root no quorum signed was proven on a peer's word")
	require.False(t, ok)
	require.NotZero(t, peer.asked, "the history was not read")

	ok, err = s.ProveRoot(ctx, peer, root(5), 5)
	require.NoError(t, err)
	require.True(t, ok, "the root the quorum did sign is proven")
}

// failingPeer answers nothing.
type failingPeer struct{}

func (failingPeer) Query(context.Context, *url.URL, api.Query) (api.Record, error) {
	return nil, errors.NoPeer.With("no peer answered")
}

// forgingPeer is a lyingPeer that also names the bpt chain index of the base
// root -- the verified StateTreeAnchor the history is read from -- as it
// pleases, so that every number the proof could take from a peer is forged.
type forgingPeer struct {
	lyingPeer
	base      [32]byte
	baseIndex uint64
}

func (f *forgingPeer) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	cq, ok := q.(*api.ChainQuery)
	if !ok || cq.Name != "bpt" || cq.Entry == nil || string(cq.Entry) != string(f.base[:]) {
		return f.lyingPeer.Query(ctx, scope, q)
	}
	rec, err := f.lyingPeer.Query(ctx, scope, q)
	if err != nil {
		return nil, err
	}
	rec.(*api.ChainEntryRecord[api.Record]).Index = f.baseIndex
	return rec, nil
}

// The peer that serves a receipt is the peer that says where on the bpt chain
// the entry is, and index chains are not anchored, so nothing a peer says
// about an index is proof. A transaction's true receipt, served as a bpt
// entry's with the base's index, the entry's index and the entries between
// all chosen to fit it, must be refused: a transaction hash is something
// anybody can put on a chain, and a root that is one makes the state served
// under it the peer's word (#4301).
//
// The base index is chosen the way that beats a proof which takes the shape
// of the rebuilt chain from that number: one below a power of two, so that
// the base's whole subtree is one pending hash and the forged entry's receipt
// climbs from it in one step.
func TestATransactionsReceiptIsRefusedThoughThePeerForgesTheBptIndex(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := 0; i < 11; i++ {
		p.block()
	}
	s := p.verified(f, []uint64{4, 11}, 0, 1, 2)
	height := p.signed[11].index

	batch := p.db.Begin(false)
	defer batch.Discard()
	main, err := batch.Account(p.account).MainChain().Get()
	require.NoError(t, err)
	rootChain, err := batch.Account(bvn0().JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)

	// Block 8's transaction is main chain entry 7, and the main chain's anchor
	// for that block is root chain entry 14.
	var txn [32]byte
	copy(txn[:], entryHash("txn", 8))
	fromTxn, err := main.Receipt(7, 7)
	require.NoError(t, err)
	fromLeaf, err := rootChain.Receipt(14, int64(height))
	require.NoError(t, err)
	fromTxn, err = fromTxn.Combine(fromLeaf)
	require.NoError(t, err)
	require.True(t, fromTxn.Validate(nil))

	peer := &forgingPeer{
		lyingPeer: lyingPeer{Querier: p.peer(), entry: txn, index: 7, receipt: fromTxn},
		base:      root(4),
		baseIndex: 1,
	}
	ok, err := s.ProveRoot(ctx, peer, txn, 0)
	require.False(t, ok, "a transaction hash was proven as a BPT root by a peer that forged the bpt chain's index")
	require.Error(t, err)
}
