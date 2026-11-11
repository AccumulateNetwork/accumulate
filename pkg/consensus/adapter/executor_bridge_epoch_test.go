// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func globalsWith(version uint64, keys ...[32]byte) *core.GlobalValues {
	g := new(core.GlobalValues)
	g.Network = &protocol.NetworkDefinition{Version: version}
	for _, k := range keys {
		k := k
		g.Network.Validators = append(g.Network.Validators, &protocol.ValidatorInfo{
			PublicKey: k[:],
			Partitions: []*protocol.ValidatorPartitionInfo{
				{ID: "BVN1", Active: true},
			},
		})
	}
	return g
}

// TestBridgeEpochTracksNetworkVersion verifies the committee-epoch source of
// truth: the bridge reports the network definition version alongside the
// validator set, and notifies on version bumps even when the set is
// unchanged. The version is executed state, so a node that restores from a
// snapshot or replays blocks derives the same epoch as nodes that were
// running — a locally incremented counter cannot guarantee that (#4058).
func TestBridgeEpochTracksNetworkVersion(t *testing.T) {
	b := &ExecutorBridge{partitionID: "BVN1"}

	var gotValidators []ValidatorInfo
	var gotVersion uint64
	var calls int
	b.OnValidatorSetChange(func(v []ValidatorInfo, version uint64, _ protocol.ExecutorVersion) {
		gotValidators, gotVersion = v, version
		calls++
	})

	k1, k2 := [32]byte{1}, [32]byte{2}

	// Initial set at version 1
	b.updateValidatorsFromGlobals(nil, globalsWith(1, k1))
	if calls != 1 || gotVersion != 1 || len(gotValidators) != 1 {
		t.Fatalf("initial: calls=%d version=%d validators=%d", calls, gotVersion, len(gotValidators))
	}

	// Same set, same version — no notification
	b.updateValidatorsFromGlobals(nil, globalsWith(1, k1))
	if calls != 1 {
		t.Fatalf("duplicate globals notified: calls=%d", calls)
	}

	// Version bump with NO set change must still notify: nodes that skip it
	// would disagree with a later joiner whose initial epoch is the restored
	// version
	b.updateValidatorsFromGlobals(nil, globalsWith(2, k1))
	if calls != 2 || gotVersion != 2 {
		t.Fatalf("version-only bump: calls=%d version=%d", calls, gotVersion)
	}

	// A validator add arrives with the full set, not a diff
	b.updateValidatorsFromGlobals(nil, globalsWith(3, k1, k2))
	if calls != 3 || gotVersion != 3 {
		t.Fatalf("add: calls=%d version=%d", calls, gotVersion)
	}
	if len(gotValidators) != 2 {
		t.Fatalf("add must deliver the FULL set (old committee + new member), got %d validators", len(gotValidators))
	}

	// A removal likewise delivers the remaining set
	b.updateValidatorsFromGlobals(nil, globalsWith(4, k2))
	if len(gotValidators) != 1 || gotValidators[0].PublicKey != k2 {
		t.Fatalf("remove must deliver the remaining set, got %d validators", len(gotValidators))
	}
}

// A validator change is reported with the executor version active for the
// block that made it: the globals it replaces, which is what the executor
// gates the change's ledger fields on, so the block that activates
// v2-canaveral reports Kourou. Globals loaded with nothing before them report
// their own version (#4593).
func TestBridgeReportsTheExecutorVersionOfTheBlockThatMadeTheChange(t *testing.T) {
	bus := events.NewBus(nil)
	b, err := NewExecutorBridge(ExecutorBridgeConfig{Executor: &fakeExec{}, PartitionID: "BVN1", EventBus: bus})
	require.NoError(t, err)
	var got []protocol.ExecutorVersion
	b.OnValidatorSetChange(func(_ []ValidatorInfo, _ uint64, executor protocol.ExecutorVersion) {
		got = append(got, executor)
	})

	k1, k2 := [32]byte{1}, [32]byte{2}
	loaded := globalsWith(1, k1)
	loaded.ExecutorVersion = protocol.ExecutorVersionV2Kourou
	require.NoError(t, bus.Publish(events.WillChangeGlobals{New: loaded}))
	require.Equal(t, []protocol.ExecutorVersion{protocol.ExecutorVersionV2Kourou}, got, "loaded: the new globals' version")

	old := loaded
	next := globalsWith(2, k1, k2)
	next.ExecutorVersion = protocol.ExecutorVersionV2Canaveral
	require.NoError(t, bus.Publish(events.WillChangeGlobals{Old: old, New: next}))
	require.Equal(t, protocol.ExecutorVersionV2Kourou, got[1], "the block's version, not the one it activates")

	later := globalsWith(3, k2)
	later.ExecutorVersion = protocol.ExecutorVersionV2Canaveral
	require.NoError(t, bus.Publish(events.WillChangeGlobals{Old: next, New: later}))
	require.Equal(t, protocol.ExecutorVersionV2Canaveral, got[2])
}
