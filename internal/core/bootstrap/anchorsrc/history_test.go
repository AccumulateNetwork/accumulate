// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// bptQuerier answers a query for one entry of <producer>/ledger's bpt chain,
// the way the API answers it, with a receipt that ends at anchor.
type bptQuerier struct {
	ledger  *url.URL
	entry   [32]byte
	sibling [32]byte
	anchor  []byte // zero means the true anchor of entry and sibling
	asked   *api.ReceiptOptions
}

func (b *bptQuerier) Query(_ context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	cq, ok := q.(*api.ChainQuery)
	if !ok || !scope.Equal(b.ledger) || cq.Name != "bpt" || string(cq.Entry) != string(b.entry[:]) {
		return nil, errors.NotFound.With("no such entry")
	}
	b.asked = cq.IncludeReceipt
	r := merkle.Receipt{
		Start:   b.entry[:],
		Entries: []*merkle.ReceiptEntry{{Right: true, Hash: b.sibling[:]}},
		Anchor:  b.anchor,
	}
	if r.Anchor == nil {
		r.Anchor = combined(b.entry, b.sibling)
	}
	return &api.ChainEntryRecord[api.Record]{Name: "bpt", Entry: b.entry, Receipt: &api.Receipt{Receipt: r}}, nil
}

func combined(a, b [32]byte) []byte {
	h := sha256.Sum256(append(a[:], b[:]...))
	return h[:]
}

// A root no anchor carries -- the root a peer's state is current at -- is
// proven by the bpt chain entry that records it and a receipt to the root
// chain anchor a verified anchor carries. It is proven only there: a receipt
// to any other anchor is refused, and an entry the peer does not have yet is
// a wait.
func TestARootIsProvenByTheHistoryAVerifiedAnchorCarries(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)

	current, sibling := root(0x42), root(0x43)
	var rca [32]byte
	copy(rca[:], combined(current, sibling))

	signed := f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 70, root: root(0x70),
		signers: []int{0, 1, 2}, rootChainIndex: 900, rootChainAnchor: rca})
	s := sourceOver(t, a, dn().JoinPath(protocol.AnchorPool), bvn0(), signed)
	peer := &bptQuerier{ledger: bvn0().JoinPath(protocol.Ledger), entry: current, sibling: sibling}

	ok, err := s.ProveRoot(ctx, peer, current)
	require.NoError(t, err)
	require.True(t, ok, "the peer's current root is provable into a verified anchor")
	require.Equal(t, uint64(900), peer.asked.ForHeight,
		"the receipt is asked for at the root chain index the verified anchor names")

	ok, err = s.ProveRoot(ctx, peer, root(0x44))
	require.NoError(t, err)
	require.False(t, ok, "a root the chain does not record yet is a wait")

	peer.anchor = combined(sibling, current)
	ok, err = s.ProveRoot(ctx, peer, current)
	require.Error(t, err, "a receipt that ends where no verified anchor ends is refused")
	require.False(t, ok)
}

// Nothing is proven before an anchor is verified: an unsigned anchor's root
// chain anchor is a peer's word.
func TestNoRootIsProvenWithoutAVerifiedAnchor(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)

	current, sibling := root(0x42), root(0x43)
	var rca [32]byte
	copy(rca[:], combined(current, sibling))

	unsigned := f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 70, root: root(0x70),
		rootChainIndex: 900, rootChainAnchor: rca})
	s := sourceOver(t, a, dn().JoinPath(protocol.AnchorPool), bvn0(), unsigned)
	peer := &bptQuerier{ledger: bvn0().JoinPath(protocol.Ledger), entry: current, sibling: sibling}

	ok, err := s.ProveRoot(ctx, peer, current)
	require.NoError(t, err)
	require.False(t, ok)
}
