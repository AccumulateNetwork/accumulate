// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"context"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// oneSectionGate holds one section not loaded; a block's load of it succeeds
// at once, so only the readers the gate refuses see it.
type oneSectionGate struct {
	section bpt.Section
	on      atomic.Bool
}

func (g *oneSectionGate) AnyNotLoaded() bool                      { return g.on.Load() }
func (g *oneSectionGate) NotLoaded(s bpt.Section) bool            { return g.on.Load() && s == g.section }
func (g *oneSectionGate) Load(*database.Batch, bpt.Section) error { return nil }
func (g *oneSectionGate) Committed([]bpt.Section)                 {}
func (g *oneSectionGate) Discarded([]bpt.Section)                 {}
func (g *oneSectionGate) Opened(*database.Batch)                  {}

// TestTheSimulatorsQuerierSharesTheExecutorsGate — #4497. The simulator's
// querier is built on the node's own database, so a gate put on the database
// the executor runs on refuses its reads, as the daemon's does once it opens
// one database per store (cmd/accumulated/run,
// TestTheDaemonsAPIIsRefusedASectionNotLoaded). Both tests read through the
// API and set the gate where the join sets it; if either wiring opens a
// database of its own for the API again, its test fails.
func TestTheSimulatorsQuerierSharesTheExecutorsGate(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	alice := url.MustParse("alice")
	sim.SetRoute(alice, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, acctesting.GenerateKey(alice)[32:])
	tokens := alice.JoinPath("tokens")
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: tokens, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1)})
	sim.StepN(3)

	db := sim.S.Partition("BVN0").NodeDatabase(0)
	gate := &oneSectionGate{section: database.SectionOfAccount(tokens)}
	db.SetSectionGate(gate)
	t.Cleanup(func() { db.SetSectionGate(nil) })

	ctx := context.Background()
	_, err := sim.Query().QueryAccount(ctx, tokens, nil)
	require.NoError(t, err)

	gate.on.Store(true)
	_, err = sim.Query().QueryAccount(ctx, tokens, nil)
	require.Error(t, err, "the simulator's querier answered an account in a section the executor's gate holds not loaded")
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	d, ok := database.NotLoadedOf(err)
	require.True(t, ok, "the refusal does not carry the redirect shape: %v", err)
	require.Equal(t, uint16(gate.section), *d.Section)
}
