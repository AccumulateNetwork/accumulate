// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/dagbft"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// stubAdapter is the executor side of a DAG-BFT service, doing nothing. The
// submit path under test never reaches it.
type stubAdapter struct{}

func (stubAdapter) ProduceBlock(context.Context, adapter.BlockParams) ([32]byte, error) {
	return [32]byte{}, errors.NotAllowed.With("no executor")
}
func (stubAdapter) ValidateTransaction([]byte) error                           { return nil }
func (stubAdapter) LastBlock() (uint64, [32]byte, error)                       { return 0, [32]byte{}, nil }
func (stubAdapter) LastMajorBlock() (uint64, time.Time, bool)                  { return 0, time.Time{}, false }
func (stubAdapter) StateHash() [32]byte                                        { return [32]byte{} }
func (stubAdapter) Validators() []adapter.ValidatorInfo                        { return nil }
func (stubAdapter) OnValidatorSetChange(func([]adapter.ValidatorInfo, uint64)) {}

var _ adapter.ConsensusAdapter = stubAdapter{}

func definitionWith(partition string, active []ed25519.PublicKey) *network.GlobalValues {
	g := new(network.GlobalValues)
	g.Network = new(protocol.NetworkDefinition)
	for _, k := range active {
		g.Network.AddValidator(k, partition, true)
	}
	return g
}

// TestNewSubmitterService_TheDaemonWiresTheCommitteeAndTheRelay — the wiring
// itself, which had no test and survived four production-side mutations
// (#4366 note_3869830125, M1-M4).
//
// The daemon must build the committee predicate from THIS node's author key,
// seed it with the globals it already waited for, subscribe it so a
// committee change moves it, and give the submit service a relay. Each step
// is silently survivable alone: without the predicate or the seed the node
// proposes what it can never get into a block; without the subscription it
// does so from the first committee change onwards; without the relay
// "relay it" is "refuse it" again.
//
// Nothing here is a hand-built predicate: the test hands newSubmitterService
// what the daemon hands it — a key, a globals value, an event bus and a p2p
// node — and reads what the resulting service does with a submission.
func TestNewSubmitterService_TheDaemonWiresTheCommitteeAndTheRelay(t *testing.T) {
	const partition = "BVN7"
	ctx := context.Background()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	someoneElse, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	nodeCfg := consensus.NodeConfig{Partition: partition, KeyPair: priv, NumWorkers: 1}
	svc, err := dagbft.NewService(dagbft.ServiceConfig{
		Partition:  &protocol.PartitionInfo{ID: partition, Type: protocol.PartitionTypeBlockValidator},
		NodeConfig: nodeCfg,
		Adapter:    stubAdapter{},
		EventBus:   events.NewBus(nil),
	})
	require.NoError(t, err)

	// A real p2p node with no peers: a relay from it can find nobody, which
	// is a different answer from every other way this can go wrong.
	apiNode, err := p2p.New(p2p.Options{
		Network: "WiringNet",
		Listen:  []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/0")},
	})
	require.NoError(t, err)
	defer func() { _ = apiNode.Close() }()

	bus := events.NewBus(nil)
	sub := newSubmitterService(submitterParams{
		Partition: partition,
		AuthorKey: pub,
		// Seeded with a committee this node is not in.
		Globals:  definitionWith(partition, []ed25519.PublicKey{someoneElse}),
		EventBus: bus,
		Service:  svc,
		Node:     apiNode,
		Network:  "WiringNet",
	})

	verify := false
	env := &messaging.Envelope{TxHash: []byte("0123456789abcdef0123456789abcdef")}
	opts := api.SubmitOptions{Verify: &verify}

	// In no committee: it relays, and the relay finds nobody. Not "node not
	// started" (which would mean it proposed), and not "has no relay".
	_, err = sub.Submit(ctx, env, opts)
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NoPeer),
		"a node in no committee must relay, not propose and not refuse: %v", err)
	require.Contains(t, err.Error(), partition)

	// The committee changes on chain and this node is now in it: it
	// proposes. "node not started" is this fixture's consensus node, and it
	// is proof the submission went to THIS node's worker.
	require.NoError(t, bus.Publish(events.WillChangeGlobals{
		New: definitionWith(partition, []ed25519.PublicKey{pub, someoneElse}),
	}))
	_, err = sub.Submit(ctx, env, opts)
	require.ErrorContains(t, err, "node not started",
		"a node the committee now contains must propose, not relay")

	// And back out again, so the subscription is not a one-way latch.
	require.NoError(t, bus.Publish(events.WillChangeGlobals{
		New: definitionWith(partition, []ed25519.PublicKey{someoneElse}),
	}))
	_, err = sub.Submit(ctx, env, opts)
	require.True(t, errors.Is(err, errors.NoPeer), "got %v", err)
}
