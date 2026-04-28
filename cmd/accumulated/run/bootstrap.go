// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	stderrors "errors"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pinned"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// detectBootstrapState looks for a bootstrap-state.json artifact in
// the data directory. If one is present, it reconstructs the nodestate
// Machine from the recorded state and stashes both the machine and
// artifact on the Instance. If absent, returns nil — this is the
// normal path for nodes that didn't go through the bootstrap launcher.
//
// Pin enforcement: the artifact records which network it bootstrapped
// against. We resolve the binary's pinned hash for that network via
// the pinned package and require it to match the artifact's recorded
// hash. A mismatch indicates the binary was rebuilt against a
// different genesis snapshot than the artifact was created with —
// continuing without an explicit migration would silently lie about
// the proof of derivation.
//
// When the binary has no pinned hash for the artifact's network (e.g.,
// dev networks where pinned/pinned.go is empty), the pin check is
// skipped with a warning. This matches the behavior of the bootstrap
// launcher itself.
func (inst *Instance) detectBootstrapState() error {
	art, err := bootpersist.Peek(inst.rootDir)
	if err != nil {
		if stderrors.Is(err, os.ErrNotExist) {
			return nil // normal startup, no bootstrap artifact
		}
		return fmt.Errorf("peek bootstrap artifact: %w", err)
	}

	expected := pinned.GenesisHash(art.Network)
	if expected == ([32]byte{}) {
		inst.logger.Warn(
			"Bootstrap artifact present but no pinned genesis hash for network — pin check skipped",
			"network", art.Network,
		)
	} else if art.PinnedGenesisHash != expected {
		return fmt.Errorf("bootstrap pin mismatch for network %q: artifact=%x expected=%x — explicit migration required",
			art.Network, art.PinnedGenesisHash[:8], expected[:8])
	}

	state, err := nodestate.ParseState(art.State.Current)
	if err != nil {
		return fmt.Errorf("parse persisted state %q: %w", art.State.Current, err)
	}

	machine, err := nodestate.Restore(
		state,
		art.PinBlock.MinorBlockIndex,
		art.State.BptRootMatched,
		art.State.HistoryDepth,
	)
	if err != nil {
		return fmt.Errorf("restore nodestate: %w", err)
	}

	inst.bootArtifact = art
	inst.bootMachine = machine
	inst.logger.Info(
		"Resuming from bootstrap-launched state",
		"state", state,
		"network", art.Network,
		"pinBlock", art.PinBlock.MinorBlockIndex,
		"partition", art.PinBlock.Partition,
	)
	return nil
}

// advertisementFromMachine projects the in-process nodestate machine
// onto the wire-format BootstrapAdvertisement that NodeInfo carries
// (#3982). Returns nil if the machine is nil — callers should treat
// nil as "this node didn't go through the bootstrap launcher."
func advertisementFromMachine(m *nodestate.Machine) *apiv3.BootstrapAdvertisement {
	if m == nil {
		return nil
	}
	ad := m.Get()
	return &apiv3.BootstrapAdvertisement{
		State:          stateToWire(ad.State),
		SinceBlock:     ad.SinceBlock,
		BptRootMatched: ad.BptRootMatched,
		HistoryDepth:   ad.HistoryDepth,
		LastUpdated:    ad.LastUpdated,
	}
}

func stateToWire(s nodestate.State) apiv3.BootstrapState {
	switch s {
	case nodestate.StateBooting:
		return apiv3.BootstrapStateBooting
	case nodestate.StateActive:
		return apiv3.BootstrapStateActive
	case nodestate.StateComplete:
		return apiv3.BootstrapStateComplete
	default:
		return apiv3.BootstrapStateUnknown
	}
}
