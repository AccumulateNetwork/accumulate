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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// anchored is a source that has verified the producer's anchors for the
// given blocks, each carrying root(block), signed by a quorum.
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

// A root is proven by one thing: it equals the StateTreeAnchor of an anchor a
// quorum signed. The anchor of block N carries the root block N committed, so
// a state a peer served at block N is proven when that anchor is verified,
// and not before.
func TestARootIsProvenByEqualityWithAVerifiedAnchorsStateTreeAnchor(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	s := anchored(t, f, 3, 5)

	for _, b := range []byte{3, 5} {
		ok, err := s.ProveRoot(ctx, root(b), uint64(b))
		require.NoError(t, err)
		require.True(t, ok, "block %d's root is what its verified anchor carries", b)
	}

	// The root a peer is current at after the latest anchored block: the
	// next anchor may carry it, so it waits.
	ok, err := s.ProveRoot(ctx, root(6), 6)
	require.NoError(t, err)
	require.False(t, ok, "a root served after the latest verified anchor waits for the next one")

	// And when that anchor arrives, it is proven.
	pool := s.Query.(*poolQuerier)
	pool.entries = append(pool.entries, f.anchor(t, anchorOpts{
		source: bvn0(), destination: dn(), block: 6, root: root(6), signers: []int{0, 1, 2},
	}))
	ok, err = s.ProveRoot(ctx, root(6), 6)
	require.NoError(t, err)
	require.True(t, ok, "the anchor of the block the state was served at is verified")
}

// anythingPeer serves the pool honestly and answers every other query with a
// record of its own making, so that whatever ProveRoot might ask a peer
// besides the pool, the peer is glad to supply.
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

// A root equal to no verified StateTreeAnchor is never proven, whatever a
// peer serves for it: an anchor nobody signed, an anchor one validator
// signed, a chain entry with a receipt. Nothing but a quorum's signatures on
// that exact root is proof, and ProveRoot asks the peers for nothing else.
func TestARootEqualToNoVerifiedStateTreeAnchorIsNeverProven(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	forged := root(0x44)
	pool := &poolQuerier{pool: dn().JoinPath(protocol.AnchorPool), entries: []*api.MessageRecord[messaging.Message]{
		// The anchor of block 4 as one peer would like it: unsigned.
		f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 4, root: forged}),
		// The same, with one validator's signature.
		f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 4, root: forged, signers: []int{3}}),
		// And a real one, for block 5.
		f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 5, root: root(5), signers: []int{0, 1, 2}}),
	}}
	peer := &anythingPeer{poolQuerier: pool}
	s, err := New(peer, pool.pool, bvn0(), f.authority(t))
	require.NoError(t, err)

	ok, err := s.ProveRoot(ctx, forged, 0)
	require.NoError(t, err)
	require.False(t, ok, "a root no quorum signed was proven")

	ok, err = s.ProveRoot(ctx, forged, 5)
	require.NoError(t, err)
	require.False(t, ok, "a root no quorum signed was proven")

	// Served at block 4, and block 5's anchor is verified: if block 4 had
	// sent an anchor it would be verified too, and it does not carry this.
	ok, err = s.ProveRoot(ctx, forged, 4)
	require.Error(t, err, "the history has passed a root no anchor carries, and that is not a wait")
	require.False(t, ok)

	require.Zero(t, peer.asked, "ProveRoot asked a peer for something other than the pool")
	ok, err = s.ProveRoot(ctx, root(5), 5)
	require.NoError(t, err)
	require.True(t, ok, "the root the quorum did sign is proven")
}

// Waiting is for a root no verified anchor carries yet. A state served with
// no receipt, and a root the history has passed, are not waits: waiting on
// them is waiting for nothing, and the state is fetched again.
func TestWhatIsNotAWaitIsAnError(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	s := anchored(t, f, 5) // The anchor of block 5

	_, err := s.ProveRoot(ctx, [32]byte{}, 0)
	require.Error(t, err, "a zero root is a state served without a receipt")
	require.True(t, errors.Is(err, errors.BadRequest))

	ok, err := s.ProveRoot(ctx, root(0x44), 5)
	require.NoError(t, err)
	require.False(t, ok, "served at the latest anchored block, and not that block's root: this peer's word against the quorum's, and it waits for the next anchor")

	ok, err = s.ProveRoot(ctx, root(0x44), 0)
	require.NoError(t, err)
	require.False(t, ok, "served at no block the peer named: a wait")

	_, err = s.ProveRoot(ctx, root(0x44), 4)
	require.Error(t, err, "served at block 4, the anchor of block 5 is verified, and no verified anchor carries it")
	require.True(t, errors.Is(err, errors.NotFound))
}

// Nothing is proven before an anchor is verified: an unsigned anchor's
// StateTreeAnchor is a peer's word.
func TestNoRootIsProvenWithoutAVerifiedAnchor(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	unsigned := f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 5, root: root(5)})
	s := sourceOver(t, f.authority(t), dn().JoinPath(protocol.AnchorPool), bvn0(), unsigned)

	ok, err := s.ProveRoot(ctx, root(5), 5)
	require.NoError(t, err)
	require.False(t, ok)

	// A pool that cannot be read is an error, and the join fetches again;
	// what was verified before the peers went away stays proven.
	s.Query = failingPeer{}
	_, err = s.ProveRoot(ctx, root(5), 5)
	require.Error(t, err)

	proven := anchored(t, f, 5)
	ok, err = proven.ProveRoot(ctx, root(5), 5)
	require.NoError(t, err)
	require.True(t, ok)
	proven.Query = failingPeer{}
	ok, err = proven.ProveRoot(ctx, root(5), 5)
	require.NoError(t, err, "a root verified already is proven whether or not the pool answers this round")
	require.True(t, ok)
}

// failingPeer answers nothing.
type failingPeer struct{}

func (failingPeer) Query(context.Context, *url.URL, api.Query) (api.Record, error) {
	return nil, errors.NoPeer.With("no peer answered")
}
