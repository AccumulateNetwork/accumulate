// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A SYNCING NODE REFUSES EVERY READ, NOT TWO QUERY KINDS (#4295).
//
// executor.md, "Sync", step 6: "In this phase a syncing node refuses every
// read and answers once it is fully synced (Paul, 2026-09-19): BOOTING
// refuses with NotReady, ACTIVE serves." Until #4295 the querier gated
// exactly two kinds — a BPT page and an account read carrying a receipt — and
// a joining node answered an ordinary account read out of the store its own
// pull was half filling. That read is wrong in the same way the other two
// are: the account it returns is whatever the pull has written so far, which
// is some of one block's state and some of another's.
//
// WHAT IS PRODUCTION HERE AND WHAT IS BY HAND. The querier is not built: the
// daemon's own (*Querier).start runs against an Instance, and the service
// this test asks is the one start registered, with the nodestate the ioc
// registry handed it — the line whose deletion the auditor showed no test
// caught (#4317, #4320). The promotion is the real one: tracker.Check reads
// the node's own BPT root and promotes the same machine the querier holds.
// By hand: the machine starts at BOOTING because nothing here runs a join
// (a BOOTING node cannot be produced end to end in this process — the netsim
// starts every node from genesis, and the restart path that would join is
// #4361/#4362's, still skipped), and the anchored root fed to Observe is this
// node's own root rather than one read out of a Directory anchor.
func TestAJoiningNodeRefusesEveryRead(t *testing.T) {
	const part = "BVN0"
	partUrl := protocol.PartitionUrl(part)
	alice := protocol.AccountUrl("alice")
	ctx := context.Background()
	logger := logging.NewSlogLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))

	node, err := p2p.New(p2p.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })
	inst := &Instance{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		services: ioc.Registry{},
		p2p:      node,
	}

	// One store, registered the way the daemon registers a partition's
	// storage, so the querier the daemon starts and the tracker that promotes
	// it read the same database — as they do in dagbft.go.
	store := memory.New(nil)
	require.NoError(t, ioc.Register[keyvalue.Beginner](inst.services, part, store))

	// An account this node holds, so that "it answers" is a record and not
	// merely the absence of a refusal.
	db := database.New(store, logger)
	batch := db.Begin(true)
	require.NoError(t, batch.Account(alice).Main().Put(&protocol.UnknownAccount{Url: alice}))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	// BOOTING: the state a node is in from the moment it starts joining until
	// its root matches one the Directory anchored.
	machine := nodestate.New(partUrl)
	q := &Querier{Partition: part, Storage: &StorageOrRef{}}
	require.NoError(t, querierWantsNodeState.Register(inst.services, q, machine))
	require.NoError(t, q.start(inst))
	svc, err := querierProvides.Get(inst.services, q)
	require.NoError(t, err)

	// EVERY read: one of each query kind the querier answers. The list is
	// checked against api.QueryType below, so a kind added to the querier
	// that this test does not drive is a failure rather than a silence — the
	// gate used to be a switch, and a switch that exempts six search kinds
	// stays green against a test that drives seven.
	type read struct {
		name  string
		scope *url.URL
		query apiv3.Query
	}
	count := uint64(4)
	minor := uint64(1)
	hash := [32]byte{1}
	every := []read{
		{"bpt-page", partUrl, &apiv3.BptPageQuery{Count: 4}},
		{"account-with-receipt", alice, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}}},
		{"ordinary-account", alice, &apiv3.DefaultQuery{}},
		{"chain", alice, &apiv3.ChainQuery{Name: "main"}},
		{"data", alice, &apiv3.DataQuery{Range: &apiv3.RangeOptions{Count: &count}}},
		{"directory", alice, &apiv3.DirectoryQuery{Range: &apiv3.RangeOptions{Count: &count}}},
		{"pending", alice, &apiv3.PendingQuery{Range: &apiv3.RangeOptions{Count: &count}}},
		{"block", partUrl, &apiv3.BlockQuery{Minor: &minor}},
		{"anchor-search", partUrl, &apiv3.AnchorSearchQuery{Anchor: hash[:]}},
		{"public-key-search", alice, &apiv3.PublicKeySearchQuery{PublicKey: hash[:], Type: protocol.SignatureTypeED25519}},
		{"public-key-hash-search", alice, &apiv3.PublicKeyHashSearchQuery{PublicKeyHash: hash[:]}},
		{"delegate-search", alice, &apiv3.DelegateSearchQuery{Delegate: protocol.AccountUrl("bob")}},
		{"message-hash-search", alice, &apiv3.MessageHashSearchQuery{Hash: hash}},
	}

	// The list is complete: every QueryType the querier's own switch names is
	// driven above. This is what makes "every read" a claim rather than a
	// sample (test audit, gap 4 / M4).
	driven := map[apiv3.QueryType]bool{}
	for _, r := range every {
		driven[r.query.QueryType()] = true
	}
	for _, qt := range []apiv3.QueryType{
		apiv3.QueryTypeDefault, apiv3.QueryTypeChain, apiv3.QueryTypeData,
		apiv3.QueryTypeDirectory, apiv3.QueryTypePending, apiv3.QueryTypeBlock,
		apiv3.QueryTypeAnchorSearch, apiv3.QueryTypePublicKeySearch,
		apiv3.QueryTypePublicKeyHashSearch, apiv3.QueryTypeDelegateSearch,
		apiv3.QueryTypeMessageHashSearch, apiv3.QueryTypeBptPage,
	} {
		require.True(t, driven[qt], "no read of kind %v is driven: the gate is not proved for it", qt)
	}

	for _, r := range every {
		_, err := svc.Query(ctx, r.scope, r.query)
		require.Error(t, err, "a joining node answered %s", r.name)
		require.True(t, errors.Is(err, errors.NotReady),
			"%s: a joining node must refuse NotReady, got %v", r.name, err)
		require.Contains(t, err.Error(), "is joining", "%s: it must say why it refused", r.name)
	}

	// --- and once its root matches an anchored root, it serves --------------
	//
	// The real tracker, on the node's own database: it reads the local BPT
	// root and promotes the machine the querier is holding. Nothing about the
	// querier is rebuilt; it is the same registered service.
	trk, err := tracker.New(db, machine)
	require.NoError(t, err)
	local := func() [32]byte {
		b := db.Begin(false)
		defer b.Discard()
		root, err := b.GetBptRootHash()
		require.NoError(t, err)
		return root
	}()
	trk.Observe(partUrl, 12, local)
	promoted, err := trk.Check(ctx)
	require.NoError(t, err)
	require.True(t, promoted, "the tracker did not promote on a matching root")
	require.Equal(t, nodestate.StateActive, machine.State())

	// The ordinary read is now answered, with the account this node holds.
	r, err := svc.Query(ctx, alice, &apiv3.DefaultQuery{})
	require.NoError(t, err, "a node whose root matches an anchored root refused an ordinary read")
	require.NotNil(t, r)

	// And so is the BPT page, which is what says the gate came off rather
	// than the store having gone empty.
	_, err = svc.Query(ctx, partUrl, &apiv3.BptPageQuery{Count: 4})
	require.NoError(t, err, "a node whose root matches an anchored root refused a BPT page")
}

// NETWORK STATUS IS A READ (#4295 F1).
//
// executor.md, "Sync", step 6: a syncing node refuses every read. A node's
// network status is its globals, its ORACLE and its ROUTING TABLE, read out
// of the store — and while the node is joining that store is the one its own
// pull is filling, so the routing table it answers with is part one block's
// and part another's. It is not a monitoring read that a peer needs in order
// to skip this node: a peer learns that from ConsensusStatus.CatchingUp,
// which is a different service and is not gated. It is a read a WALLET takes:
// pkg/api/v3/p2p/client.go builds an external client's router out of it, so a
// client that happens to connect to a joining node routes by a stale table
// for as long as it holds the client.
//
// Production wiring: the daemon's own (*NetworkService).start against an
// Instance, and the service start registered. By hand: the machine starts at
// BOOTING because nothing here runs a join.
func TestAJoiningNodeRefusesNetworkStatus(t *testing.T) {
	const part = "BVN0"
	ctx := context.Background()

	node, err := p2p.New(p2p.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })
	inst := &Instance{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		services: ioc.Registry{},
		p2p:      node,
	}
	require.NoError(t, ioc.Register[keyvalue.Beginner](inst.services, part, memory.New(nil)))
	require.NoError(t, ioc.Register[*events.Bus](inst.services, part, events.NewBus(nil)))

	machine := nodestate.New(protocol.PartitionUrl(part))
	n := &NetworkService{Partition: part}
	require.NoError(t, networkWantsNodeState.Register(inst.services, n, machine))
	require.NoError(t, n.start(inst))
	svc, err := networkProvides.Get(inst.services, n)
	require.NoError(t, err)

	_, err = svc.NetworkStatus(ctx, apiv3.NetworkStatusOptions{Partition: part})
	require.Error(t, err, "a joining node answered for the network's routing table")
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.Contains(t, err.Error(), "is joining")

	// And a node that never joined answers, so the test is not satisfied by a
	// service that refuses everything.
	inst2 := &Instance{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		services: ioc.Registry{},
		p2p:      node,
	}
	require.NoError(t, ioc.Register[keyvalue.Beginner](inst2.services, part, memory.New(nil)))
	require.NoError(t, ioc.Register[*events.Bus](inst2.services, part, events.NewBus(nil)))
	n2 := &NetworkService{Partition: part}
	require.NoError(t, networkWantsNodeState.Register(inst2.services, n2, nodestate.Always{}))
	require.NoError(t, n2.start(inst2))
	svc2, err := networkProvides.Get(inst2.services, n2)
	require.NoError(t, err)
	_, err = svc2.NetworkStatus(ctx, apiv3.NetworkStatusOptions{Partition: part})
	require.False(t, errors.Is(err, errors.NotReady), "a node that never joined refused its network status: %v", err)
}
