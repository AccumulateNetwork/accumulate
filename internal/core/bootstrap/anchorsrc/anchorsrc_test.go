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
		if p.liedCount > 0 {
			count = p.liedCount
		}
		return &api.ChainRecord{Name: "main", Count: count}, nil
	}
	start := cq.Range.Start
	rr := new(api.RecordRange[api.Record])
	for i := start; i < uint64(len(p.entries)); i++ {
		index := i
		if p.liedIndex > 0 {
			index = p.liedIndex
		}
		rr.Records = append(rr.Records, &api.ChainEntryRecord[api.Record]{
			Name:  "main",
			Index: index,
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

// (d) The walk stands at one set: the carrier of a change is accepted
// although it declares the version it INTRODUCES — which is how production
// signs it — the set is replaced, and what it replaced stops being a signer.
func TestTheWalkStandsAtOneSet(t *testing.T) {
	ctx := context.Background()
	pool := bvn0().JoinPath(protocol.AnchorPool)

	// Four seated validators and four the network knows but has not seated.
	newFixture := func() *netFixture { return newNet(t, 4, 1, 4) }

	t.Run("the carrier is signed under the version it introduces, and is accepted", func(t *testing.T) {
		// This is the timing, traced through production: the executor
		// publishes the new globals at the close of the block that executed
		// the change (block_end.go:421-432), the conductor stores them
		// (conductor.go:183), and THAT block's anchor goes out at the start
		// of the next one with SignerVersion taken from the new definition
		// (conductor.go:261,346 -> anchoring.go:146). A verifier that
		// insisted on "the set of the time" would refuse the one anchor the
		// walk exists to accept and would never move again (#4301, threat
		// review F4).
		f := newFixture()
		a := f.authority(t)
		v2 := f.withActive(t, 2, 0, 1, 2) // validator 3 stands down

		carrier := f.anchor(t, anchorOpts{
			source: dn(), destination: bvn0(), block: 20, root: root(0x20),
			signers: []int{0, 1, 2}, version: 2, // the NEW version, as production signs it
			updates: []protocol.NetworkAccountUpdate{update(v2)},
		})
		s := sourceOver(t, a, pool, dn(), carrier)

		got, err := s.AnchoredRoot(ctx, dn(), 20)
		require.NoError(t, err)
		require.Equal(t, root(0x20), got)
		require.Equal(t, uint64(2), a.Version(), "the walk did not take the change it verified")

		set, err := a.SetFor(protocol.Directory)
		require.NoError(t, err)
		require.Equal(t, 3, set.Size())
		require.Equal(t, uint64(2), set.Threshold, "2/3 of three is two")
		require.False(t, set.MaySign(hashOf(f.keys[3][32:])), "the validator that stood down is not a signer")
	})

	t.Run("a superseded quorum cannot sign a new root", func(t *testing.T) {
		// The whole seat is turned over: 0-3 out, 4-7 in. An attacker with a
		// threshold of the retired keys -- retired, commonly, because they
		// were compromised -- must not be able to sign anything, whatever
		// version their signature declares (#4301, threat review F3).
		f := newFixture()
		a := f.authority(t)
		v2 := f.withActive(t, 2, 4, 5, 6, 7)

		carrier := f.anchor(t, anchorOpts{
			source: dn(), destination: bvn0(), block: 30, root: root(0x30),
			signers: []int{0, 1, 2}, version: 2,
			updates: []protocol.NetworkAccountUpdate{update(v2)},
		})
		// Signed by a full quorum of the RETIRED set, declaring the version
		// they were seated under.
		byRetired := f.anchor(t, anchorOpts{
			source: dn(), destination: bvn0(), block: 31, root: root(0x31),
			signers: []int{0, 1, 2}, version: 1,
		})
		// And a definition of their own, to take the network back.
		theirs := f.withActive(t, 3, 0, 1, 2, 3)
		byRetiredWithUpdate := f.anchor(t, anchorOpts{
			source: dn(), destination: bvn0(), block: 32, root: root(0x32),
			signers: []int{0, 1, 2}, version: 1,
			updates: []protocol.NetworkAccountUpdate{update(theirs)},
		})

		s := sourceOver(t, a, pool, dn(), carrier, byRetired, byRetiredWithUpdate)
		require.NoError(t, s.Read(ctx))
		require.Equal(t, uint64(2), a.Version(), "a retired quorum named the validators")

		_, err := s.AnchoredRoot(ctx, dn(), 31)
		require.ErrorIs(t, err, ErrNotAnchored, "a retired quorum signed a new root")
		_, err = s.AnchoredRoot(ctx, dn(), 32)
		require.ErrorIs(t, err, ErrNotAnchored)
	})

	t.Run("a retired key does not fill a current threshold", func(t *testing.T) {
		// {k0,k1,k2,k3}/3 becomes {k0,k1,k2}/2. An anchor carrying k3's
		// signature and k0's reaches two -- which is what the new set means
		// TWO CURRENT KEYS by -- if signatures from different sets are
		// pooled against one threshold (#4301, threat review F2).
		f := newFixture()
		a := f.authority(t)
		v2 := f.withActive(t, 2, 0, 1, 2)

		carrier := f.anchor(t, anchorOpts{
			source: dn(), destination: bvn0(), block: 40, root: root(0x40),
			signers: []int{0, 1, 2}, version: 2,
			updates: []protocol.NetworkAccountUpdate{update(v2)},
		})
		mixed := f.anchor(t, anchorOpts{
			source: dn(), destination: bvn0(), block: 41, root: root(0x41),
			signers: []int{0, 3}, // one current, one retired
		})

		s := sourceOver(t, a, pool, dn(), carrier, mixed)
		require.NoError(t, s.Read(ctx))
		require.Equal(t, uint64(2), a.Version())

		_, err := s.AnchoredRoot(ctx, dn(), 41)
		require.ErrorIs(t, err, ErrNotAnchored,
			"one current key and one retired key reached a threshold of two current keys")
	})
}

// (F1) Only the Directory carries an operator change.
//
// A BVN's quorum is three keys of four in the soak topology; the Directory's
// is eight of twelve, and every container's single key is a validator of
// both. So a quorum of one BVN is a quarter of what naming the validators
// should cost -- and before this test it bought all of it, for every
// partition, on any peer willing to serve the anchor. The executor refuses
// the same thing at execute/v2/chain/directory_anchor.go:40.
func TestOnlyTheDirectoryCarriesAnOperatorChange(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1, 4)
	a := f.authority(t)

	// The attacker's definition: their four spare keys, seated everywhere.
	theirs := f.withActive(t, 2, 4, 5, 6, 7)

	// Signed by a full quorum of BVN0's validators, in a BlockValidatorAnchor
	// that is otherwise perfectly valid, and served in the Directory's pool
	// -- which is what a BVN's join reads.
	forged := f.anchor(t, anchorOpts{
		source: bvn0(), destination: dn(), block: 50, root: root(0x50),
		signers: []int{0, 1, 2},
	})
	// And the same with the body a DirectoryAnchor, still sourced at the BVN.
	forgedDir := f.anchor(t, anchorOpts{
		source: bvn0(), destination: dn(), block: 51, root: root(0x51),
		signers: []int{0, 1, 2}, forceDirBody: true,
		updates: []protocol.NetworkAccountUpdate{update(theirs)},
	})

	s := sourceOver(t, a, dn().JoinPath(protocol.AnchorPool), bvn0(), forged, forgedDir)
	require.NoError(t, s.Read(ctx))

	require.Equal(t, uint64(1), a.Version(),
		"a BVN's quorum named the validators of every partition")
	set, err := a.SetFor(protocol.Directory)
	require.NoError(t, err)
	require.False(t, set.MaySign(hashOf(f.keys[4][32:])), "the attacker's key is a directory signer")

	// The BVN's own root is still recorded: the anchor was valid for what a
	// BVN's quorum may say, which is its own state and not who the
	// validators are.
	got, err := s.AnchoredRoot(ctx, bvn0(), 51)
	require.NoError(t, err)
	require.Equal(t, root(0x51), got)
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

// An authority walks only on a VERIFIED anchor: a change carried by an anchor
// nobody signed is not a change.
func TestAnUnverifiedAnchorDoesNotWalkTheSet(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)
	a := f.authority(t)

	v2 := f.withActive(t, 2, 0, 1, 2)
	carrier := f.anchor(t, anchorOpts{
		source: dn(), destination: bvn0(), block: 30, root: root(0x30),
		updates: []protocol.NetworkAccountUpdate{update(v2)},
	}) // no signers

	s := sourceOver(t, a, bvn0().JoinPath(protocol.AnchorPool), dn(), carrier)
	require.NoError(t, s.Read(ctx))
	require.Equal(t, uint64(1), a.Version(), "one peer must not be able to name the validators")
}
