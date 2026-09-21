// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// putDefinition writes the four data accounts core.GlobalValues.Load reads,
// as genesis writes them and as the pull settles them.
func putDefinition(t *testing.T, db *database.Database, partition *url.URL, values *core.GlobalValues) {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()
	put := func(name string, entry protocol.DataEntry) {
		u := partition.JoinPath(name)
		require.NoError(t, batch.Account(u).Main().Put(&protocol.DataAccount{Url: u, Entry: entry}))
	}
	put(protocol.Oracle, values.FormatOracle())
	put(protocol.Globals, values.FormatGlobals())
	put(protocol.Network, values.FormatNetwork())
	put(protocol.Routing, values.FormatRouting())
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
}

// TEST (d): the definition this join PULLED is published at the handoff, and
// it is the pulled one, not the one this node had before it went away
// (#4366's M3, note_3869977850).
//
// On this line a definition moves in exactly one way while a node is running:
// past Vandenberg an anchor does not carry a change to the validator sets
// (#4301 (c)), so a joined node that did not republish kept the committee it
// had before it left until the next ON-CHAIN change -- which can be for ever.
// Three readers take the one event, so all three moved together or none of
// them did: the membership and submit gate, the conductor's anchor gate and
// the adapter's committee.
//
// The scenario the reviewer named: a validator demoted on chain while it was
// offline restarts, joins, goes ACTIVE, and its CanPropose reads the stale
// definition, so it takes submissions and strands them.
func TestHandoff_PublishesTheDefinitionItPulled(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")

	// This node stopped holding a definition of four validators.
	local := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = local.Close() })
	putLedger(t, local, here, 76)
	before, _ := genesisValues(t, 4)
	putDefinition(t, local, here, before)

	bus := events.NewBus(nil)
	var seen []*core.GlobalValues
	events.SubscribeSync(bus, func(e events.WillChangeGlobals) error {
		seen = append(seen, e.New)
		return nil
	})

	s, err := NewState(StateOptions{
		Partition: here,
		Database:  local,
		Sources:   &peerSources{partition: here, querier: noAnchors{}},
		EventBus:  bus,
	})
	require.NoError(t, err)
	require.Empty(t, seen, "a join that has not handed off has published nothing")

	// The pull writes the network's definition into this node's store -- that
	// is what pulling <partition>/network means, and it arrives with a
	// receipt that ends at a root a quorum signed.
	after, _ := genesisValues(t, 7)
	putDefinition(t, local, here, after)

	require.NoError(t, s.Handoff(ctx, 91))

	require.Len(t, seen, 1, "the handoff publishes exactly one definition")
	require.Equal(t, len(after.Network.Partitions), len(seen[0].Network.Partitions))
	var got int
	for _, p := range seen[0].Network.Partitions {
		if p.ID == "BVN0" {
			got = len(seen[0].Network.Validators)
		}
	}
	require.NotZero(t, got)
	require.Len(t, seen[0].Network.Validators, len(after.Network.Validators),
		"the definition published is the one the join PULLED, not the one this node stopped with")
	require.NotEqual(t, len(before.Network.Validators), len(seen[0].Network.Validators),
		"the test proves nothing unless the two differ")

	// And the node is executing from that block, so its own services answer
	// for it again (#4295).
	require.Equal(t, nodestate.StateActive, s.Machine().State())
	require.Equal(t, uint64(91), s.Machine().Get().SinceBlock)
}

// A join with no event bus publishes nothing and says nothing: a test that
// drives the pull alone has nothing to publish to. It must not be how
// production runs, and it is not -- the daemon hands the bus over
// (cmd/accumulated/run/dagbft.go).
func TestHandoff_WithoutABusStillRecordsThatItIsExecuting(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")

	local := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = local.Close() })
	putLedger(t, local, here, 76)
	values, _ := genesisValues(t, 4)
	putDefinition(t, local, here, values)

	s, err := NewState(StateOptions{
		Partition: here,
		Database:  local,
		Sources:   &peerSources{partition: here, querier: noAnchors{}},
	})
	require.NoError(t, err)
	require.NoError(t, s.Handoff(ctx, 91))
	require.Equal(t, nodestate.StateActive, s.Machine().State())
}
