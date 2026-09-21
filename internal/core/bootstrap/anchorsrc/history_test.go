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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// producer is a partition's store with the chains a block end writes, built
// the way block_end.go builds them: an account's main chain and the ledger's
// bpt chain, each anchored into the root chain and indexed, and the root chain
// indexed by block. It is served by the production querier.
type producer struct {
	t       *testing.T
	db      *database.Database
	account *url.URL
	blocks  uint64
}

func newProducer(t *testing.T) *producer {
	p := &producer{t: t, db: database.OpenInMemory(nil), account: bvn0().JoinPath("alice")}
	batch := p.db.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Account(bvn0().JoinPath(protocol.Ledger)).Main().Put(
		&protocol.SystemLedger{Url: bvn0().JoinPath(protocol.Ledger)}))
	require.NoError(t, batch.Account(p.account).Main().Put(&protocol.UnknownAccount{Url: p.account}))
	require.NoError(t, batch.Commit())
	return p
}

// block ends a block whose previous state root was bptRoot.
func (p *producer) block(bptRoot [32]byte) {
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
	anchor(ledger.BptChain(), bptRoot[:])
	p.index(ledger.RootChain().Index(), &protocol.IndexEntry{
		BlockIndex: p.blocks,
		Source:     uint64(rootChain.Height() - 1),
	})
	require.NoError(p.t, batch.Commit())
}

func (p *producer) index(c *database.Chain2, e *protocol.IndexEntry) {
	b, err := e.MarshalBinary()
	require.NoError(p.t, err)
	chain, err := c.Get()
	require.NoError(p.t, err)
	require.NoError(p.t, chain.AddEntry(b, false))
}

// rootChain is what an anchor sent now would sign: the root chain's last
// index and its anchor.
func (p *producer) rootChain() (uint64, [32]byte) {
	batch := p.db.Begin(false)
	defer batch.Discard()
	chain, err := batch.Account(bvn0().JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(p.t, err)
	return uint64(chain.Height() - 1), *(*[32]byte)(chain.Anchor())
}

func (p *producer) peer() api.Querier {
	return apiimpl.NewQuerier(apiimpl.QuerierParams{Database: p.db, Partition: "BVN0"})
}

// verified is a source that has verified one anchor of the producer as it
// stands.
func (p *producer) verified(f *netFixture, signers ...int) *Source {
	index, anchor := p.rootChain()
	signed := f.anchor(p.t, anchorOpts{source: bvn0(), destination: dn(), block: p.blocks, root: root(0x70),
		signers: signers, rootChainIndex: index, rootChainAnchor: anchor})
	return sourceOver(p.t, f.authority(p.t), dn().JoinPath(protocol.AnchorPool), bvn0(), signed)
}

// A root no anchor carries -- the root a peer's state is current at -- is
// proven by the bpt chain entry that records it and a receipt to the root
// chain anchor a verified anchor carries. An entry the peer does not have yet
// is a wait.
func TestARootIsProvenByTheHistoryAVerifiedAnchorCarries(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := byte(1); i <= 11; i++ {
		p.block(root(i))
	}
	s := p.verified(f, 0, 1, 2)

	for i := byte(1); i <= 11; i++ {
		ok, err := s.ProveRoot(ctx, p.peer(), root(i), 0)
		require.NoError(t, err)
		require.True(t, ok, "root %d is on the bpt chain under a verified anchor", i)
	}

	ok, err := s.ProveRoot(ctx, p.peer(), root(0x44), 0)
	require.NoError(t, err)
	require.False(t, ok, "a root the chain does not record yet is a wait")

	// Recorded, and anchored after the only verified anchor: a wait as well.
	p.block(root(12))
	ok, err = s.ProveRoot(ctx, p.peer(), root(12), 0)
	require.NoError(t, err)
	require.False(t, ok, "a root recorded after the latest verified anchor waits for the next one")
}

// lyingPeer answers a query for a bpt entry with a receipt of its own choosing
// and everything else honestly.
type lyingPeer struct {
	api.Querier
	entry   [32]byte
	index   uint64
	receipt *merkle.Receipt
}

func (l *lyingPeer) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	cq, ok := q.(*api.ChainQuery)
	if !ok || cq.Name != "bpt" || string(cq.Entry) != string(l.entry[:]) {
		return l.Querier.Query(ctx, scope, q)
	}
	return &api.ChainEntryRecord[api.Record]{Name: "bpt", Index: l.index, Entry: l.entry,
		Receipt: &api.Receipt{Receipt: *l.receipt}}, nil
}

// Everything a partition anchors hangs under its root chain, so a valid
// receipt to a signed root chain anchor proves only that the partition
// recorded the hash somewhere. A root is proven by the BPT CHAIN's entry: a
// receipt that starts at another chain's anchor, or at another chain's entry,
// is refused though every hash in it is true (#4301).
func TestAReceiptThatIsNotTheBptChainsIsRefused(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := byte(1); i <= 11; i++ {
		p.block(root(i))
	}
	s := p.verified(f, 0, 1, 2)
	height, _ := p.rootChain()

	batch := p.db.Begin(false)
	defer batch.Discard()
	main, err := batch.Account(p.account).MainChain().Get()
	require.NoError(t, err)
	rootChain, err := batch.Account(bvn0().JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)

	// The main chain's anchor at block 3 is root chain entry 4: a root chain
	// leaf, and not the bpt chain's.
	var leaf [32]byte
	e, err := rootChain.Entry(4)
	require.NoError(t, err)
	copy(leaf[:], e)
	fromLeaf, err := rootChain.Receipt(4, int64(height))
	require.NoError(t, err)
	require.True(t, fromLeaf.Validate(nil))

	ok, err := s.ProveRoot(ctx, &lyingPeer{Querier: p.peer(), entry: leaf, index: 2, receipt: fromLeaf}, leaf, 0)
	require.Error(t, err, "a receipt that starts at a root chain leaf the bpt chain did not write is refused")
	require.False(t, ok)

	// A transaction on the main chain, with its whole true receipt.
	var txn [32]byte
	copy(txn[:], entryHash("txn", 3))
	fromTxn, err := main.Receipt(2, 2)
	require.NoError(t, err)
	fromTxn, err = fromTxn.Combine(fromLeaf)
	require.NoError(t, err)
	require.True(t, fromTxn.Validate(nil))

	ok, err = s.ProveRoot(ctx, &lyingPeer{Querier: p.peer(), entry: txn, index: 2, receipt: fromTxn}, txn, 0)
	require.Error(t, err, "a transaction's receipt is refused as a root's")
	require.False(t, ok)
}

// A receipt to an anchor no verified anchor carries is refused.
func TestAReceiptToAnUnverifiedAnchorIsRefused(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := byte(1); i <= 5; i++ {
		p.block(root(i))
	}
	s := p.verified(f, 0, 1, 2)

	batch := p.db.Begin(false)
	defer batch.Discard()
	bpt, err := batch.Account(bvn0().JoinPath(protocol.Ledger)).BptChain().Get()
	require.NoError(t, err)
	short, err := bpt.Receipt(0, 4) // Ends at the bpt chain's anchor, which nobody signs
	require.NoError(t, err)

	ok, err := s.ProveRoot(ctx, &lyingPeer{Querier: p.peer(), entry: root(1), receipt: short}, root(1), 0)
	require.Error(t, err)
	require.False(t, ok)
}

// failingPeer answers nothing.
type failingPeer struct{}

func (failingPeer) Query(context.Context, *url.URL, api.Query) (api.Record, error) {
	return nil, errors.NoPeer.With("no peer answered")
}

// Waiting is for a root the chain has not recorded or no anchor has reached.
// Peers that do not answer, a state served with no receipt, and a root the
// history has passed are not waits: waiting on them is waiting for nothing.
func TestWhatIsNotAWaitIsAnError(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := byte(1); i <= 5; i++ {
		p.block(root(i))
	}
	s := p.verified(f, 0, 1, 2) // The anchor of block 5

	_, err := s.ProveRoot(ctx, failingPeer{}, root(1), 0)
	require.Error(t, err, "no peer answered")

	_, err = s.ProveRoot(ctx, p.peer(), [32]byte{}, 0)
	require.Error(t, err, "a zero root is a state served without a receipt")

	ok, err := s.ProveRoot(ctx, p.peer(), root(0x44), 5)
	require.NoError(t, err)
	require.False(t, ok, "served at the latest anchored block: the next block records it")

	_, err = s.ProveRoot(ctx, p.peer(), root(0x44), 4)
	require.Error(t, err, "served at block 4, the anchor of block 5 is verified, and the chain does not record it")
}

// Nothing is proven before an anchor is verified: an unsigned anchor's root
// chain anchor is a peer's word.
func TestNoRootIsProvenWithoutAVerifiedAnchor(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	p := newProducer(t)
	for i := byte(1); i <= 5; i++ {
		p.block(root(i))
	}
	s := p.verified(f) // Nobody signed it

	ok, err := s.ProveRoot(ctx, p.peer(), root(3), 0)
	require.NoError(t, err)
	require.False(t, ok)
}
