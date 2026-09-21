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

	// Every read, by the kind of query it is. The first two are the ones
	// #4297 gated; the rest were answered from the half-filled store.
	type read struct {
		name  string
		scope *url.URL
		query apiv3.Query
	}
	count := uint64(4)
	minor := uint64(1)
	every := []read{
		{"bpt-page", partUrl, &apiv3.BptPageQuery{Count: 4}},
		{"account-with-receipt", alice, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}}},
		{"ordinary-account", alice, &apiv3.DefaultQuery{}},
		{"chain", alice, &apiv3.ChainQuery{Name: "main"}},
		{"directory", alice, &apiv3.DirectoryQuery{Range: &apiv3.RangeOptions{Count: &count}}},
		{"pending", alice, &apiv3.PendingQuery{Range: &apiv3.RangeOptions{Count: &count}}},
		{"block", partUrl, &apiv3.BlockQuery{Minor: &minor}},
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
