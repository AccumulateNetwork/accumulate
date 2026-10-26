// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// A network-account write that is rejected after its entry parses must change
// neither the pending nor the active globals: block close publishes no
// WillChangeGlobals for it and hands consensus no validator updates. Before #4615 the executor parsed the
// entry into its pending globals first and rejected afterwards, and block
// close made the parsed value active.
func TestFailedNetworkWriteChangesNoGlobals(t *testing.T) {
	type write struct {
		partition string
		account   string
		entry     func(*core.GlobalValues) DataEntry
		toState   bool
		message   string
	}
	var sim *Sim
	nextNetwork := func(g *core.GlobalValues) DataEntry {
		changeValidators(sim, g)
		return g.FormatNetwork()
	}
	cases := map[string]write{
		"network without WriteToState": {Directory, Network, nextNetwork, false, "updates to %[1]v must write to state"},
		"globals without WriteToState": {Directory, Globals, func(g *core.GlobalValues) DataEntry {
			g.Globals.OperatorAcceptThreshold.Set(1, 99)
			return g.FormatGlobals()
		}, false, "updates to %[1]v must write to state"},
		"oracle without WriteToState": {Directory, Oracle, func(g *core.GlobalValues) DataEntry {
			g.Oracle.Price++
			return g.FormatOracle()
		}, false, "updates to %[1]v must write to state"},
		"routing without WriteToState": {Directory, Routing, func(g *core.GlobalValues) DataEntry {
			g.Routing.Overrides = append(g.Routing.Overrides, RouteOverride{Account: AccountUrl("staking"), Partition: Directory})
			return g.FormatRouting()
		}, false, "updates to %[1]v must write to state"},
		"network version does not increase": {Directory, Network, func(g *core.GlobalValues) DataEntry {
			g.Network.NetworkName = "stale"
			return g.FormatNetwork()
		}, true, "version must increase: %[2]d <= %[2]d"},
		"direct write to a BVN": {"BVN0", Network, nextNetwork, true, "%[1]v cannot be updated directly"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var w *globalsWatch
			sim, w = newGlobalsSim(t, ExecutorVersionLatest, c.partition)
			before := partitionGlobals(sim, c.partition)
			principal := PartitionUrl(c.partition).JoinPath(c.account)

			st := sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(principal).
					Body(&WriteData{Entry: c.entry(before.Copy()), WriteToState: c.toState}).
					SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)))

			sim.StepUntil(
				Txn(st.TxID).Fails().
					WithError(errors.BadRequest).
					WithMessagef(c.message, principal, before.Network.Version))
			sim.StepN(5)

			require.Zero(t, w.globalsChanges, "block close must publish no globals change for a failed write")
			require.Zero(t, w.validatorUpdates, "block close must return no validator updates for a failed write")
			requireSameGlobals(t, before, partitionGlobals(sim, c.partition))
		})
	}
}

// The control: the same write succeeding changes the globals at block close.
func TestSucceededNetworkWriteChangesGlobals(t *testing.T) {
	sim, w := newGlobalsSim(t, ExecutorVersionLatest, Directory)
	g := partitionGlobals(sim, Directory)
	changeValidators(sim, g)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl().JoinPath(Network)).
			Body(&WriteData{Entry: g.FormatNetwork(), WriteToState: true}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)))
	sim.StepUntil(
		Txn(st.TxID).Succeeds())
	sim.StepN(5)

	require.NotZero(t, w.globalsChanges)
	require.NotZero(t, w.validatorUpdates)
	require.Equal(t, g.Network.Version, partitionGlobals(sim, Directory).Network.Version)
}

// Before V2Canaveral a write rejected after the parse did change the globals,
// and replaying that history must do so again.
func TestFailedNetworkWriteBeforeCanaveral(t *testing.T) {
	sim, w := newGlobalsSim(t, ExecutorVersionV2Kourou, Directory)
	g := partitionGlobals(sim, Directory)
	g.Network.Version++

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl().JoinPath(Network)).
			Body(&WriteData{Entry: g.FormatNetwork(), WriteToState: false}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)))
	sim.StepUntil(
		Txn(st.TxID).Fails())
	sim.StepN(5)

	require.NotZero(t, w.globalsChanges)
	require.Equal(t, g.Network.Version, partitionGlobals(sim, Directory).Network.Version)
}

// globalsWatch counts, on a partition's first node, the globals changes block
// close publishes and the blocks it closes with validator updates.
type globalsWatch struct {
	globalsChanges   int
	validatorUpdates int
}

// newGlobalsSim starts a network at the given version and watches the
// partition from when the network has settled.
func newGlobalsSim(t *testing.T, version ExecutorVersion, partition string) (*Sim, *globalsWatch) {
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	g.ExecutorVersion = version
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.GenesisWith(GenesisTime, g),
	)
	sim.StepN(10)

	w := new(globalsWatch)
	bus := sim.S.EventBus(partition)
	events.SubscribeSync(bus, func(events.WillChangeGlobals) error {
		w.globalsChanges++
		return nil
	})
	events.SubscribeSync(bus, func(e execute.WillCommitBlock) error {
		if _, ok := e.Block.DidUpdateValidators(); ok {
			w.validatorUpdates++
		}
		return nil
	})
	return sim, w
}

// changeValidators deactivates the third node's validators on the Directory
// and BVN0, so that a definition that took effect would change both validator
// sets. The two left keep a quorum.
func changeValidators(sim *Sim, g *core.GlobalValues) {
	for _, part := range []string{Directory, "BVN0"} {
		g.Network.AddValidator(sim.S.SignWithNode(part, 2).Key()[32:], part, false)
	}
	g.Network.Version++
}

func partitionGlobals(sim *Sim, partition string) *core.GlobalValues {
	ns := sim.NetworkStatus(api.NetworkStatusOptions{Partition: partition})
	return &core.GlobalValues{
		Oracle:          ns.Oracle,
		Globals:         ns.Globals,
		Network:         ns.Network,
		Routing:         ns.Routing,
		ExecutorVersion: ns.ExecutorVersion,
	}
}

func requireSameGlobals(t *testing.T, want, got *core.GlobalValues) {
	t.Helper()
	require.True(t, want.Network.Equal(got.Network), "network definition changed")
	require.True(t, want.Globals.Equal(got.Globals), "globals changed")
	require.True(t, want.Oracle.Equal(got.Oracle), "oracle changed")
	require.True(t, want.Routing.Equal(got.Routing), "routing changed")
	require.Equal(t, want.ExecutorVersion, got.ExecutorVersion)
}
