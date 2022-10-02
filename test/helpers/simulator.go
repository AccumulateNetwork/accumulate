// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package helpers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

var GenesisTime = time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)

type Sim struct {
	*simulator.Simulator
	T testing.TB
}

func NewSim(t testing.TB, database simulator.OpenDatabaseFunc, init *accumulated.NetworkInit, snapshot simulator.SnapshotFunc) *Sim {
	s, err := simulator.New(acctesting.NewTestLogger(t), database, init, snapshot)
	require.NoError(t, err)
	return &Sim{s, t}
}

func (s *Sim) Submit(delivery *chain.Delivery) *protocol.TransactionStatus {
	s.T.Helper()
	status, err := s.Simulator.Submit(delivery)
	require.NoError(s.T, err)
	return status
}

func (s *Sim) SubmitSuccessfully(delivery *chain.Delivery) *protocol.TransactionStatus {
	s.T.Helper()
	status := s.Submit(delivery)
	if status.Error != nil {
		require.NoError(s.T, status.Error)
	}
	return status
}

func (s *Sim) Step() {
	s.T.Helper()
	require.NoError(s.T, s.Simulator.Step())
}

func (s *Sim) StepN(n int) {
	s.T.Helper()
	for i := 0; i < n; i++ {
		require.NoError(s.T, s.Simulator.Step())
	}
}

func (s *Sim) StepUntil(conditions ...Condition) {
	s.T.Helper()
	for i := 0; ; i++ {
		if i >= 50 {
			s.T.Fatal("Condition not met after 50 blocks")
		}
		ok := true
		for _, c := range conditions {
			if !c(s) {
				ok = false
			}
		}
		if ok {
			break
		}
		s.Step()
	}
}

func (s *Sim) GetStatus(txid *url.TxID) *protocol.TransactionStatus {
	s.T.Helper()
	var v *protocol.TransactionStatus
	var err error
	View(s.T, s.DatabaseFor(txid.Account()), func(batch *database.Batch) {
		s.T.Helper()
		h := txid.Hash()
		v, err = batch.Transaction(h[:]).Status().Get()
		require.NoError(s.T, err)

		// The database returns a new status if it can't find one
		if v.Equal(new(protocol.TransactionStatus)) {
			s.T.Fatalf("Transaction %x@%v cannot be found", h[:4], txid.Account())
		}
	})
	return v
}
