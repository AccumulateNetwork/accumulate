// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package harness

import (
	"context"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A Condition is a function that is used by Harness.StepUntil to wait until
// some condition is met.
type Condition func(*Harness) bool

// Txn defines a condition on a transaction.
func Txn(id *url.TxID) *condTxn { return &condTxn{id} }

// Produced defines a condition on the synthetic transaction(s) produced by a
// transaction.
func (c condTxn) Produced() condProduced { return condProduced(c) }

// Refund defines a condition on the refund transaction(s) produced by the
// synthetic transaction(s) produced by a transaction.
func (c condTxn) Refund() condRefund { return condRefund(c) }

// Received waits until the transaction has been received.
func (c condTxn) Received() Condition { return c.status(received) }

// IsDelivered waits until the transaction has been delivered (executed, whether
// success or failure).
func (c condTxn) IsDelivered() Condition { return c.status(isDelivered) }

// IsPending waits until the transaction is pending (received but not executed).
// IsPending will fail if the transaction has been recorded with any status
// other than pending.
func (c condTxn) IsPending() Condition { return c.status(isPending) }

// Succeeds waits until the transaction has been delivered and succeeds if the
// transaction succeeded (and fails otherwise).
func (c condTxn) Succeeds() Condition { return c.status(succeeds) }

// Fails waits until the transaction has been delivered and succeeds if the
// transaction failed (and fails otherwise).
func (c condTxn) Fails() Condition { return c.status(fails) }

// Fails waits until the transaction has been delivered and succeeds if the
// transaction failed with the given code (and fails otherwise).
func (c condTxn) FailsWithCode(code errors.Status) Condition {
	return c.status(failsWithCode(code))
}

// Received waits until the transaction has been received.
func (c condProduced) Received() Condition { return c.status(received) }

// IsDelivered waits until the produced transaction(s) have been delivered
// (executed, whether success or failure).
func (c condProduced) IsDelivered() Condition { return c.status(isDelivered) }

// IsPending waits until the produced transaction(s) are pending (received but
// not executed). IsPending will fail if the transaction(s) have been recorded
// with any status other than pending.
func (c condProduced) IsPending() Condition { return c.status(isPending) }

// Succeeds waits until the produced transaction(s) have been delivered and
// succeeds if the transaction(s) succeeded (and fails otherwise).
func (c condProduced) Succeeds() Condition { return c.status(succeeds) }

// Fails waits until the produced transaction(s) have been delivered and
// succeeds if the transaction(s) failed (and fails otherwise).
func (c condProduced) Fails() Condition { return c.status(fails) }

// Fails waits until the produced transaction(s) have been delivered and
// succeeds if the transaction(s) failed with the given code (and fails
// otherwise).
func (c condProduced) FailsWithCode(code errors.Status) Condition {
	return c.status(failsWithCode(code))
}

// Received waits until the transaction has been received.
func (c condRefund) Received() Condition { return c.status(received) }

// IsDelivered waits until the refund transaction(s) have been delivered
// (executed, whether success or failure).
func (c condRefund) IsDelivered() Condition { return c.status(isDelivered) }

// IsPending waits until the refund transaction(s) are pending (received but not
// executed). IsPending will fail if the transaction(s) have been recorded with
// any status other than pending.
func (c condRefund) IsPending() Condition { return c.status(isPending) }

// Succeeds waits until the refund transaction(s) have been delivered and
// succeeds if the transaction(s) succeeded (and fails otherwise).
func (c condRefund) Succeeds() Condition { return c.status(succeeds) }

// Fails waits until the refund transaction(s) have been delivered and succeeds
// if the transaction(s) failed (and fails otherwise).
func (c condRefund) Fails() Condition { return c.status(fails) }

// Fails waits until the refund transaction(s) have been delivered and succeeds
// if the transaction(s) failed with the given code (and fails otherwise).
func (c condRefund) FailsWithCode(code errors.Status) Condition {
	return c.status(failsWithCode(code))
}

type condTxn struct{ id *url.TxID }
type condProduced struct{ id *url.TxID }
type condRefund struct{ id *url.TxID }

func (c condTxn) status(predicate func(h *Harness, c any, status *protocol.TransactionStatus) bool) Condition {
	return func(h *Harness) bool {
		// Query the transaction
		h.TB.Helper()
		status, _, ok := queryStatusAndProduced(h, c.id)
		if !ok {
			return false // Wait
		}

		// Evaluate the predicate
		status.TxID = c.id
		return predicate(h, c, status)
	}
}

func (c condProduced) status(predicate func(h *Harness, c any, status *protocol.TransactionStatus) bool) Condition {
	var produced []*api.TxIDRecord
	return func(h *Harness) bool {
		h.TB.Helper()

		// Wait for the transaction to resolve
		if produced == nil {
			status, p, ok := queryStatusAndProduced(h, c.id)
			if !ok {
				return false // Wait
			}

			// If the transaction is pending, wait
			if !status.Delivered() {
				return false
			}

			// Record the produced transactions
			produced = p
		}

		// Expect produced transactions
		if len(produced) == 0 {
			h.TB.Fatalf("%v did not produce transactions", c.id)
		}

		// Wait for the produced transactions to be received
		for _, r := range produced {
			status, _, ok := queryStatusAndProduced(h, r.Value)
			if !ok {
				return false // Wait
			}

			// Evaluate the predicate
			status.TxID = r.Value
			if !predicate(h, c, status) {
				return false
			}
		}

		// All predicates passed
		return true
	}
}

func (c condRefund) status(predicate func(h *Harness, c any, status *protocol.TransactionStatus) bool) Condition {
	var produced []*api.TxIDRecord
	var refund [][]*api.TxIDRecord
	var refundCount int
	return func(h *Harness) bool {
		h.TB.Helper()

		// Wait for the transaction to resolve
		if produced == nil {
			status, p, ok := queryStatusAndProduced(h, c.id)
			if !ok {
				return false // Wait
			}

			// If the transaction is pending, wait
			if !status.Delivered() {
				return false
			}

			// Record the produced transactions
			produced = p
			refund = make([][]*api.TxIDRecord, len(p))
		}

		// Expect produced transactions
		if len(produced) == 0 {
			h.TB.Fatalf("%v did not produce transactions", c.id)
		}

		// Wait for the produced transactions to complete
		for i, r := range produced {
			if refund[i] == nil {
				status, p, ok := queryStatusAndProduced(h, r.Value)
				if !ok {
					return false // Wait
				}

				// If the transaction is pending, wait
				if !status.Delivered() {
					return false
				}

				// Record the refund transactions
				refund[i] = p
				refundCount += len(p)
			}
		}

		// Expect refunds transactions
		if refundCount == 0 {
			h.TB.Fatalf("%v did not produce refunds", c.id)
		}

		// Wait for the refund transactions to be received
		for _, r := range refund {
			for _, r := range r {
				status, _, ok := queryStatusAndProduced(h, r.Value)
				if !ok {
					return false // Wait
				}

				// Evaluate the predicate
				status.TxID = r.Value
				if !predicate(h, c, status) {
					return false
				}
			}
		}

		// All predicates passed
		return true
	}
}

func queryStatusAndProduced(h *Harness, id *url.TxID) (*protocol.TransactionStatus, []*api.TxIDRecord, bool) {
	h.TB.Helper()
	r, err := h.Query().Query(context.Background(), id.AsUrl(), new(api.DefaultQuery))
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil, nil, false

	default:
		// Unknown error
		require.NoError(h.TB, err)
		panic("not reached")
	}

	switch r := r.(type) {
	case *api.SignatureRecord:
		return r.Status, r.Produced.Records, true
	case *api.TransactionRecord:
		return r.Status, r.Produced.Records, true
	default:
		h.TB.Fatalf("Unsupported record type %v", r.RecordType())
		panic("not reached")
	}
}

func received(h *Harness, _ any, status *protocol.TransactionStatus) bool {
	h.TB.Helper()
	return status.Code != 0
}

func isDelivered(h *Harness, _ any, status *protocol.TransactionStatus) bool {
	h.TB.Helper()
	return status.Delivered()
}

func isPending(h *Harness, c any, status *protocol.TransactionStatus) bool {
	h.TB.Helper()

	// Check if the transaction is recorded as pending
	if status.Code != 0 {
		// Must be pending
		if status.Code == errors.Pending {
			return true
		}
		switch c.(type) {
		case condProduced:
			h.TB.Fatal("Expected produced transaction to be pending")
		default:
			h.TB.Fatal("Expected transaction to be pending")
		}
	}

	// Check if the account lists the transaction as pending
	r, err := h.Query().QueryPendingIds(context.Background(), status.TxID.Account(), nil)
	switch {
	case err == nil:
		for _, r := range r.Records {
			if r.Value.Hash() == status.TxID.Hash() {
				return true
			}
		}

	case !errors.Is(err, errors.NotFound):
		// Unknown error
		require.NoError(h.TB, err)
		panic("not reached")
	}

	return false
}

func succeeds(h *Harness, c any, status *protocol.TransactionStatus) bool {
	h.TB.Helper()

	// Wait for delivery
	if !status.Delivered() {
		return false
	}

	// Must be success
	if status.Failed() {
		switch c.(type) {
		case condProduced:
			h.TB.Fatal("Expected produced transaction to succeed")
		default:
			h.TB.Fatal("Expected transaction to succeed")
		}
	}
	return true
}

func fails(h *Harness, c any, status *protocol.TransactionStatus) bool {
	h.TB.Helper()

	// Wait for delivery
	if !status.Delivered() {
		return false
	}

	// Must be failure
	if !status.Failed() {
		switch c.(type) {
		case condProduced:
			h.TB.Fatal("Expected produced transaction to fail")
		default:
			h.TB.Fatal("Expected transaction to fail")
		}
	}
	return true
}

func failsWithCode(code errors.Status) func(h *Harness, c any, status *protocol.TransactionStatus) bool {
	return func(h *Harness, c any, status *protocol.TransactionStatus) bool {
		h.TB.Helper()

		if status.Code != 0 {
			print("")
		}

		// Wait for delivery
		if !status.Delivered() {
			return false
		}

		// Must be failure
		if !status.Failed() {
			switch c.(type) {
			case condProduced:
				h.TB.Fatal("Expected produced transaction to fail")
			default:
				h.TB.Fatal("Expected transaction to fail")
			}
		}

		if status.Code != code {
			h.TB.Fatalf("Expected code %v, got %v", code, status.Code)
		}
		return true
	}
}
