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

func newNet(t *testing.T, n int, version uint64, spare ...int) *netFixture {
	t.Helper()
	f := &netFixture{}
	def := new(protocol.NetworkDefinition)
	def.NetworkName = "spine-test"
	def.Version = version
	def.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)
	def.AddPartition("BVN0", protocol.PartitionTypeBlockValidator)
	extra := 0
	if len(spare) > 0 {
		extra = spare[0]
	}
	for i := 0; i < n+extra; i++ {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		f.keys = append(f.keys, priv)
		// A spare key is in the definition and active on nothing: a key the
		// network knows and has not given a seat.
		def.AddValidator(priv[32:], protocol.Directory, i < n)
		def.AddValidator(priv[32:], "BVN0", i < n)
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

// withActive is the definition at a new version with exactly these keys
// active on both partitions — an operator change, of any size.
func (f *netFixture) withActive(t *testing.T, version uint64, active ...int) *core.GlobalValues {
	t.Helper()
	next := &core.GlobalValues{Network: f.values.Network.Copy(), Globals: f.values.Globals}
	next.Network.Version = version
	on := map[int]bool{}
	for _, i := range active {
		on[i] = true
	}
	for i := range f.keys {
		_, v, ok := next.Network.ValidatorByKey(f.keys[i][32:])
		require.True(t, ok)
		for _, p := range v.Partitions {
			p.Active = on[i]
		}
	}
	return next
}

// update is the NetworkAccountUpdate a DirectoryAnchor carries to publish a
// new definition — a WriteData on <partition>/network
// (execute/v2/block/network_accounts.go, ParseNetwork).
func update(values *core.GlobalValues) protocol.NetworkAccountUpdate {
	return protocol.NetworkAccountUpdate{
		Name: protocol.Network,
		Body: &protocol.WriteData{Entry: values.FormatNetwork()},
	}
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
	// forceDirBody makes the body a DirectoryAnchor whatever the source is,
	// which is what a forged operator change from a BVN looks like.
	forceDirBody bool

	// The producer's root chain as the anchor states it: the index of its
	// last entry and its anchor, at the end of the anchor's block.
	rootChainIndex  uint64
	rootChainAnchor [32]byte
}

// anchor builds the API record for one anchor in a pool, signed the way
// crosschain signs one.
func (f *netFixture) anchor(t *testing.T, o anchorOpts) *api.MessageRecord[messaging.Message] {
	t.Helper()

	pa := protocol.PartitionAnchor{
		Source:          o.source,
		MinorBlockIndex: o.block,
		StateTreeAnchor: o.root,
		RootChainIndex:  o.rootChainIndex,
		RootChainAnchor: o.rootChainAnchor,
	}
	var body protocol.AnchorBody
	if o.forceDirBody || protocol.DnUrl().Equal(o.source) {
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

// verifiedRoots is the roots of the anchors a quorum of the producer's
// validators signed, judged one by one by VerifyQuorum, the rule the
// collector takes an anchor by.
type verifiedRoots struct{ roots map[uint64][32]byte }

// ErrNotAnchored says no anchor a quorum signed covers the block.
var ErrNotAnchored = errors.NotReady.With("no verified anchor covers that block")

func sourceOver(t *testing.T, a *Authority, _, producer *url.URL, entries ...*api.MessageRecord[messaging.Message]) *verifiedRoots {
	t.Helper()
	id, ok := protocol.ParsePartitionUrl(producer)
	require.True(t, ok)
	v := &verifiedRoots{roots: map[uint64][32]byte{}}
	for _, e := range entries {
		rec, err := api.MessageRecordAs[*messaging.TransactionMessage](e)
		require.NoError(t, err)
		body, ok := rec.Message.Transaction.Body.(protocol.AnchorBody)
		require.True(t, ok)
		pa := body.GetPartitionAnchor()
		if !pa.Source.Equal(producer) {
			continue
		}
		if VerifyQuorum(a, id, rec) == nil {
			v.roots[pa.MinorBlockIndex] = pa.StateTreeAnchor
		}
	}
	return v
}

func (v *verifiedRoots) Read(context.Context) error { return nil }

func (v *verifiedRoots) AnchoredRoot(_ context.Context, _ *url.URL, block uint64) ([32]byte, error) {
	r, ok := v.roots[block]
	if !ok {
		return [32]byte{}, ErrNotAnchored
	}
	return r, nil
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

	_, err := s.AnchoredRoot(ctx, bvn0(), 7)
	require.ErrorIs(t, err, ErrNotAnchored)
}

// (c) Fewer than the threshold of DISTINCT verified signers is refused, and
// neither repeats of one signer nor the API's manufactured signature records
// accumulate.
func TestQuorumCountsDistinctVerifiedSigners(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1) // threshold is 2/3 of 4 = 3

	set, err := f.authority(t).SetFor("BVN0")
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

// The Directory's canonical form of the envelope is accepted too — parity
// with the executor, not a live requirement.
//
// **No anchor this line produces has that form.** The conductor signs one
// envelope per destination (crosschain/conductor.go, sendBlockAnchor →
// PrepareAnchorSubmission(destination)), and a measurement of both pools on
// one and on three validators found zero DirectoryAnchors signed over the
// Directory's form; (e) passes with the second form removed. My earlier note
// said that without this form every DirectoryAnchor in a BVN pool fails
// verification, and that is wrong — corrected on #4301.
//
// The form stays because the executor accepts it (msg_block_anchor.go:333,
// the Vandenberg signature-reuse path, which is how a re-submitted anchor
// carries the Directory's one signature to every BVN), and a verifier of
// anchors that refuses what the executor executes would refuse a root the
// network agreed on. This test pins the parity; it does not claim a live
// producer.
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

// (d) The sets move by Update — a definition the caller verified — and an
// anchor is judged by membership in the set this node trusts, never by the
// version its signature declares.
func TestTheTrustedSetDecides(t *testing.T) {
	ctx := context.Background()
	pool := bvn0().JoinPath(protocol.AnchorPool)

	// Four seated validators and four the network knows but has not seated.
	newFixture := func() *netFixture { return newNet(t, 4, 1, 4) }

	t.Run("an anchor signed after a change this node has not crossed is accepted", func(t *testing.T) {
		// This is what the whole rework is for. Past Vandenberg a change to
		// dn.acme/network never travels in an anchor (block_end.go:791-793),
		// and the anchor of the block that executed the change is already
		// signed under the NEW version. A verifier that demanded the set of
		// the time would refuse every anchor from that moment and the node
		// would never join again (#4301, review finding 1). The declared
		// version is ignored; the signers are members of the set this node
		// holds, and that is enough.
		f := newFixture()
		a := f.authority(t)
		after := f.anchor(t, anchorOpts{
			source: dn(), destination: bvn0(), block: 20, root: root(0x20),
			signers: []int{0, 1, 2}, version: 9, // far ahead of this node
		})
		s := sourceOver(t, a, pool, dn(), after)

		got, err := s.AnchoredRoot(ctx, dn(), 20)
		require.NoError(t, err)
		require.Equal(t, root(0x20), got)
	})

	t.Run("a signature made under a set this node has moved past is refused", func(t *testing.T) {
		f := newFixture()
		a := f.authority(t)
		// The node crosses to version 2 the way the join does: a definition
		// it verified as state.
		require.True(t, a.Update(f.withActive(t, 2, 0, 1, 2)))

		old := f.anchor(t, anchorOpts{
			source: dn(), destination: bvn0(), block: 21, root: root(0x21),
			signers: []int{0, 1, 2}, version: 1, // made under the old set
		})
		s := sourceOver(t, a, pool, dn(), old)

		_, err := s.AnchoredRoot(ctx, dn(), 21)
		require.ErrorIs(t, err, ErrNotAnchored,
			"an old signature was replayed forward past the set that made it")
	})

	t.Run("a superseded quorum cannot sign a new root", func(t *testing.T) {
		// The whole seat turns over: 0-3 out, 4-7 in. An attacker with a
		// quorum of the retired keys -- retired, commonly, because they were
		// compromised -- must not be able to sign anything, whatever version
		// they declare (#4301, threat review F3).
		f := newFixture()
		a := f.authority(t)
		require.True(t, a.Update(f.withActive(t, 2, 4, 5, 6, 7)))

		byRetired := f.anchor(t, anchorOpts{
			source: dn(), destination: bvn0(), block: 31, root: root(0x31),
			signers: []int{0, 1, 2}, version: 2, // declaring the current version
		})
		s := sourceOver(t, a, pool, dn(), byRetired)

		_, err := s.AnchoredRoot(ctx, dn(), 31)
		require.ErrorIs(t, err, ErrNotAnchored, "a retired quorum signed a new root")
	})

	t.Run("the old quorum is not a quorum when the set grew", func(t *testing.T) {
		// The reviewer's discriminating fixture (finding 3): version 2 ADDS
		// validators, so the threshold rises from three to four. Three
		// signers were a quorum before and are not one now, and the only
		// thing that can tell the two apart is which set is being counted
		// against.
		f := newFixture()
		a := f.authority(t)
		before := f.anchor(t, anchorOpts{
			source: dn(), destination: bvn0(), block: 40, root: root(0x40),
			signers: []int{0, 1, 2}, version: 2,
		})

		under1 := sourceOver(t, f.authority(t), pool, dn(), before)
		got, err := under1.AnchoredRoot(ctx, dn(), 40)
		require.NoError(t, err, "three of four is a quorum at version 1")
		require.Equal(t, root(0x40), got)

		require.True(t, a.Update(f.withActive(t, 2, 0, 1, 2, 3, 4, 5)))
		set, err := a.SetFor(protocol.Directory)
		require.NoError(t, err)
		require.Equal(t, uint64(4), set.Threshold, "2/3 of six is four")

		under2 := sourceOver(t, a, pool, dn(), before)
		_, err = under2.AnchoredRoot(ctx, dn(), 40)
		require.ErrorIs(t, err, ErrNotAnchored, "three signers were taken as a quorum of six")
	})

	t.Run("a definition older than the trusted one is ignored", func(t *testing.T) {
		f := newFixture()
		a := f.authority(t)
		require.True(t, a.Update(f.withActive(t, 3, 4, 5, 6, 7)))
		require.False(t, a.Update(f.withActive(t, 2, 0, 1, 2, 3)),
			"a replayed older definition rolled the set back")
		require.Equal(t, uint64(3), a.Version())
	})
}

// An anchor no longer carries the validator sets anywhere, which is both what
// this line does and one fewer way in.
//
// A change to dn.acme/network leaves the Directory as a messaging.NetworkUpdate
// past Vandenberg and never enters an anchor (block_end.go:791-793,
// network_accounts.go:128-131), so a walk over DirectoryAnchor.Updates could
// not fire at all (#4301, review finding 1) -- and while it was here, a
// quorum of ONE BVN could have used it to name the validators of every
// partition (threat review F1). Nothing an anchor carries moves the sets.
func TestAnAnchorNeverMovesTheValidatorSets(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1, 4)
	a := f.authority(t)
	theirs := f.withActive(t, 2, 4, 5, 6, 7)

	// From the Directory, with a full Directory quorum, carrying a perfectly
	// well-formed update.
	fromDn := f.anchor(t, anchorOpts{
		source: dn(), destination: bvn0(), block: 50, root: root(0x50),
		signers: []int{0, 1, 2}, updates: []protocol.NetworkAccountUpdate{update(theirs)},
	})
	// And from a BVN, with a full BVN quorum, which is a quarter of what
	// naming the validators should cost.
	fromBvn := f.anchor(t, anchorOpts{
		source: bvn0(), destination: dn(), block: 51, root: root(0x51),
		signers: []int{0, 1, 2}, forceDirBody: true,
		updates: []protocol.NetworkAccountUpdate{update(theirs)},
	})

	s := sourceOver(t, a, bvn0().JoinPath(protocol.AnchorPool), dn(), fromDn)
	require.NoError(t, s.Read(ctx))
	s2 := sourceOver(t, a, dn().JoinPath(protocol.AnchorPool), bvn0(), fromBvn)
	require.NoError(t, s2.Read(ctx))

	require.Equal(t, uint64(1), a.Version(), "an anchor moved the validator sets")
	set, err := a.SetFor(protocol.Directory)
	require.NoError(t, err)
	require.False(t, set.MaySign(hashOf(f.keys[4][32:])), "a key an anchor named became a signer")

	// Both anchors are still recorded: they were valid for what their own
	// quorum may say, which is their partition's state.
	got, err := s.AnchoredRoot(ctx, dn(), 50)
	require.NoError(t, err)
	require.Equal(t, root(0x50), got)
	got, err = s2.AnchoredRoot(ctx, bvn0(), 51)
	require.NoError(t, err)
	require.Equal(t, root(0x51), got)
}
