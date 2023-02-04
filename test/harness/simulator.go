// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package harness

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// GenesisTime is 2022-7-1 0:00 UTC.
var GenesisTime = time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)

// NewSim creates a simulator with the given database, network initialization,
// and snapshot function and calls NewSimWith.
func NewSim(tb testing.TB, database simulator.OpenDatabaseFunc, init *accumulated.NetworkInit, snapshot simulator.SnapshotFunc) *Sim {
	s, err := simulator.New(acctesting.NewTestLogger(tb), database, init, snapshot)
	require.NoError(tb, err)
	return NewSimWith(tb, s)
}

// NewSimWith creates a Harness for the given simulator instance and wraps it as
// a Sim.
func NewSimWith(tb testing.TB, s *simulator.Simulator) *Sim {
	return &Sim{*New(tb, s.Services(), s), s}
}

// Sim is a Harness with some extra simulator-specific features.
type Sim struct {
	Harness
	s *simulator.Simulator
}

// Router calls Simulator.Router.
func (s *Sim) Router() routing.Router {
	return s.s.Router()
}

// Partitions calls Simulator.Partitions.
func (s *Sim) Partitions() []*protocol.PartitionInfo {
	return s.s.Partitions()
}

// Database calls Simulator.Database.
func (s *Sim) Database(partition string) database.Updater {
	return s.s.Database(partition)
}

// DatabaseFor calls Simulator.DatabaseFor.
func (s *Sim) DatabaseFor(account *url.URL) database.Updater {
	return s.s.DatabaseFor(account)
}

// SetRoute calls Simulator.SetRoute.
func (s *Sim) SetRoute(account *url.URL, partition string) {
	s.s.SetRoute(account, partition)
}

// SetSubmitHook calls Simulator.SetSubmitHook.
func (s *Sim) SetSubmitHook(partition string, fn simulator.SubmitHookFunc) {
	s.s.SetSubmitHook(partition, fn)
}

// SetSubmitHookFor calls Simulator.SetSubmitHookFor.
func (s *Sim) SetSubmitHookFor(account *url.URL, fn simulator.SubmitHookFunc) {
	s.s.SetSubmitHookFor(account, fn)
}

// SignWithNode calls Simulator.SignWithNode.
func (s *Sim) SignWithNode(partition string, i int) signing.Signer {
	return s.s.SignWithNode(partition, i)
}

func (s *Sim) SubmitTo(partition string, message []messaging.Message) ([]*protocol.TransactionStatus, error) {
	return s.s.SubmitTo(partition, message)
}
