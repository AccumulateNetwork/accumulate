// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

// AUDIT ONLY (no production change). These tests record what a node that is
// still joining -- nodestate BOOTING, nothing executed, store half filled by
// its own pull -- actually answers today. They are written against Paul's
// rule: "while a node is syncing it sends nothing to anyone and responds to
// nothing", and every one of them PASSES, which is the finding.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// auditJoiningDB is the store of a node that is joining: a peer's answers have
// been written into it and nothing has been executed. What it holds is exactly
// what somebody else said, unverified against anything this node produced.
func auditJoiningDB(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())

	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Account(protocol.AccountUrl("alice.acme")).Main().Put(&protocol.ADI{
		Url: protocol.AccountUrl("alice.acme"),
		AccountAuth: protocol.AccountAuth{Authorities: []protocol.AuthorityEntry{
			{Url: protocol.AccountUrl("alice.acme", "book")},
		}},
	}))
	require.NoError(t, batch.Commit())
	return db
}

func auditJoiningQuerier(t *testing.T) *Querier {
	t.Helper()
	q := NewQuerier(QuerierParams{
		Database:  auditJoiningDB(t),
		Partition: "BVN0",
		NodeState: nodestate.New(protocol.PartitionUrl("BVN0")), // BOOTING
	})
	require.False(t, q.nodeState.CanServeCurrent(),
		"the node must be joining for this test to mean anything")
	return q
}

// TestAudit_JoiningNodeServesStateItHasNotExecuted -- the gate at
// querier.go:150 (servingFor) refuses exactly two shapes: BptPageQuery, and
// DefaultQuery with IncludeReceipt. Every other query is answered, from the
// un-executed store, with real data in it.
//
// Paul's rule says a syncing node serves nothing. This is what it serves
// instead.
func TestAudit_JoiningNodeServesStateItHasNotExecuted(t *testing.T) {
	q := auditJoiningQuerier(t)
	ctx := context.Background()

	// A plain account read: answered, with the account body this node took
	// from a peer and never executed.
	r, err := q.Query(ctx, protocol.AccountUrl("alice.acme"), &api.DefaultQuery{})
	require.NoError(t, err, "a joining node refused a plain account read")
	rec, ok := r.(*api.AccountRecord)
	require.True(t, ok, "got %T", r)
	require.Equal(t, "alice.acme", rec.Account.GetUrl().Authority,
		"a joining node served account state it has not executed")

	// The same read, this time asking for the receipt, is the one shape that
	// is refused. The difference is the CALLER'S FLAG, not the node's state.
	_, err = q.Query(ctx, protocol.AccountUrl("alice.acme"),
		&api.DefaultQuery{IncludeReceipt: &api.ReceiptOptions{ForAny: true}})
	require.True(t, errors.Is(err, errors.NotReady), "with a receipt: %v", err)
}

// TestAudit_JoiningNodeAnswersEveryOtherQueryShape enumerates the query shapes
// the gate does not cover. None is refused for being joining: each either
// answers or fails for an ordinary reason (the record is not there). NotReady
// is the refusal; its absence is the finding.
func TestAudit_JoiningNodeAnswersEveryOtherQueryShape(t *testing.T) {
	q := auditJoiningQuerier(t)
	ctx := context.Background()
	alice := protocol.AccountUrl("alice.acme")
	part := protocol.PartitionUrl("BVN0")
	one := uint64(1)

	for _, c := range []struct {
		name  string
		scope *url.URL
		query api.Query
	}{
		{"ChainQuery", alice, &api.ChainQuery{Name: "main"}},
		{"DirectoryQuery", alice, &api.DirectoryQuery{}},
		{"PendingQuery", alice, &api.PendingQuery{}},
		{"DataQuery", alice, &api.DataQuery{}},
		{"BlockQuery", part, &api.BlockQuery{Minor: &one}},
		{"AnchorSearchQuery", part, &api.AnchorSearchQuery{Anchor: make([]byte, 32)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := q.Query(ctx, c.scope, c.query)
			require.False(t, errors.Is(err, errors.NotReady),
				"%s was refused by the join gate -- this audit is out of date: %v", c.name, err)
		})
	}
}

// TestAudit_ProofServiceAnchorReceiptIsNotGated -- #4272's public proof
// service is wired at cmd/accumulated/run/dagbft.go:686 with NO node state.
// Two of its three calls delegate to the Sequencer and inherit its gate; the
// third, AnchorReceipt, reads the node's own database directly
// (internal/api/v3/proof.go:43 Database, :110 AnchorReceipt) and is answered
// while the node is joining.
//
// AnchorReceipt is the call that binds a partition BPT root to a Directory
// root -- the hop an external verifier treats as the root of trust. A joining
// node answering it builds that binding out of a bpt chain and a root chain
// its own pull has not finished filling.
func TestAudit_ProofServiceAnchorReceiptIsNotGated(t *testing.T) {
	db := auditJoiningDB(t)
	machine := nodestate.New(protocol.PartitionUrl("BVN0")) // BOOTING
	seq := NewSequencer(SequencerParams{
		Database:  db,
		Partition: "BVN0",
		Cache:     synthcache.New(0),
		EventBus:  events.NewBus(nil),
		Globals:   new(core.GlobalValues),
		NodeState: machine,
	})
	svc := &ProofService{
		Ranger:    seq,
		Database:  db,
		Directory: func() (database.Viewer, error) { return db, nil },
		Partition: config.NetworkUrl{URL: protocol.PartitionUrl("BVN0")},
	}
	ctx := context.Background()

	// The delegating calls inherit the sequencer's gate. Good.
	_, err := svc.MajorHeaderRange(ctx, api.MajorHeaderRangeOptions{Partition: "BVN0", Start: 1, End: 2})
	require.True(t, errors.Is(err, errors.NotReady), "MajorHeaderRange: %v", err)
	_, err = svc.MinorRootRange(ctx, api.MinorRootRangeOptions{Partition: "BVN0", Since: 1, Until: 2})
	require.True(t, errors.Is(err, errors.NotReady), "MinorRootRange: %v", err)

	// AnchorReceipt does not. It reads the store and answers about it, and
	// whatever it says is a statement about the DATA -- never about this node
	// being joining. The proof: the answer is byte-identical before and after
	// the node's state machine goes ACTIVE, so node state is not consulted.
	ask := func() error {
		_, err := svc.AnchorReceipt(ctx, api.AnchorReceiptOptions{
			Partition: "BVN0",
			BptRoot:   [32]byte{1},
		})
		return err
	}

	whileJoining := ask()
	require.Error(t, whileJoining, "the empty store should not produce a receipt")
	require.NotContains(t, whileJoining.Error(), "is joining",
		"AnchorReceipt refused for joining -- this audit is out of date: %v", whileJoining)

	require.True(t, machine.PromoteToActive([32]byte{2}, 42))
	require.Equal(t, whileJoining.Error(), ask().Error(),
		"AnchorReceipt answers the same joining or not: it does not consult node state")
}

// TestAudit_TheGateDoesNotCoverWhatThePullActuallyReads is the transitive
// case, and it is the one that matters.
//
// internal/api/v3/querier.go:139-147 justifies gating two shapes with: "the
// two things another node's pull reads from a peer are a BPT page and an
// account with a receipt". The pull's own code disagrees. Besides
// BptPageQuery (enumerate.go:120) and DefaultQuery+receipt (pull.go:401) it
// reads:
//
//   - QueryAccountChains and QueryChainEntries -- ChainQuery -- pull.go:508,
//     :553, :613: the chains of every account it pulls;
//   - QueryAccount, plain -- join/state.go:333: the BLOCK LEDGER, which is
//     what decides which accounts get pulled at all;
//   - QueryChain and QueryMainChainEntries -- ChainQuery --
//     pull/verify.go:190 and :201: the DIRECTORY'S ANCHOR CHAIN, which is the
//     root every pulled account is verified against (verify.go:26-32: "the
//     only root a pulled account is verified against").
//
// None of the last four is refused. So a joining node B can take, from a
// joining node A, the anchored roots it verifies everything else against --
// and A's anchor pool is the one A's own pull has not filled. The chain of
// verification ends at "the peer said so", which is exactly the compounding
// case #4297 says the gate exists to prevent.
func TestAudit_TheGateDoesNotCoverWhatThePullActuallyReads(t *testing.T) {
	q := api.Querier2{Querier: auditJoiningQuerier(t)}
	ctx := context.Background()
	anchors := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	ledger := protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)
	count := uint64(8)
	expand := true

	notRefused := func(t *testing.T, err error) {
		t.Helper()
		require.False(t, errors.Is(err, errors.NotReady),
			"the join gate refused this read -- the audit is out of date: %v", err)
	}

	t.Run("the directory's anchor chain head", func(t *testing.T) {
		// pull/verify.go:190
		_, err := q.QueryChain(ctx, anchors, &api.ChainQuery{Name: "main"})
		notRefused(t, err)
	})
	t.Run("the directory's anchors themselves", func(t *testing.T) {
		// pull/verify.go:201 -- the roots everything else is verified against
		_, err := q.QueryMainChainEntries(ctx, anchors, &api.ChainQuery{
			Name:  "main",
			Range: &api.RangeOptions{Start: 0, Count: &count, Expand: &expand},
		})
		notRefused(t, err)
	})
	t.Run("the block ledger that names what to pull", func(t *testing.T) {
		// join/state.go:333
		_, err := q.QueryAccount(ctx, ledger, nil)
		notRefused(t, err)
	})
	t.Run("an account's chains", func(t *testing.T) {
		// pull/pull.go:508, :613
		_, err := q.QueryAccountChains(ctx, protocol.AccountUrl("alice.acme"), &api.ChainQuery{})
		notRefused(t, err)
	})
	t.Run("an account's chain entries", func(t *testing.T) {
		// pull/pull.go:553
		_, err := q.QueryChainEntries(ctx, protocol.AccountUrl("alice.acme"), &api.ChainQuery{
			Name:  "main",
			Range: &api.RangeOptions{Start: 0, Count: &count},
		})
		notRefused(t, err)
	})
}
