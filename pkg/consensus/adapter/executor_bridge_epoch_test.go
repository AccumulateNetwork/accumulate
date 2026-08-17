// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
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
	b.OnValidatorSetChange(func(v []ValidatorInfo, version uint64) {
		gotValidators, gotVersion = v, version
		calls++
	})

	k1, k2 := [32]byte{1}, [32]byte{2}

	// Initial set at version 1
	b.updateValidatorsFromGlobals(globalsWith(1, k1))
	if calls != 1 || gotVersion != 1 || len(gotValidators) != 1 {
		t.Fatalf("initial: calls=%d version=%d validators=%d", calls, gotVersion, len(gotValidators))
	}

	// Same set, same version — no notification
	b.updateValidatorsFromGlobals(globalsWith(1, k1))
	if calls != 1 {
		t.Fatalf("duplicate globals notified: calls=%d", calls)
	}

	// Version bump with NO set change must still notify: nodes that skip it
	// would disagree with a later joiner whose initial epoch is the restored
	// version
	b.updateValidatorsFromGlobals(globalsWith(2, k1))
	if calls != 2 || gotVersion != 2 {
		t.Fatalf("version-only bump: calls=%d version=%d", calls, gotVersion)
	}

	// A validator add arrives with the full set, not a diff
	b.updateValidatorsFromGlobals(globalsWith(3, k1, k2))
	if calls != 3 || gotVersion != 3 {
		t.Fatalf("add: calls=%d version=%d", calls, gotVersion)
	}
	if len(gotValidators) != 2 {
		t.Fatalf("add must deliver the FULL set (old committee + new member), got %d validators", len(gotValidators))
	}

	// A removal likewise delivers the remaining set
	b.updateValidatorsFromGlobals(globalsWith(4, k2))
	if len(gotValidators) != 1 || gotValidators[0].PublicKey != k2 {
		t.Fatalf("remove must deliver the remaining set, got %d validators", len(gotValidators))
	}
}
