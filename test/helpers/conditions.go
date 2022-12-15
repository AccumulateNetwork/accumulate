// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package helpers

import (
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Condition func(sim *Sim) bool

func Txn(id *url.TxID) *condTxn { return &condTxn{id} }

func (c condTxn) IsDelivered() Condition { return c.status(isDelivered) }
func (c condTxn) IsPending() Condition   { return c.status(isPending) }
func (c condTxn) Succeeds() Condition    { return c.status(succeeds) }
func (c condTxn) Fails() Condition       { return c.status(fails) }
func (c condTxn) Received() Condition    { return c.status(received) }

func (c condTxn) Produced() condProduced { return condProduced(c) }

func (c condProduced) IsDelivered() Condition { return c.status(isDelivered) }
func (c condProduced) IsPending() Condition   { return c.status(isPending) }
func (c condProduced) Succeeds() Condition    { return c.status(succeeds) }
func (c condProduced) Fails() Condition       { return c.status(fails) }

type condTxn struct{ id *url.TxID }
type condProduced struct{ id *url.TxID }

func (c condTxn) status(predicate func(sim *Sim, status *protocol.TransactionStatus) bool) Condition {
	return func(s *Sim) bool {
		var ok bool
		s.T.Helper()
		View(s.T, s.DatabaseFor(c.id.Account()), func(batch *database.Batch) {
			s.T.Helper()

			h := c.id.Hash()
			status, err := batch.Transaction(h[:]).Status().Get()
			require.NoError(s.T, err)
			ok = predicate(s, status)
		})
		return ok
	}
}

func (c condProduced) status(predicate func(sim *Sim, status *protocol.TransactionStatus) bool) Condition {
	var produced []*url.TxID
	return func(s *Sim) bool {
		s.T.Helper()

		if len(produced) == 0 {
			View(s.T, s.DatabaseFor(c.id.Account()), func(batch *database.Batch) {
				h := c.id.Hash()
				var err error
				produced, err = batch.Transaction(h[:]).Produced().Get()
				require.NoError(s.T, err)
			})
			if len(produced) == 0 {
				return false
			}
		}

		for _, id := range produced {
			var ok bool
			View(s.T, s.DatabaseFor(id.Account()), func(batch *database.Batch) {
				s.T.Helper()

				h := id.Hash()
				status, err := batch.Transaction(h[:]).Status().Get()
				require.NoError(s.T, err)
				ok = predicate(s, status)
			})
			if !ok {
				return false
			}
		}

		return true
	}
}

func isDelivered(s *Sim, status *protocol.TransactionStatus) bool {
	s.T.Helper()
	return status.Delivered()
}

func isPending(s *Sim, status *protocol.TransactionStatus) bool {
	s.T.Helper()

	if status.Code == 0 {
		return false
	}
	if status.Code != errors.StatusPending {
		s.T.Fatal("Expected transaction to be pending")
	}
	return true
}

func succeeds(s *Sim, status *protocol.TransactionStatus) bool {
	s.T.Helper()

	if !status.Delivered() {
		return false
	}
	if status.Failed() {
		s.T.Fatal("Expected transaction to succeeed")
	}
	return true
}

func fails(s *Sim, status *protocol.TransactionStatus) bool {
	s.T.Helper()

	if !status.Delivered() {
		return false
	}
	if !status.Failed() {
		s.T.Fatal("Expected transaction to fail")
	}
	return true
}

func received(s *Sim, status *protocol.TransactionStatus) bool {
	s.T.Helper()
	return status.Code != 0
}
