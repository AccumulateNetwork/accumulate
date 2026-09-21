// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// namedPeers is the join's Sources as the daemon builds it: a client whose
// dialer resolves a peer's query service, and a router. Nothing here is a
// direct handle on the network's services -- a handle answers every
// partition from one object, which is exactly the wiring a Directory node
// reading a BVN's pool has to get right (#4301).
//
// **`Self` is empty, and that is not the daemon's case.** The daemon passes
// the node's own peer ID so `selectPeers` can drop it, because a joining
// node in production serves the partition it is joining and would otherwise
// read its own un-executed store (#4303). The node in these tests is not one
// of the simulator's peers, so there is no ID to drop and this wiring cannot
// exercise that guard; `TestSelectPeers_NeverThisNode` is what does.
func namedPeers(t *testing.T, sim *Sim) *join.QueryPeers {
	t.Helper()
	return &join.QueryPeers{
		Client:  sim.S.Services(),
		Network: t.Name(),
		Router:  sim.S.Router(),
	}
}

// theDirectorysPool is the pool a Directory node's join really reads, chosen
// the way the join chooses it. Pinning a rewriter to "BVN0/anchors" instead
// makes a test that cannot tell a routing change from a passing run: the
// rewriter stops firing, the node promotes on untouched anchors, and the
// first assertion to speak says "the node promoted".
func theDirectorysPool(t *testing.T, sim *Sim) *url.URL {
	t.Helper()
	a, err := anchorsrc.FromStore(sim.S.Database(Directory), DnUrl())
	require.NoError(t, err)
	pool, err := anchorsrc.PoolFor(DnUrl(), a.BvnNames())
	require.NoError(t, err)
	return pool
}

// genesisOf copies the network accounts a node holds from its own genesis
// into an empty store. This is the node's OWN copy -- what says who may sign
// an anchor -- and it is the one thing a join must not take from a peer.
//
// It is not the daemon's genesis snapshot; a join from a restored snapshot is
// a gap this branch names and does not close.
func genesisOf(t *testing.T, sim *Sim, partition string) *database.Database {
	t.Helper()
	part := PartitionUrl(partition)
	db := emptyDb()
	batch := db.Begin(true)
	defer batch.Discard()
	for _, name := range []string{Network, Globals} {
		u := part.JoinPath(name)
		var acct Account
		require.NoError(t, sim.S.Database(partition).View(func(b *database.Batch) error {
			return b.Account(u).Main().GetAs(&acct)
		}))
		require.NoError(t, batch.Account(u).Main().Put(acct))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	return db
}

// spineNetwork is the shape every test here runs on: bvns BVNs of nodes
// nodes, some traffic, and the anchors settled.
func spineNetwork(t *testing.T, nodes int) *Sim {
	t.Helper()
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, nodes),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	for i := 0; i < 2; i++ {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	return sim
}

// joinTheDirectory builds a Directory node with nothing but its own genesis
// network accounts and drives it through the real join for up to rounds
// rounds, reporting the block it matched at (zero for never).
func joinTheDirectory(t *testing.T, sim *Sim, sources join.Sources, rounds int) (*join.PulledState, *database.Database, uint64) {
	t.Helper()
	local := genesisOf(t, sim, Directory)
	t.Cleanup(func() { _ = local.Close() })
	state, block := joinTheDirectoryFrom(t, sim, sources, local, rounds)
	return state, local, block
}

// joinTheDirectoryFrom is the same from a store the caller seeded, so a test
// can seed one BEFORE the network changes under it.
func joinTheDirectoryFrom(t *testing.T, sim *Sim, sources join.Sources, local *database.Database, rounds int) (*join.PulledState, uint64) {
	t.Helper()
	ctx := context.Background()

	state, err := join.NewState(join.StateOptions{
		Partition:     DnUrl(),
		Database:      local,
		Sources:       sources,
		ExecutedBlock: 1,
	})
	require.NoError(t, err)

	for i := 0; i < rounds; i++ {
		require.NoError(t, state.Pull(ctx))
		block, ok, err := state.Matched(ctx)
		require.NoError(t, err)
		if ok {
			return state, block
		}
		sim.Step()
	}
	return state, 0
}

// requireNoSpineKept says the node wrote none of the four accounts a join
// takes first. They used to be settled with Keep, unverified, which made the
// accounts a joining node needs most the four it took on a peer's word
// (#4301).
func requireNoSpineKept(t *testing.T, local *database.Database) {
	t.Helper()
	View(t, local, func(batch *database.Batch) {
		for _, u := range []*url.URL{
			DnUrl().JoinPath(AnchorPool),
			DnUrl().JoinPath(Ledger),
			DnUrl().JoinPath(Operators),
			DnUrl().JoinPath(Operators, "1"),
		} {
			_, err := batch.Account(u).Main().Get()
			require.Error(t, err, "%v was written from a peer nobody could verify", u)
		}
	})
}

// TestTheDirectorysOwnRootIsObtainedThroughTheJoin is (e) of #4301.
//
// **The Directory's own root lives in a BVN's anchor pool.** A produced
// anchor is on the RECEIVING partition's pool, so `anchorsrc.PoolFor` points
// the Directory's join at `bvn-X.acme/anchors`, and this is the test that it
// can read it there and verify what it finds.
//
// Nothing about the source is built by hand. join.NewState chooses the pool,
// reads the validator sets out of the node's own store, builds the source and
// verifies the signatures; the reads go through join.QueryPeers, which looks
// the partition's query service up and addresses one peer at a time -- so the
// Directory node really does reach a BVN peer's service for its own roots.
// The anchors are the simulator's, signed by the real crosschain conductor.
//
// It runs on three nodes, so the Directory's quorum is two and not one.
func TestTheDirectorysOwnRootIsObtainedThroughTheJoin(t *testing.T) {
	sim := spineNetwork(t, 3)
	_, _, matched := joinTheDirectory(t, sim, namedPeers(t, sim), 80)
	require.NotZero(t, matched,
		"the Directory never matched a root it verified: its own anchors are in a BVN's pool, "+
			"and a join that cannot read them there can never obtain the Directory's root (#4301)")
	t.Logf("the Directory's state matched the root anchored for its block %d", matched)
}

// rewritePool is a peer that changes what it serves out of one anchor pool
// and is otherwise honest -- the true anchors, the true roots, the true
// state, the true receipts. It wraps a real Sources, so the reads still go
// out through QueryPeers to a named peer's query service.
type rewritePool struct {
	inner   join.Sources
	pool    *url.URL
	rewrite func(*api.MessageRecord[messaging.Message]) bool
	touched *int
}

func (r rewritePool) For(ctx context.Context, account *url.URL) ([]pull.Source, *url.URL, error) {
	return r.inner.For(ctx, account)
}

func (r rewritePool) Querier(partition *url.URL) api.Querier {
	return rewriteQuerier{inner: r.inner.Querier(partition), owner: r}
}

type rewriteQuerier struct {
	inner api.Querier
	owner rewritePool
}

func (r rewriteQuerier) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	rec, err := r.inner.Query(ctx, scope, q)
	if err != nil || !scope.Equal(r.owner.pool) {
		return rec, err
	}
	rr, ok := rec.(*api.RecordRange[api.Record])
	if !ok {
		return rec, nil
	}
	for _, x := range rr.Records {
		ce, ok := x.(*api.ChainEntryRecord[api.Record])
		if !ok {
			continue
		}
		mr, ok := ce.Value.(*api.MessageRecord[messaging.Message])
		if !ok {
			continue
		}
		if r.owner.rewrite(mr) {
			*r.owner.touched++
		}
	}
	return rr, nil
}

// TestAPeerConsistentWithItselfDoesNotPromoteTheNode is (b) of #4301.
//
// The peer serves the true anchors, the true roots and the true state, and
// only the signatures are missing. Before this issue the join recorded every
// StateTreeAnchor it could decode out of the pool and handed it to the
// tracker, so the root the whole pull was verified against was a number the
// peer sent -- and the scheme proved only that the peer agreed with itself.
func TestAPeerConsistentWithItselfDoesNotPromoteTheNode(t *testing.T) {
	sim := spineNetwork(t, 3)
	touched := 0
	sources := rewritePool{
		inner:   namedPeers(t, sim),
		pool:    theDirectorysPool(t, sim),
		touched: &touched,
		rewrite: func(mr *api.MessageRecord[messaging.Message]) bool {
			if mr.Signatures == nil || len(mr.Signatures.Records) == 0 {
				return false
			}
			mr.Signatures = new(api.RecordRange[*api.SignatureSetRecord])
			return true
		},
	}

	state, local, matched := joinTheDirectory(t, sim, sources, 40)
	require.NotZero(t, touched, "no anchor was served to the node at all, so this test proves nothing")
	require.Zero(t, matched,
		"a node promoted on a root nobody signed: the chain of trust terminates in the peer (#4301)")
	_ = state
	requireNoSpineKept(t, local)
	t.Logf("%d anchors were served with their signatures removed; the node did not promote and kept no spine", touched)
}

// TestForgedValidatorsDoNotPromoteTheNode is the Done-when of #4301 said
// exactly: "a peer serving a self-consistent but unquorumed history".
//
// The peer here is not lazy. It takes each real anchor, re-signs the very
// envelope the real validators signed with four keys of its OWN, declaring
// the current network version, and serves a full quorum of them. Every
// signature verifies. Everything else it serves is true. The only thing
// wrong with it is that the definition this node holds does not name those
// keys -- which is the whole of what "a quorum of THAT PARTITION'S
// validators" means, and the only thing standing between a joining node and
// a history one peer made up.
func TestForgedValidatorsDoNotPromoteTheNode(t *testing.T) {
	sim := spineNetwork(t, 3)

	var keys []ed25519.PrivateKey
	for i := 0; i < 4; i++ {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys = append(keys, priv)
	}

	forged := 0
	sources := rewritePool{
		inner:   namedPeers(t, sim),
		pool:    theDirectorysPool(t, sim),
		touched: &forged,
		rewrite: func(mr *api.MessageRecord[messaging.Message]) bool {
			if mr.Sequence == nil || mr.Message == nil {
				return false
			}
			seq := *mr.Sequence
			seq.Message = mr.Message
			h := seq.Hash()

			set := &api.SignatureSetRecord{
				Account:    &UnknownAccount{Url: mr.ID.Account()},
				Signatures: new(api.RecordRange[*api.MessageRecord[messaging.Message]]),
			}
			for _, k := range keys {
				sig, err := new(signing.Builder).
					SetType(SignatureTypeED25519).
					SetPrivateKey(k).
					SetUrl(DnUrl().JoinPath(Network)).
					SetVersion(1).
					SetTimestamp(1).
					Sign(h[:])
				if err != nil {
					return false
				}
				set.Signatures.Records = append(set.Signatures.Records,
					&api.MessageRecord[messaging.Message]{
						Message: &messaging.BlockAnchor{Anchor: &seq, Signature: sig.(KeySignature)},
					})
			}
			set.Signatures.Total = uint64(len(set.Signatures.Records))
			mr.Signatures = &api.RecordRange[*api.SignatureSetRecord]{
				Records: []*api.SignatureSetRecord{set}, Total: 1,
			}
			return true
		},
	}

	_, local, matched := joinTheDirectory(t, sim, sources, 40)
	require.NotZero(t, forged, "no anchor was re-signed, so this test proves nothing")
	require.Zero(t, matched,
		"a node promoted on a history signed by four keys of the peer's own making: "+
			"the quorum was counted and its membership was not (#4301)")
	requireNoSpineKept(t, local)
	t.Logf("%d anchors were re-signed by a quorum of keys the network does not name; none promoted the node", forged)
}

// TestAPartialQuorumDoesNotPromoteTheNode is the other half of the Done-when:
// "fewer than a quorum".
//
// Three nodes, so the Directory's threshold is two and the anchors carry
// exactly two signatures -- the threshold and no margin. One is removed from
// every anchor in the pool. Everything else the peer serves is true and every
// signature that remains is a real validator's.
func TestAPartialQuorumDoesNotPromoteTheNode(t *testing.T) {
	sim := spineNetwork(t, 3)

	thinned := 0
	sources := rewritePool{
		inner:   namedPeers(t, sim),
		pool:    theDirectorysPool(t, sim),
		touched: &thinned,
		rewrite: func(mr *api.MessageRecord[messaging.Message]) bool {
			if mr.Signatures == nil {
				return false
			}
			kept := 0
			for _, set := range mr.Signatures.Records {
				if set == nil || set.Signatures == nil {
					continue
				}
				var keep []*api.MessageRecord[messaging.Message]
				for _, s := range set.Signatures.Records {
					ba, ok := s.Message.(*messaging.BlockAnchor)
					if !ok || ba.Signature == nil {
						keep = append(keep, s) // not a validator signature; leave it
						continue
					}
					if kept == 0 {
						kept++
						keep = append(keep, s)
					}
				}
				set.Signatures.Records = keep
				set.Signatures.Total = uint64(len(keep))
			}
			return kept > 0
		},
	}

	_, local, matched := joinTheDirectory(t, sim, sources, 40)
	require.NotZero(t, thinned, "no anchor was thinned, so this test proves nothing")
	require.Zero(t, matched,
		"a node promoted on one validator's signature where the set requires two (#4301)")
	requireNoSpineKept(t, local)
	t.Logf("%d anchors were served with one signature of the two the set requires; none promoted the node", thinned)
}

// TestAJoinCrossesAChangeToTheValidatorSet is the regression for review
// finding 1 on #4301.
//
// **There is no carrier.** Past Vandenberg a change to `dn.acme/network`
// leaves the Directory as a `messaging.NetworkUpdate` and never enters an
// anchor (`block_end.go:791-793`, `network_accounts.go:128-131`), and the
// anchor of the block that executed the change is already signed under the
// new version. A verifier that chose the validator set by the version a
// signature declared therefore refused every anchor from the moment the
// network moved, and a node down across any governance write to
// `dn.acme/network` -- even one that changes no active key -- never joined
// again. This is that node.
//
// It crosses because an anchor is judged by MEMBERSHIP in the set this node
// holds, and because the new definition then arrives the only way it safely
// can: as a spine account, with a receipt that ends at a root a quorum
// signed and passes through the leaf the pulled body hashes to.
func TestAJoinCrossesAChangeToTheValidatorSet(t *testing.T) {
	sim := spineNetwork(t, 3)

	// The node's own store, seeded BEFORE the network moves: this is the
	// validator down across the change.
	local := genesisOf(t, sim, Directory)
	t.Cleanup(func() { _ = local.Close() })
	before := versionOf(t, local, DnUrl())

	// A real change, executed by the network: a validator added to the
	// definition. The simulator's consensus nodes are fixed at construction,
	// so it joins inactive -- which is the reviewer's case exactly, a
	// governance write that does not even change who signs.
	current := loadDirectoryGlobals(t, sim)
	newKey := acctesting.GenerateKey(t.Name(), "new-validator")
	current.Network.AddValidator(newKey[32:], Directory, false)
	current.Network.Version++
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl(), Network).
			Body(&WriteData{Entry: current.FormatNetwork(), WriteToState: true}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 1)))
	sim.StepUntil(Txn(st.TxID).Completes())
	sim.StepN(10)

	after := versionOf(t, sim.S.Database(Directory), DnUrl())
	require.Greater(t, after, before, "the network definition did not change; this test proves nothing")

	state, matched := joinTheDirectoryFrom(t, sim, namedPeers(t, sim), local, 80)
	require.NotZero(t, matched,
		"a node holding the definition from before a governance write never joined again: "+
			"nothing on this line carries a change in an anchor, so a verifier that picks the set "+
			"by the version a signature declares refuses every anchor forever (#4301)")

	require.Equal(t, after, versionOf(t, local, DnUrl()),
		"the node matched but its store never took the new definition")
	require.Equal(t, after, state.TrustedVersion(),
		"the node matched but is still verifying against the set it started with: "+
			"it crossed on overlap alone, and the next change would strand it")
	t.Logf("a node holding definition version %d joined a network at version %d, matched at block %d",
		before, after, matched)
}

// versionOf is the network definition version a store holds for a partition.
func versionOf(t *testing.T, db database.Viewer, part *url.URL) uint64 {
	t.Helper()
	g := new(core.GlobalValues)
	require.NoError(t, db.View(func(batch *database.Batch) error {
		return g.LoadNetwork(part, func(account *url.URL, target interface{}) error {
			return batch.Account(account).Main().GetAs(target)
		})
	}))
	return g.Network.Version
}
