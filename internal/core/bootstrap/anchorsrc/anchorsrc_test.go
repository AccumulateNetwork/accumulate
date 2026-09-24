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
	"fmt"
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
type poolQuerier struct {
	pool    *url.URL
	entries []*api.MessageRecord[messaging.Message]

	// What a lying peer says: a chain count and an entry index from beyond
	// the end of the real chain. Zero means it tells the truth.
	liedCount uint64
	liedIndex uint64

	// countBehind subtracts from the count this peer reports without
	// changing what it serves -- a peer one entry behind the one that
	// served the last page, which per-call rotation makes ordinary.
	countBehind uint64

	// decoy is what a lying peer serves at the window it named, instead of
	// the real chain: a page that comes back, so the cursor advances, made
	// of something this node will not record.
	decoy *api.MessageRecord[messaging.Message]

	// served counts the entries handed out, so a re-read is visible.
	served int

	// holes are entries this peer serves without their body: the value is
	// what it serves in the body's place -- an ErrorRecord, as the API
	// serves a message it does not hold, or nil, no value at all (#4418).
	holes map[int]api.Record
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
		count := uint64(len(p.entries))
		if p.countBehind > 0 && count >= p.countBehind {
			count -= p.countBehind
		}
		if p.liedCount > 0 {
			count = p.liedCount
		}
		return &api.ChainRecord{Name: "main", Count: count}, nil
	}
	start := cq.Range.Start
	rr := new(api.RecordRange[api.Record])
	if p.decoy != nil && p.liedIndex > 0 {
		rr.Records = append(rr.Records, &api.ChainEntryRecord[api.Record]{
			Name: "main", Index: p.liedIndex, Value: p.decoy,
		})
		rr.Start = start
		rr.Total = p.liedCount
		return rr, nil
	}
	for i := start; i < uint64(len(p.entries)); i++ {
		index := i
		if p.liedIndex > 0 {
			index = p.liedIndex
		}
		var value api.Record = p.entries[i]
		if hole, ok := p.holes[int(i)]; ok {
			value = hole
		}
		rr.Records = append(rr.Records, &api.ChainEntryRecord[api.Record]{
			Name:  "main",
			Index: index,
			Value: value,
		})
		p.served++
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

// A BVN's root is read from the Directory's pool, and a source pointed at the
// Directory's pool cannot answer for the Directory.
func TestPoolForRoutesByProducer(t *testing.T) {
	pool, err := PoolFor(bvn0(), []string{"BVN0"})
	require.NoError(t, err)
	require.Equal(t, dn().JoinPath(protocol.AnchorPool).String(), pool.String())

	_, err = PoolFor(dn(), nil)
	require.Error(t, err, "with no BVN there is no pool that holds the directory's own anchors")
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

// Only this source's producer has its roots recorded. A source for a BVN
// reads the Directory's pool, which is full of the Directory's own anchors;
// handing those to the BVN's tracker would offer it a root from another
// partition's block numbering, and block numbers collide across partitions
// (#4205).
func TestOnlyTheProducersRootsAreRecorded(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)

	mine := f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 70, root: root(0x70), signers: []int{0, 1, 2}})
	theirs := f.anchor(t, anchorOpts{source: dn(), destination: dn(), block: 70, root: root(0x71), signers: []int{0, 1, 2}})

	var observed []string
	s := sourceOver(t, a, dn().JoinPath(protocol.AnchorPool), bvn0(), mine, theirs)
	s.OnAnchor = func(p *url.URL, block uint64, _ [32]byte) {
		observed = append(observed, p.String())
	}
	require.NoError(t, s.Read(ctx))

	require.Equal(t, []string{bvn0().String()}, observed,
		"a root from another partition's block numbering was handed to this partition's tracker")
	got, err := s.AnchoredRoot(ctx, bvn0(), 70)
	require.NoError(t, err)
	require.Equal(t, root(0x70), got, "the producer's own root at that block")
}

// The backfill window is a stated limit, and a refusal rather than a silence.
func TestAnAnchorOutsideTheBackfillWindowIsNotRead(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)
	pool := dn().JoinPath(protocol.AnchorPool)

	var entries []*api.MessageRecord[messaging.Message]
	for i := 0; i < 6; i++ {
		entries = append(entries, f.anchor(t, anchorOpts{
			source: bvn0(), destination: dn(), block: uint64(80 + i), root: root(byte(0x80 + i)),
			signers: []int{0, 1, 2},
		}))
	}

	s, err := New(&poolQuerier{pool: pool, entries: entries}, pool, bvn0(), a)
	require.NoError(t, err)
	s.Backfill = 2

	_, err = s.AnchoredRoot(ctx, bvn0(), 80)
	require.ErrorIs(t, err, ErrNotAnchored, "an anchor older than the window was read anyway")
	got, err := s.AnchoredRoot(ctx, bvn0(), 85)
	require.NoError(t, err, "the newest anchors are what the window is for")
	require.Equal(t, root(0x85), got)
}

// (F5) One page from one peer cannot park the cursor past the chain.
//
// s.next used to be seeded from the peer's reported Count and advanced from
// the peer's reported Index. One answer claiming index 2^40 put it past the
// end of the real chain, every honest peer thereafter answered NotFound for
// that range, and nothing reset it -- a node that verifies no anchor again
// for the life of the process, from one response, and rotation does not cure
// it because the cursor is not the peer's.
func TestTheCursorCannotBeParkedPastTheChain(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)
	pool := dn().JoinPath(protocol.AnchorPool)

	good := f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: 60, root: root(0x60), signers: []int{0, 1, 2}})
	q := &poolQuerier{pool: pool, entries: []*api.MessageRecord[messaging.Message]{good}}

	s, err := New(q, pool, bvn0(), a)
	require.NoError(t, err)

	// The lying peer answers first: a count and an index from beyond the end
	// of the world.
	q.liedCount = 1 << 40
	q.liedIndex = 1 << 40
	require.NoError(t, s.Read(ctx))
	_, err = s.AnchoredRoot(ctx, bvn0(), 60)
	require.ErrorIs(t, err, ErrNotAnchored, "the liar's page was taken as real")

	// The next peer in the rotation is honest, and the node recovers on its
	// own.
	q.liedCount, q.liedIndex = 0, 0
	got, err := s.AnchoredRoot(ctx, bvn0(), 60)
	require.NoError(t, err, "the cursor stayed parked past the chain after the liar was gone")
	require.Equal(t, root(0x60), got)
}

// (re-check finding 1) A peer whose chain count is behind the one that served
// the last page must not rewind the cursor.
//
// The peers rotate per call (join/sources.go, peerQuerier), so the count and
// the pages come from different nodes, and a count one entry behind is the
// ordinary case rather than an attack. It used to send the cursor a full
// window back and re-verify up to 1024 anchors -- threshold ed25519
// verifications each -- and then do it again next round.
func TestALaggingPeersCountDoesNotRewindTheCursor(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)
	pool := dn().JoinPath(protocol.AnchorPool)

	var entries []*api.MessageRecord[messaging.Message]
	for i := 0; i < 300; i++ {
		entries = append(entries, f.anchor(t, anchorOpts{
			source: bvn0(), destination: dn(), block: uint64(1000 + i),
			root: root(byte(i)), signers: []int{0, 1, 2},
		}))
	}
	q := &poolQuerier{pool: pool, entries: entries}
	s, err := New(q, pool, bvn0(), a)
	require.NoError(t, err)

	require.NoError(t, s.Read(ctx))
	require.Equal(t, 300, q.served, "the first read takes the window once")

	// The next read's count comes from a peer one entry behind.
	q.countBehind = 1
	require.NoError(t, s.Read(ctx))
	require.Equal(t, 300, q.served,
		"a peer one entry behind sent the cursor back a window and the whole window was read again")

	// And the roots are still there; nothing was dropped by not re-reading.
	got, err := s.AnchoredRoot(ctx, bvn0(), 1299)
	require.NoError(t, err)
	require.Equal(t, root(uint8(299%256)), got)
}

// A cursor every peer says is past the end of the chain is re-anchored, a
// window back from the chain's end, as a cold start is (#4384).
//
// The lie-high case has two shapes. A peer that names a window and then
// serves nothing at it is caught by the cursor not being this node's until a
// page comes back. A peer that names a window and SERVES records at it --
// they need not verify; the cursor advances by what was asked for and
// answered -- parks the cursor for real, and only the agreement of the peers
// that follow can say so.
func TestACursorPastEveryPeersChainIsReanchored(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)
	pool := dn().JoinPath(protocol.AnchorPool)

	good := f.anchor(t, anchorOpts{
		source: bvn0(), destination: dn(), block: 900, root: root(0x90), signers: []int{0, 1, 2},
	})
	// What the liar serves at the window it named: an anchor of its own,
	// signed by nobody. It is not recorded -- but a page came back, so the
	// cursor moves and stays moved.
	decoy := f.anchor(t, anchorOpts{
		source: bvn0(), destination: dn(), block: 901, root: root(0x91),
	})
	q := &poolQuerier{pool: pool, entries: []*api.MessageRecord[messaging.Message]{good}, decoy: decoy}
	s, err := New(q, pool, bvn0(), a)
	require.NoError(t, err)

	q.liedCount = 1 << 40
	q.liedIndex = 1 << 40
	require.NoError(t, s.Read(ctx))

	// Every peer after it is honest, and each says the chain is one entry
	// long. The first few are taken for peers that lag -- AnchoredRoot reads
	// too, so this is doubtRounds-1 reads in all.
	q.liedCount, q.liedIndex = 0, 0
	for i := 0; i < doubtRounds-2; i++ {
		require.NoError(t, s.Read(ctx))
	}
	_, err = s.AnchoredRoot(ctx, bvn0(), 900)
	require.ErrorIs(t, err, ErrNotAnchored,
		"peers short of doubtRounds disagreeing with the cursor were enough to move it")
	got, err := s.AnchoredRoot(ctx, bvn0(), 900)
	require.NoError(t, err, "the cursor stayed parked although every peer said it was past the end")
	require.Equal(t, root(0x90), got)
}

// (re-check finding 2) Another partition's anchor is not this source's, and
// it is not refused either -- it is never verified at all.
//
// A pool holds every partition's anchors. Verifying the rest costs a full
// signature check per anchor for a root nobody here wants, and a partition
// that has not yet executed a network update signs under the older version,
// fails this node's floor, and used to be logged as an anchor refused -- read
// as a finding by the soak checklist (D1).
func TestAnotherPartitionsAnchorIsNeitherVerifiedNorRefused(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)
	pool := dn().JoinPath(protocol.AnchorPool)

	mine := f.anchor(t, anchorOpts{
		source: bvn0(), destination: dn(), block: 700, root: root(0x70), signers: []int{0, 1, 2},
	})
	// The Directory's own anchor, in the Directory's pool, signed by nobody:
	// it would fail verification loudly if this source looked at it.
	theirs := f.anchor(t, anchorOpts{
		source: dn(), destination: dn(), block: 701, root: root(0x71),
	})

	var refused []uint64
	s := sourceOver(t, a, pool, bvn0(), mine, theirs)
	s.OnRefused = func(block uint64, _ error) { refused = append(refused, block) }
	require.NoError(t, s.Read(ctx))

	require.Empty(t, refused,
		"another partition's anchor was verified and its failure logged as a refusal")
	got, err := s.AnchoredRoot(ctx, bvn0(), 700)
	require.NoError(t, err)
	require.Equal(t, root(0x70), got)
}

// Rewind makes the source read its window again.
//
// The join calls it when the trusted sets move, and this is the case it is
// for: a network that ADDS validators produces anchors the new set signs,
// and those do not reach the old set's threshold. Under the old definition
// such an anchor is refused and the cursor moves past it, so without a
// rewind the roots in flight across the change are lost and the node waits
// for anchors that come after it.
func TestRewindReadsTheWindowAgain(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1, 2) // four seated, two the network has not seated
	pool := dn().JoinPath(protocol.AnchorPool)

	// Signed by two of the old set and the two the change seats: two of six
	// under the old definition, where three of four are needed.
	entry := f.anchor(t, anchorOpts{
		source: bvn0(), destination: dn(), block: 500, root: root(0x50),
		signers: []int{0, 1, 4, 5}, version: 2,
	})
	q := &poolQuerier{pool: pool, entries: []*api.MessageRecord[messaging.Message]{entry}}

	a := f.authority(t)
	s, err := New(q, pool, bvn0(), a)
	require.NoError(t, err)

	_, err = s.AnchoredRoot(ctx, bvn0(), 500)
	require.ErrorIs(t, err, ErrNotAnchored, "the old set's threshold was reached by keys it does not name")

	// Reading again takes nothing: the cursor has moved past it.
	served := q.served
	require.NoError(t, s.Read(ctx))
	require.Equal(t, served, q.served, "the cursor did not move past what it read")

	// The node adopts the new definition out of verified state, which is the
	// only way the sets ever move, and the window is read again.
	require.True(t, a.Update(f.withActive(t, 2, 0, 1, 2, 3, 4, 5)))
	set, err := a.SetFor("BVN0")
	require.NoError(t, err)
	require.Equal(t, uint64(4), set.Threshold, "2/3 of six is four")

	s.Rewind()
	got, err := s.AnchoredRoot(ctx, bvn0(), 500)
	require.NoError(t, err, "the anchor refused under the old set was never read again")
	require.Equal(t, root(0x50), got)
}

// ringQuerier is peerQuerier's rotation (join/sources.go): each call starts
// at the next peer and takes the first answer that is not an error. A peer
// that serves a page, however bare, answers the call.
type ringQuerier struct {
	peers []api.Querier
	next  int

	pages int      // page calls made of the ring, however many peers each tried
	asked []string // the peers the last call asked, as peerQuerier names them
}

func (r *ringQuerier) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	if cq, ok := q.(*api.ChainQuery); ok && cq.Range != nil {
		r.pages++
	}
	start := r.next
	r.next++
	r.asked = r.asked[:0]
	var last error
	for i := range r.peers {
		n := (start + i) % len(r.peers)
		r.asked = append(r.asked, fmt.Sprintf("peer%d", n))
		rec, err := r.peers[n].Query(ctx, scope, q)
		if err == nil {
			return rec, nil
		}
		last = err
	}
	return nil, last
}

// PeerCount and LastAsked are what join's peerQuerier says about itself.
func (r *ringQuerier) PeerCount(context.Context) int { return len(r.peers) }
func (r *ringQuerier) LastAsked() []string           { return append([]string(nil), r.asked...) }

// paged serves at most the page asked for, as the API does; poolQuerier
// serves everything from the start.
type paged struct{ *poolQuerier }

func (p paged) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	rec, err := p.poolQuerier.Query(ctx, scope, q)
	if err != nil {
		return nil, err
	}
	cq := q.(*api.ChainQuery)
	if rr, ok := rec.(*api.RecordRange[api.Record]); ok && cq.Range != nil && cq.Range.Count != nil && uint64(len(rr.Records)) > *cq.Range.Count {
		rr.Records = rr.Records[:*cq.Range.Count]
	}
	return rec, nil
}

// (#4413) An anchor a peer serves without its signatures does not move the
// cursor past it: the next peer is asked for it, and no root is lost.
//
// The peer is a node that joined by pull: it executed the blocks before its
// restart and holds their anchors signed, and it holds the range it pulled as
// bodies with no signatures behind them (#4416). The cursor used to move past
// that range on the page that bare peer served, and the roots in it were never
// read again this join. In a ring of two, and the peers rotating per call, the
// bare peer serves every page when it is second: the count comes from the one,
// the page from the other.
func TestAnAnchorServedWithoutSignaturesIsAskedOfTheNextPeer(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)
	pool := dn().JoinPath(protocol.AnchorPool)

	const n, pulledFrom = 40, 10
	var signed, bare []*api.MessageRecord[messaging.Message]
	for i := 0; i < n; i++ {
		rec := f.anchor(t, anchorOpts{
			source: bvn0(), destination: dn(), block: uint64(100 + i),
			root: root(byte(i)), signers: []int{0, 1, 2},
		})
		signed = append(signed, rec)
		if i < pulledFrom {
			bare = append(bare, rec)
			continue
		}
		stripped := *rec
		stripped.Signatures = new(api.RecordRange[*api.SignatureSetRecord])
		bare = append(bare, &stripped)
	}

	for _, bareFirst := range []bool{true, false} {
		honest := &poolQuerier{pool: pool, entries: signed}
		joined := &poolQuerier{pool: pool, entries: bare}
		ring := &ringQuerier{peers: []api.Querier{paged{honest}, paged{joined}}}
		if bareFirst {
			ring.peers = []api.Querier{paged{joined}, paged{honest}}
		}
		s, err := New(ring, pool, bvn0(), a)
		require.NoError(t, err)
		s.PageSize = 8

		observed := map[uint64]bool{}
		var refused int
		s.OnAnchor = func(_ *url.URL, block uint64, _ [32]byte) { observed[block] = true }
		s.OnRefused = func(uint64, error) { refused++ }
		require.NoError(t, s.Read(ctx))

		t.Logf("bare peer first=%v: %d of %d roots observed, %d bare anchors refused", bareFirst, len(observed), n, refused)
		for i := 0; i < n; i++ {
			require.True(t, observed[uint64(100+i)],
				"bare peer first=%v: the root for block %d was skipped; the cursor moved past an anchor served without signatures", bareFirst, 100+i)
			got, err := s.AnchoredRoot(ctx, bvn0(), uint64(100+i))
			require.NoError(t, err)
			require.Equal(t, root(byte(i)), got)
		}
	}
}

// (#4418) An entry a peer serves without its body -- an ErrorRecord in the
// body's place, as the API serves a message it does not hold, or no value at
// all -- is that peer's gap, not a fact about the pool. The cursor stops in
// front of it, the next peer is asked, and the refusal is said.
//
// The reviewer's ring R4 (#4413 note_3896860371, F1): one peer whose page has
// entry 12 (block 112) without its body, an honest peer beside it. It used to
// read 39 of 40 roots with the holed peer first, block 112 lost and nothing
// refused.
func TestAnEntryServedWithoutItsBodyIsAskedOfTheNextPeer(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)
	pool := dn().JoinPath(protocol.AnchorPool)

	const n, hole = 40, 12
	var signed []*api.MessageRecord[messaging.Message]
	for i := 0; i < n; i++ {
		signed = append(signed, f.anchor(t, anchorOpts{
			source: bvn0(), destination: dn(), block: uint64(100 + i),
			root: root(byte(i)), signers: []int{0, 1, 2},
		}))
	}

	holes := []struct {
		kind   string
		served api.Record
	}{
		{"error record", &api.ErrorRecord{Value: errors.NotFound.With("message not found")}},
		{"no value", nil},
	}
	for _, h := range holes {
		kind, served := h.kind, h.served
		for _, holedFirst := range []bool{true, false} {
			honest := &poolQuerier{pool: pool, entries: signed}
			holed := &poolQuerier{pool: pool, entries: signed, holes: map[int]api.Record{hole: served}}
			ring := &ringQuerier{peers: []api.Querier{paged{honest}, paged{holed}}}
			if holedFirst {
				ring.peers = []api.Querier{paged{holed}, paged{honest}}
			}
			s, err := New(ring, pool, bvn0(), a)
			require.NoError(t, err)
			s.PageSize = 8

			observed := map[uint64]bool{}
			var refusals []string
			s.OnAnchor = func(_ *url.URL, block uint64, _ [32]byte) { observed[block] = true }
			s.OnRefused = func(_ uint64, err error) { refusals = append(refusals, err.Error()) }
			require.NoError(t, s.Read(ctx))

			t.Logf("%s, holed peer first=%v: %d of %d roots observed, %d refusals %q",
				kind, holedFirst, len(observed), n, len(refusals), refusals)
			require.Len(t, observed, n, "%s, holed peer first=%v: a root was skipped", kind, holedFirst)
			require.True(t, observed[100+hole], "%s, holed peer first=%v: block %d was skipped on one peer's gap", kind, holedFirst, 100+hole)
			if holedFirst {
				require.NotEmpty(t, refusals, "%s: the holed peer's gap was passed over without a word", kind)
			}
		}
	}
}

// notReadyPaged is a current node that holds the anchors from `from` on
// without their signatures: it fails any page that reaches one of them with
// NotReady, whole, as internal/api/v3's servedSigned does (#4413).
type notReadyPaged struct {
	*poolQuerier
	from uint64
}

func (p notReadyPaged) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	if cq, ok := q.(*api.ChainQuery); ok && cq.Range != nil && cq.Range.Count != nil &&
		cq.Range.Start+*cq.Range.Count > p.from && cq.Range.Start < uint64(len(p.entries)) {
		return nil, errors.NotReady.WithFormat("this node holds anchor %d without its signatures and cannot serve it", p.from)
	}
	return paged{p.poolQuerier}.Query(ctx, scope, q)
}

// stallFixture is 40 anchors of BVN0, blocks 100-139, signed, and the same
// anchors with 10 onward stripped of their signatures: what a node that
// joined by pull at entry 10 holds.
func stallFixture(t *testing.T) (*Authority, *url.URL, []*api.MessageRecord[messaging.Message], []*api.MessageRecord[messaging.Message]) {
	t.Helper()
	f := newNet(t, 4, 1)
	var signed, bare []*api.MessageRecord[messaging.Message]
	for i := 0; i < 40; i++ {
		rec := f.anchor(t, anchorOpts{
			source: bvn0(), destination: dn(), block: uint64(100 + i),
			root: root(byte(i)), signers: []int{0, 1, 2},
		})
		signed = append(signed, rec)
		if i < 10 {
			bare = append(bare, rec)
			continue
		}
		stripped := *rec
		stripped.Signatures = new(api.RecordRange[*api.SignatureSetRecord])
		bare = append(bare, &stripped)
	}
	return f.authority(t), dn().JoinPath(protocol.AnchorPool), signed, bare
}

// (#4419a) A ring in which no peer can serve an entry signed costs one page
// call per peer per read for that entry, not the read's whole budget.
//
// Measured by the reviewer of #4413 (F2, ring R1): 64 page calls and 63-64
// refusals every read, every 2 s, for as long as the stall lasts.
func TestARingThatCannotServeAnEntryIsAskedOncePerPeer(t *testing.T) {
	ctx := context.Background()
	a, pool, _, bare := stallFixture(t)

	for _, peers := range []int{1, 3} {
		ring := &ringQuerier{}
		for i := 0; i < peers; i++ {
			ring.peers = append(ring.peers, paged{&poolQuerier{pool: pool, entries: bare}})
		}
		s, err := New(ring, pool, bvn0(), a)
		require.NoError(t, err)
		s.PageSize = 8

		observed := map[uint64]bool{}
		var refused int
		s.OnAnchor = func(_ *url.URL, block uint64, _ [32]byte) { observed[block] = true }
		s.OnRefused = func(uint64, error) { refused++ }

		require.NoError(t, s.Read(ctx))
		t.Logf("%d bare peers, read 1: %d roots observed, %d page calls, %d refusals", peers, len(observed), ring.pages, refused)
		require.Len(t, observed, 10, "the signed prefix is read")
		// Two pages reach entry 10 (0-7, then 8-15 held at 10); every peer is
		// then asked for it once.
		require.LessOrEqual(t, ring.pages, 1+peers, "%d bare peers, read 1", peers)

		for read := 2; read <= 3; read++ {
			ring.pages, refused = 0, 0
			require.NoError(t, s.Read(ctx))
			t.Logf("%d bare peers, read %d: %d page calls, %d refusals", peers, read, ring.pages, refused)
			require.Equal(t, peers, ring.pages, "%d bare peers, read %d: one page call per peer for the held entry", peers, read)
			require.Equal(t, peers, refused, "%d bare peers, read %d: one refusal per peer's answer", peers, read)
		}
	}
}

// (#4419b) A page a peer fails whole with NotReady pins the cursor at the
// first entry it cannot serve, not at the page's start: the entries before it
// are served signed by the same peer and are read.
//
// The reviewer of #4413 (F3, ring R5): page size 8, the entries from 10 on
// held unsigned; the read stopped at 8, and 8 and 9 were not read although
// every peer serves them signed.
func TestANotReadyPagePinsTheCursorAtTheFirstBadEntry(t *testing.T) {
	ctx := context.Background()
	a, pool, _, bare := stallFixture(t)

	ring := &ringQuerier{peers: []api.Querier{
		notReadyPaged{&poolQuerier{pool: pool, entries: bare}, 10},
		notReadyPaged{&poolQuerier{pool: pool, entries: bare}, 10},
	}}
	s, err := New(ring, pool, bvn0(), a)
	require.NoError(t, err)
	s.PageSize = 8

	observed := map[uint64]bool{}
	var calls int
	s.OnAnchor = func(_ *url.URL, block uint64, _ [32]byte) { observed[block] = true; calls++ }
	err = s.Read(ctx)
	t.Logf("read 1: %d roots observed (%d times), %d page calls, err=%v", len(observed), calls, ring.pages, err)
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "%v", err)
	require.Len(t, observed, 10, "entries 8 and 9 are served signed and were not read")
	for i := 0; i < 10; i++ {
		require.True(t, observed[uint64(100+i)], "block %d", 100+i)
	}

	// The narrowing never rewinds (#4384; review F3): each root is read
	// once, the pages are 0-7, 8-15 (refused), 8-11 (refused), 8-9, 10-11
	// (refused) and 10 alone (refused), and the next read asks for 10 again
	// and nothing before it.
	require.Equal(t, 10, calls, "a root was read twice: the narrowing moved the cursor back")
	require.Equal(t, 6, ring.pages, "read 1: 0-7, 8-15, 8-11, 8-9, 10-11, 10")
	ring.pages, calls = 0, 0
	require.Error(t, s.Read(ctx))
	t.Logf("read 2: %d roots re-observed, %d page calls", calls, ring.pages)
	require.Zero(t, calls, "read 2 read a root before the entry it is held at")
	require.Equal(t, 4, ring.pages, "read 2: 10-17, 10-13, 10-11, 10")
	st, ok := s.Stalled()
	require.True(t, ok)
	require.Equal(t, uint64(10), st.Entry)
}

// (#4419c) A read held at an entry says so: which entry, which peers were
// asked for it, and why the last answer was not taken. It used to return nil
// with nothing to show that the join had stopped. The stall ends when a peer
// serves the entry.
func TestAStalledReadSaysWhereAndWhoWasAsked(t *testing.T) {
	ctx := context.Background()
	a, pool, signed, bare := stallFixture(t)

	for _, notReady := range []bool{false, true} {
		var peer func() api.Querier
		if notReady {
			peer = func() api.Querier { return notReadyPaged{&poolQuerier{pool: pool, entries: bare}, 10} }
		} else {
			peer = func() api.Querier { return paged{&poolQuerier{pool: pool, entries: bare}} }
		}
		ring := &ringQuerier{peers: []api.Querier{peer(), peer(), peer()}}
		s, err := New(ring, pool, bvn0(), a)
		require.NoError(t, err)
		s.PageSize = 8

		_, ok := s.Stalled()
		require.False(t, ok, "nothing read, nothing held")

		for read := 1; read <= 2; read++ {
			_ = s.Read(ctx)
			st, ok := s.Stalled()
			require.True(t, ok, "notReady=%v read %d: the read is held at entry 10 and does not say so", notReady, read)
			t.Logf("notReady=%v read %d: stalled at entry %d, asked %v: %v", notReady, read, st.Entry, st.Asked, st.Err)
			require.Equal(t, uint64(10), st.Entry, "notReady=%v", notReady)
			require.ElementsMatch(t, []string{"peer0", "peer1", "peer2"}, st.Asked, "notReady=%v: every peer is asked, once", notReady)
			require.Error(t, st.Err)
		}

		// A peer that holds the entry signed ends the stall.
		observed := map[uint64]bool{}
		s.OnAnchor = func(_ *url.URL, block uint64, _ [32]byte) { observed[block] = true }
		ring.peers = append(ring.peers, paged{&poolQuerier{pool: pool, entries: signed}})
		for read := 0; read < 4 && len(observed) < 30; read++ {
			_ = s.Read(ctx)
		}
		_, ok = s.Stalled()
		require.False(t, ok, "notReady=%v: the entry was served and the stall still reads", notReady)
		require.Len(t, observed, 30, "notReady=%v: blocks 110-139", notReady)
	}
}

// (#4419, review F1) The stall ends wherever the cursor passes its entry, not
// only on a page taken whole. Two peers that joined by pull with
// complementary holes (#4400's shape, twice) hold every page at a hole and
// the other serves it: the read advances on held pages alone, and it used to
// end with the cursor 65 past the entry the stall still named.
func TestAStallEndsWhenTheCursorPassesItOnHeldPages(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)
	pool := dn().JoinPath(protocol.AnchorPool)

	const n = 120
	var signed, bare []*api.MessageRecord[messaging.Message]
	for i := 0; i < n; i++ {
		rec := f.anchor(t, anchorOpts{
			source: bvn0(), destination: dn(), block: uint64(1000 + i),
			root: root(byte(i)), signers: []int{0, 1, 2},
		})
		signed = append(signed, rec)
		stripped := *rec
		stripped.Signatures = new(api.RecordRange[*api.SignatureSetRecord])
		if i < 10 {
			bare = append(bare, rec)
		} else {
			bare = append(bare, &stripped)
		}
	}
	odd, even := map[int]api.Record{}, map[int]api.Record{}
	for i := 11; i < n; i++ {
		hole := &api.ErrorRecord{Value: errors.NotFound.With("message not found")}
		if i%2 == 1 {
			odd[i] = hole
		} else {
			even[i] = hole
		}
	}

	// Read 1: every peer serves entry 10 bare. Held at 10.
	ring := &ringQuerier{peers: []api.Querier{
		paged{&poolQuerier{pool: pool, entries: bare}},
		paged{&poolQuerier{pool: pool, entries: bare}},
	}}
	s, err := New(ring, pool, bvn0(), a)
	require.NoError(t, err)
	s.PageSize = 8
	observed := map[uint64]bool{}
	s.OnAnchor = func(_ *url.URL, block uint64, _ [32]byte) { observed[block] = true }
	require.NoError(t, s.Read(ctx))
	st, ok := s.Stalled()
	require.True(t, ok)
	require.Equal(t, uint64(10), st.Entry)

	// Read 2: the peers now hold 10 signed and have complementary holes
	// from 11 on. Every page is held at a hole; the read advances anyway.
	ring.peers = []api.Querier{
		paged{&poolQuerier{pool: pool, entries: signed, holes: odd}},
		paged{&poolQuerier{pool: pool, entries: signed, holes: even}},
	}
	require.NoError(t, s.Read(ctx))
	var last uint64
	for b := range observed {
		if b > last {
			last = b
		}
	}
	st, ok = s.Stalled()
	t.Logf("read 2: roots through block %d (entry %d), %d page calls; stalled=%v at entry %d", last, last-1000, ring.pages, ok, st.Entry)
	require.Greater(t, last, uint64(1010), "precondition: the read moved past entry 10")
	if ok {
		require.Greater(t, st.Entry, last-1000,
			"the stall names entry %d while the cursor has read through entry %d", st.Entry, last-1000)
	}
}
