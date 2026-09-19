// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A network of one Directory and one BVN, with n nodes that are validators of
// both — which is what a container running a DN node and a BVN node makes.
type netFixture struct {
	keys   []ed25519.PrivateKey
	values *core.GlobalValues
}

func newNet(t *testing.T, n int, version uint64) *netFixture {
	t.Helper()
	f := &netFixture{}
	def := new(protocol.NetworkDefinition)
	def.NetworkName = "spine-test"
	def.Version = version
	def.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)
	def.AddPartition("BVN0", protocol.PartitionTypeBlockValidator)
	for i := 0; i < n; i++ {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		f.keys = append(f.keys, priv)
		def.AddValidator(priv[32:], protocol.Directory, true)
		def.AddValidator(priv[32:], "BVN0", true)
	}
	f.values = &core.GlobalValues{
		Network: def,
		Globals: &protocol.NetworkGlobals{
			ValidatorAcceptThreshold: protocol.Rational{Numerator: 2, Denominator: 3},
			OperatorAcceptThreshold:  protocol.Rational{Numerator: 2, Denominator: 3},
		},
	}
	return f
}

func (f *netFixture) authority(t *testing.T) *Authority {
	t.Helper()
	a, err := FromValues(f.values)
	require.NoError(t, err)
	return a
}

// drop removes a validator from a partition and bumps the definition's
// version, the way an operator change does.
func (f *netFixture) without(t *testing.T, i int, version uint64) *core.GlobalValues {
	t.Helper()
	next := &core.GlobalValues{Network: f.values.Network.Copy(), Globals: f.values.Globals}
	next.Network.Version = version
	_, v, ok := next.Network.ValidatorByKey(f.keys[i][32:])
	require.True(t, ok)
	for _, p := range v.Partitions {
		p.Active = false
	}
	return next
}

// anchorOpts says what anchor to build and who signs it.
type anchorOpts struct {
	source      *url.URL // the partition that PRODUCED the anchor
	destination *url.URL // the partition whose pool it lands in
	block       uint64
	root        [32]byte
	signers     []int  // indices into the fixture's keys
	repeat      int    // how many extra copies of signers[0] to add
	version     uint64 // the network version the signatures declare
	updates     []protocol.NetworkAccountUpdate
	unsigned    bool // add a signer record with no signature bytes, as the API does
	signAsDn    bool // sign the Directory's canonical form, as the DN really does
}

// anchor builds the API record for one anchor in a pool, signed the way
// crosschain signs one.
func (f *netFixture) anchor(t *testing.T, o anchorOpts) *api.MessageRecord[messaging.Message] {
	t.Helper()

	pa := protocol.PartitionAnchor{
		Source:          o.source,
		MinorBlockIndex: o.block,
		StateTreeAnchor: o.root,
	}
	var body protocol.AnchorBody
	if protocol.DnUrl().Equal(o.source) {
		body = &protocol.DirectoryAnchor{PartitionAnchor: pa, Updates: o.updates}
	} else {
		body = &protocol.BlockValidatorAnchor{PartitionAnchor: pa}
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = o.destination.JoinPath(protocol.AnchorPool)
	txn.Body = body
	txnMsg := &messaging.TransactionMessage{Transaction: txn}

	seq := &messaging.SequencedMessage{
		Message:     txnMsg,
		Source:      o.source,
		Destination: o.destination,
		Number:      o.block,
	}

	version := o.version
	if version == 0 {
		version = f.values.Network.Version
	}

	// What the Directory's validators really sign. Post-Vandenberg the DN
	// signs ONE anchor -- destination dn.acme, principal under dn.acme -- and
	// every BVN reuses that signature over its own copy
	// (msg_block_anchor.go, "Allow reusing signatures from the DN"). The
	// record in bvn-X.acme/anchors therefore carries a sequence whose
	// destination is the BVN and a signature made over the Directory's form.
	signed := seq
	if o.signAsDn {
		asDn := *seq
		asDn.Destination = protocol.DnUrl()
		rewritten := txn.Copy()
		rewritten.Header.Principal = protocol.DnUrl().JoinPath(txn.Header.Principal.Path)
		asDn.Message = &messaging.TransactionMessage{Transaction: rewritten}
		signed = &asDn
	}
	h := signed.Hash()

	set := &api.SignatureSetRecord{
		Account:    &protocol.UnknownAccount{Url: txn.Header.Principal},
		Signatures: new(api.RecordRange[*api.MessageRecord[messaging.Message]]),
	}
	add := func(i int) {
		sig, err := new(signing.Builder).
			SetType(protocol.SignatureTypeED25519).
			SetPrivateKey(f.keys[i]).
			SetUrl(protocol.DnUrl().JoinPath(protocol.Network)).
			SetVersion(version).
			SetTimestamp(1).
			Sign(h[:])
		require.NoError(t, err)
		set.Signatures.Records = append(set.Signatures.Records, &api.MessageRecord[messaging.Message]{
			Message: &messaging.BlockAnchor{Anchor: seq, Signature: sig.(protocol.KeySignature)},
		})
	}
	for _, i := range o.signers {
		add(i)
	}
	for j := 0; j < o.repeat; j++ {
		add(o.signers[0])
	}
	if o.unsigned {
		// What internal/api/v3/load.go manufactures from status.AnchorSigners:
		// a BlockAnchor carrying a public key and no signature at all.
		for i := range f.keys {
			set.Signatures.Records = append(set.Signatures.Records, &api.MessageRecord[messaging.Message]{
				Message: &messaging.BlockAnchor{Signature: &protocol.ED25519Signature{
					PublicKey:     f.keys[i][32:],
					Signer:        protocol.DnUrl().JoinPath(protocol.Network),
					SignerVersion: version,
					Timestamp:     1,
				}},
			})
		}
	}
	set.Signatures.Total = uint64(len(set.Signatures.Records))

	rec := &api.MessageRecord[messaging.Message]{
		ID:         txn.ID(),
		Message:    txnMsg,
		Sequence:   seq,
		Signatures: &api.RecordRange[*api.SignatureSetRecord]{Records: []*api.SignatureSetRecord{set}, Total: 1},
	}
	return rec
}

// poolQuerier answers the two queries a Source makes against one pool, out of
// a list of anchors, the way the API answers them.
type poolQuerier struct {
	pool    *url.URL
	entries []*api.MessageRecord[messaging.Message]
}

func (p *poolQuerier) Query(_ context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	if !scope.Equal(p.pool) {
		return nil, errors.NotFound.WithFormat("no such record")
	}
	cq, ok := q.(*api.ChainQuery)
	if !ok {
		return nil, errors.NotFound.WithFormat("no such record")
	}
	if cq.Range == nil {
		return &api.ChainRecord{Name: "main", Count: uint64(len(p.entries))}, nil
	}
	start := cq.Range.Start
	rr := new(api.RecordRange[api.Record])
	for i := start; i < uint64(len(p.entries)); i++ {
		rr.Records = append(rr.Records, &api.ChainEntryRecord[api.Record]{
			Name:  "main",
			Index: i,
			Value: p.entries[i],
		})
	}
	rr.Start = start
	rr.Total = uint64(len(p.entries))
	return rr, nil
}

func dn() *url.URL   { return protocol.DnUrl() }
func bvn0() *url.URL { return protocol.PartitionUrl("BVN0") }

func hashOf(key []byte) []byte {
	h := sha256.Sum256(key)
	return h[:]
}

func root(b byte) [32]byte {
	var r [32]byte
	for i := range r {
		r[i] = b
	}
	return r
}

// sourceOver builds a Source reading the given anchors as one pool.
func sourceOver(t *testing.T, a *Authority, pool, producer *url.URL, entries ...*api.MessageRecord[messaging.Message]) *Source {
	t.Helper()
	s, err := New(&poolQuerier{pool: pool, entries: entries}, pool, producer, a)
	require.NoError(t, err)
	return s
}

// (a) A well-formed anchor with no valid signature is refused.
//
// This is the whole of #4301 at its smallest: pull.DirectoryAnchors recorded
// the StateTreeAnchor of anything in the pool that decoded as an AnchorBody,
// so a peer serving a self-consistent history supplied both the root and the
// state that hashed into it.
func TestAnchorWithNoValidSignatureIsRefused(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)

	// Signed by nobody, but otherwise perfectly formed.
	rec := f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 7, root: root(0xaa)})
	s := sourceOver(t, a, dn().JoinPath(protocol.AnchorPool), bvn0(), rec)

	var observed int
	s.OnAnchor = func(*url.URL, uint64, [32]byte) { observed++ }

	_, err := s.AnchoredRoot(ctx, bvn0(), 7)
	require.ErrorIs(t, err, ErrNotAnchored)
	require.Zero(t, observed, "an unverified root must not reach the tracker")
}

// (c) Fewer than the threshold of DISTINCT verified signers is refused, and
// neither repeats of one signer nor the API's manufactured signature records
// accumulate.
func TestQuorumCountsDistinctVerifiedSigners(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1) // threshold is 2/3 of 4 = 3

	set, err := f.authority(t).SetFor("BVN0", 1)
	require.NoError(t, err)
	require.Equal(t, uint64(3), set.Threshold)

	t.Run("two of four", func(t *testing.T) {
		a := f.authority(t)
		rec := f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 9, root: root(0xbb), signers: []int{0, 1}})
		s := sourceOver(t, a, dn().JoinPath(protocol.AnchorPool), bvn0(), rec)
		_, err := s.AnchoredRoot(ctx, bvn0(), 9)
		require.ErrorIs(t, err, ErrNotAnchored)
	})

	t.Run("one signer three times", func(t *testing.T) {
		a := f.authority(t)
		rec := f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 9, root: root(0xbb), signers: []int{0}, repeat: 4})
		s := sourceOver(t, a, dn().JoinPath(protocol.AnchorPool), bvn0(), rec)
		_, err := s.AnchoredRoot(ctx, bvn0(), 9)
		require.ErrorIs(t, err, ErrNotAnchored, "a second copy from one validator is no second signature")
	})

	t.Run("the API's manufactured signers do not count", func(t *testing.T) {
		a := f.authority(t)
		rec := f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 9, root: root(0xbb), signers: []int{0}, unsigned: true})
		s := sourceOver(t, a, dn().JoinPath(protocol.AnchorPool), bvn0(), rec)
		_, err := s.AnchoredRoot(ctx, bvn0(), 9)
		require.ErrorIs(t, err, ErrNotAnchored,
			"load.go manufactures a BlockAnchor per status.AnchorSigners with no signature bytes")
	})

	t.Run("three of four", func(t *testing.T) {
		a := f.authority(t)
		rec := f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 9, root: root(0xbb), signers: []int{0, 1, 2}})
		s := sourceOver(t, a, dn().JoinPath(protocol.AnchorPool), bvn0(), rec)
		got, err := s.AnchoredRoot(ctx, bvn0(), 9)
		require.NoError(t, err)
		require.Equal(t, root(0xbb), got)
	})
}

// The Directory's own root comes out of a BVN's anchor pool, and the
// signature on it was made over the Directory's own form of the envelope.
func TestTheDirectorysRootComesFromABvnPool(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)

	pool, err := PoolFor(dn(), a.BvnNames())
	require.NoError(t, err)
	require.Equal(t, bvn0().JoinPath(protocol.AnchorPool).String(), pool.String(),
		"the directory's own anchors are the ones it sent out")

	rec := f.anchor(t, anchorOpts{source: dn(), destination: bvn0(), block: 11, root: root(0xcc), signers: []int{0, 1, 2}})
	s := sourceOver(t, a, pool, dn(), rec)

	got, err := s.AnchoredRoot(ctx, dn(), 11)
	require.NoError(t, err)
	require.Equal(t, root(0xcc), got)
}

// And the signature really on it was made over the Directory's own form of
// the envelope, not over the copy sitting in the BVN's pool. A verifier that
// only knows the record's own form refuses every DirectoryAnchor in a BVN
// pool, which is every chance the Directory has of proving its own root.
func TestADirectoryAnchorIsSignedOverTheDirectorysForm(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)
	pool := bvn0().JoinPath(protocol.AnchorPool)

	rec := f.anchor(t, anchorOpts{
		source: dn(), destination: bvn0(), block: 12, root: root(0xdd),
		signers: []int{0, 1, 2}, signAsDn: true,
	})
	s := sourceOver(t, a, pool, dn(), rec)

	got, err := s.AnchoredRoot(ctx, dn(), 12)
	require.NoError(t, err)
	require.Equal(t, root(0xdd), got)
}

// A BVN's root is read from the Directory's pool, and a source pointed at the
// Directory's pool cannot answer for the Directory.
func TestPoolForRoutesByProducer(t *testing.T) {
	pool, err := PoolFor(bvn0(), []string{"BVN0"})
	require.NoError(t, err)
	require.Equal(t, dn().JoinPath(protocol.AnchorPool).String(), pool.String())

	_, err = PoolFor(dn(), nil)
	require.Error(t, err, "with no BVN there is no pool that holds the directory's own anchors")
}

// (d) An anchor signed under a superseded validator set verifies against the
// set of its time, a version the walk has not reached is refused, and a
// signed operator change carried by a verified anchor walks the set forward.
func TestTheValidatorSetOfTheTimeIsWalkedForward(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	pool := bvn0().JoinPath(protocol.AnchorPool)

	// Version 2 drops validator 3. Its definition travels in a
	// DirectoryAnchor's Updates, which is how a BVN learns of the change.
	v2 := f.without(t, 3, 2)
	update := protocol.NetworkAccountUpdate{
		Name: protocol.Network,
		Body: &protocol.WriteData{Entry: v2.FormatNetwork()},
	}

	carrier := f.anchor(t, anchorOpts{
		source: dn(), destination: bvn0(), block: 20, root: root(0x20),
		signers: []int{0, 1, 2}, version: 1, updates: []protocol.NetworkAccountUpdate{update},
	})
	// Signed under version 2, by the set version 2 names.
	after := f.anchor(t, anchorOpts{
		source: dn(), destination: bvn0(), block: 21, root: root(0x21),
		signers: []int{0, 1, 2}, version: 2,
	})
	// Signed under version 2 by a validator version 2 removed, plus two who
	// remain: two of the three are members, and three are required.
	byRemoved := f.anchor(t, anchorOpts{
		source: dn(), destination: bvn0(), block: 22, root: root(0x22),
		signers: []int{0, 1, 3}, version: 2,
	})

	t.Run("a version the walk has not reached is refused", func(t *testing.T) {
		a := f.authority(t)
		s := sourceOver(t, a, pool, dn(), after)
		_, err := s.AnchoredRoot(ctx, dn(), 21)
		require.ErrorIs(t, err, ErrNotAnchored)
		require.Equal(t, uint64(1), a.Version())
	})

	t.Run("a verified change walks the set forward", func(t *testing.T) {
		a := f.authority(t)
		s := sourceOver(t, a, pool, dn(), carrier, after)

		got, err := s.AnchoredRoot(ctx, dn(), 21)
		require.NoError(t, err)
		require.Equal(t, root(0x21), got)
		require.Equal(t, uint64(2), a.Version())

		// And the anchor signed under version 1 is still verified against
		// version 1's set, not against the set that superseded it.
		got, err = s.AnchoredRoot(ctx, dn(), 20)
		require.NoError(t, err)
		require.Equal(t, root(0x20), got)

		v1, err := a.SetFor(protocol.Directory, 1)
		require.NoError(t, err)
		require.Equal(t, 4, v1.Size())
		v2set, err := a.SetFor(protocol.Directory, 2)
		require.NoError(t, err)
		require.Equal(t, 3, v2set.Size())
		require.Equal(t, uint64(2), v2set.Threshold, "2/3 of three is two")
	})

	t.Run("a validator the change removed does not count under the new set", func(t *testing.T) {
		a := f.authority(t)
		// Threshold under version 2 is 2 of 3, and two members signed, so
		// this one does reach it -- what must not happen is the removed
		// validator counting as a third.
		s := sourceOver(t, a, pool, dn(), carrier, byRemoved)
		require.NoError(t, s.Read(ctx))

		set, err := a.SetFor(protocol.Directory, 2)
		require.NoError(t, err)
		require.False(t, set.MaySign(hashOf(f.keys[3][32:])),
			"the set of version 2 does not contain the validator version 2 removed")
	})
}

// An authority walks only on a VERIFIED anchor: a change carried by an anchor
// nobody signed is not a change.
func TestAnUnverifiedAnchorDoesNotWalkTheSet(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)

	v2 := f.without(t, 3, 2)
	carrier := f.anchor(t, anchorOpts{
		source: dn(), destination: bvn0(), block: 30, root: root(0x30),
		updates: []protocol.NetworkAccountUpdate{{
			Name: protocol.Network,
			Body: &protocol.WriteData{Entry: v2.FormatNetwork()},
		}},
	}) // no signers

	s := sourceOver(t, a, bvn0().JoinPath(protocol.AnchorPool), dn(), carrier)
	require.NoError(t, s.Read(ctx))
	require.Equal(t, uint64(1), a.Version(), "one peer must not be able to name the validators")
}
