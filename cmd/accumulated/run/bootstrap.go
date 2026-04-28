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

// detectBootstrapState looks for a v2 bootpersist artifact in the
// data directory. If present, reconstructs the nodestate Machine
// from the recorded state and stashes it (plus the artifact) on the
// Instance for downstream consumers (advertisement publisher,
// hydrator, history backfill). If absent, returns nil — the normal
// path for nodes that didn't go through the v2 bootstrap launcher.
//
// Pin enforcement: the artifact records which network it
// bootstrapped against. We resolve the binary's pin for that network
// via pinned.Get and require it to match the artifact's recorded
// validator-set hash. Mismatch means the binary was rebuilt against
// a different validator-set anchor than the artifact was created
// with — continuing without an explicit migration would silently lie
// about what's been verified.
//
// When the binary has no pin for the artifact's network (dev
// networks where pinned/pinned.go is empty), the pin check is
// skipped with a warning. Production deployments with populated pin
// tables fail closed.
func (inst *Instance) detectBootstrapState() error {
	art, err := bootpersist.Peek(inst.rootDir)
	if err != nil {
		if stderrors.Is(err, os.ErrNotExist) {
			return nil // normal startup, no bootstrap artifact
		}
		return fmt.Errorf("peek bootstrap artifact: %w", err)
	}

	pin := pinned.Get(art.Network)
	if pin.IsZero() {
		inst.logger.Warn(
			"Bootstrap artifact present but no pin for network — pin check skipped",
			"network", art.Network,
		)
	} else if art.PinnedValidatorSetHash != pin.ValidatorSetHash {
		return fmt.Errorf("bootstrap pin mismatch for network %q: artifact=%x expected=%x — explicit migration required",
			art.Network, art.PinnedValidatorSetHash[:8], pin.ValidatorSetHash[:8])
	}

	state, err := nodestate.ParseState(art.State.Current)
	if err != nil {
		return fmt.Errorf("parse persisted state %q: %w", art.State.Current, err)
	}

	machine, err := nodestate.Restore(
		state,
		art.VerifiedHeight,
		art.VerifiedAnchor,
		art.State.HistoryDepth,
	)
	if err != nil {
		return fmt.Errorf("restore nodestate: %w", err)
	}

	inst.bootArtifact = art
	inst.bootMachine = machine
	inst.logger.Info(
		"Resuming from v2 bootstrap-launched state",
		"state", state,
		"network", art.Network,
		"partition", art.Partition,
		"pinnedHeight", art.PinnedHeight,
		"verifiedHeight", art.VerifiedHeight,
	)
	return nil
}

// advertisementFromMachine projects the in-process nodestate machine
// onto the wire-format BootstrapAdvertisement that NodeInfo carries.
// Returns nil if the machine is nil — callers should treat nil as
// "this node didn't go through the v2 bootstrap launcher."
func advertisementFromMachine(m *nodestate.Machine) *apiv3.BootstrapAdvertisement {
	if m == nil {
		return nil
	}
	ad := m.Get()
	return &apiv3.BootstrapAdvertisement{
		State:          stateToWire(ad.State),
		SinceBlock:     ad.SinceBlock,
		VerifiedAnchor: ad.VerifiedAnchor,
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
