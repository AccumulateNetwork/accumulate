// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package harness

import (
	"context"
	"fmt"
	"strings"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A Condition is a function that is used by Harness.StepUntil to wait until
// some condition is met.
type Condition interface {
	Satisfied(*Harness) bool
	String() string
}

type True func(*Harness) bool

func (f True) Satisfied(h *Harness) bool { return f(h) }

func (f True) String() string { return "(unknown predicate function)" }

// Txn defines a condition on a transaction.
func Txn(id *url.TxID) txnCond { return txnCond{msgCond{id: id, message: []string{"transaction"}}} }

// Sig defines a condition on a signature.
func Sig(id *url.TxID) sigCond { return sigCond{msgCond{id: id, message: []string{"signature"}}} }

// txnCond provides methods to define conditions on a transaction.
type txnCond struct{ msgCond }

// Produced defines a condition on the synthetic transaction(s) produced by a
// transaction.
func (c txnCond) Produced() msgCond { return c.with("produced", deliveredThen, produced) }

// Refund defines a condition on the refund transaction(s) produced by the
// synthetic transaction(s) produced by a transaction.
func (c txnCond) Refund() msgCond {
	return c.with("refund", deliveredThen, produced, deliveredThen, produced)
}

// sigCond provides methods to define conditions on a signature.
type sigCond struct{ msgCond }

// CreditPayment defines a condition on the credit payment produced by a
// signature.
func (c sigCond) CreditPayment() msgCond {
	return c.with("credit payment", deliveredThen, produced2(func(r *msgResult) bool {
		_, ok := messaging.UnwrapAs[*messaging.CreditPayment](r.Message)
		return ok
	}))
}

// SignatureRequest defines a condition on signature requests produced by a
// signature.
func (c sigCond) SignatureRequest() sigCond2 {
	return sigCond2{c.with("signature request", deliveredThen, produced2(func(r *msgResult) bool {
		_, ok := messaging.UnwrapAs[*messaging.SignatureRequest](r.Message)
		return ok
	}))}
}

// AuthoritySignature defines a condition on the authority signature produced by
// a signature.
func (c sigCond) AuthoritySignature() sigCond2 {
	return sigCond2{c.with("authority signature", deliveredThen, produced2(func(r *msgResult) bool {
		msg, ok := messaging.UnwrapAs[*messaging.UserSignature](r.Message)
		return ok && msg.Signature.Type() == protocol.SignatureTypeAuthority
	}))}
}

// sigCond2 provides methods to define conditions on a signature request or
// authority signature.
type sigCond2 struct{ msgCond }

// Produced defines a condition on messages produced by a signature request or
// authority signature.
func (c sigCond2) Produced() sigCond2 { return sigCond2{c.with("produced", deliveredThen, produced)} }

// msgCond provides methods to define conditions on a message.
type msgCond struct {
	id        *url.TxID
	modifiers []predicateModifier
	message   []string
}

func (c msgCond) with(message string, mod ...predicateModifier) msgCond {
	// Reverse the modifiers since nesting is the inverse of the natural order
	for i, n := 0, len(mod); i < n/2; i++ {
		mod[i], mod[n-i-1] = mod[n-i-1], mod[i]
	}

	parts := make([]string, len(c.message)+1)
	n := copy(parts, c.message)
	parts[n] = message

	return msgCond{
		id:        c.id,
		modifiers: append(mod, c.modifiers...),
		message:   parts,
	}
}

func (c msgCond) make(message string, predicate statusPredicate) Condition {
	parts := make([]string, len(c.message)+1)
	n := copy(parts, c.message)
	parts[n] = message
	for _, mod := range c.modifiers {
		predicate = mod(predicate)
	}
	return newMessageCond(c.id, predicate, parts)
}

// Received waits until the transaction has been received.
func (c msgCond) Received() Condition { return c.make("is received", received) }

// IsDelivered waits until the transaction has been delivered (executed, whether
// success or failure).
func (c msgCond) IsDelivered() Condition { return c.make("is delivered", isDelivered) }

// IsPending waits until the transaction is pending (received but not executed).
// IsPending will fail if the transaction has been recorded with any status
// other than pending.
func (c msgCond) IsPending() Condition { return c.make("is pending", isPending) }

// Succeeds waits until the transaction has been delivered and succeeds if the
// transaction succeeded (and fails otherwise).
func (c msgCond) Succeeds() Condition { return c.make("succeeds", deliveredThen(succeeds)) }

// Fails waits until the transaction has been delivered and succeeds if the
// transaction failed (and fails otherwise).
func (c msgCond) Fails() Condition { return c.make("fails", deliveredThen(fails)) }

// Fails waits until the transaction has been delivered and succeeds if the
// transaction failed with the given code (and fails otherwise).
func (c msgCond) FailsWithCode(code errors.Status) Condition {
	return c.make("fails", failsWithCode(code))
}

type msgResult struct {
	Message  messaging.Message
	Status   *protocol.TransactionStatus
	Produced []*url.TxID
}

type statusPredicate func(h *Harness, c *condition, r *msgResult) bool

type predicateModifier func(statusPredicate) statusPredicate

type condition struct {
	predicate  statusPredicate
	lastResult *msgResult
	message    []string
}

func (c *condition) String() string {
	return strings.Join(c.message, " ")
}

func (c *condition) Satisfied(h *Harness) bool {
	return c.predicate(h, c, c.lastResult)
}

func (c *condition) replace(h *Harness, new statusPredicate) bool {
	c.predicate = new
	return c.Satisfied(h)
}

func getMessageResult(h *Harness, id *url.TxID) (*msgResult, bool) {
	// Query the transaction
	h.TB.Helper()
	qr, err := h.Query().Query(context.Background(), id.AsUrl(), new(api.DefaultQuery))
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil, false

	default:
		// Unknown error
		require.NoError(h.TB, err)
		panic("not reached")
	}

	// Convert the result
	var res msgResult
	var produced []*api.TxIDRecord
	switch qr := qr.(type) {
	case *api.SignatureRecord:
		res.Status, produced = qr.Status, qr.Produced.Records
		res.Message = &messaging.UserSignature{Signature: qr.Signature}
	case *api.TransactionRecord:
		res.Status, produced = qr.Status, qr.Produced.Records
		if qr.Message != nil {
			res.Message = qr.Message
		} else {
			res.Message = &messaging.UserTransaction{Transaction: qr.Transaction}
		}
	default:
		h.TB.Fatalf("Unsupported record type %v", qr.RecordType())
		panic("not reached")
	}

	res.Status.TxID = id
	res.Produced = make([]*url.TxID, len(produced))
	for i, v := range produced {
		res.Produced[i] = v.Value
	}

	return &res, true
}

func newMessageCond(id *url.TxID, predicate statusPredicate, message []string) *condition {
	// parts := make([]string, len(message)+1)
	// n := copy(parts, message)
	// h := id.Hash()
	// parts[n] = fmt.Sprintf("(%x@%s)", h[:4], id.Account())

	return &condition{predicate: waitFor(id)(predicate), message: message}
}

func waitFor(id *url.TxID) predicateModifier {
	return func(predicate statusPredicate) statusPredicate {
		return func(h *Harness, c *condition, _ *msgResult) bool {
			// Wait for the message
			h.TB.Helper()
			r, ok := getMessageResult(h, id)
			if !ok {
				return false
			}

			c.lastResult = r

			// Evaluate the predicate (only replace if the status is final)
			if r.Status.Delivered() {
				return c.replace(h, predicate)
			}
			return predicate(h, c, r)
		}
	}
}

func deliveredThen(predicate statusPredicate) statusPredicate {
	return func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()

		// If the transaction is pending, wait
		if !r.Status.Delivered() {
			return false
		}

		return c.replace(h, predicate)
	}
}

func produced(predicate statusPredicate) statusPredicate {
	return func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()

		// Expect produced transactions
		if len(r.Produced) == 0 {
			h.TB.Fatalf("%v did not produce transactions", r.Status.TxID)
		}

		conditions := make([]*condition, len(r.Produced))
		for i, id := range r.Produced {
			conditions[i] = newMessageCond(id, predicate, c.message)
		}

		if len(conditions) == 1 {
			return c.replace(h, conditions[0].predicate)
		}

		return c.replace(h, func(h *Harness, _ *condition, _ *msgResult) bool {
			ok := true
			for _, c := range conditions {
				if !c.Satisfied(h) {
					ok = false
				}
			}
			return ok
		})
	}
}

func produced2(filter func(*msgResult) bool) predicateModifier {
	return func(predicate statusPredicate) statusPredicate {
		return func(h *Harness, c *condition, r *msgResult) bool {
			h.TB.Helper()

			// Expect produced transactions
			if len(r.Produced) == 0 {
				h.TB.Fatalf("%v did not produce transactions", r.Status.TxID)
			}

			// For each produced
			for _, id := range r.Produced {
				// Wait for it
				r, ok := getMessageResult(h, id)
				if !ok {
					continue
				}

				// Is it the one we want?
				if !filter(r) {
					continue
				}

				// Found it, delegate to the predicate
				return c.replace(h, waitFor(id)(predicate))
			}

			// The message we want hasn't appeared yet
			return false
		}
	}
}

func received(h *Harness, c *condition, r *msgResult) bool {
	return c.replace(h, func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()
		return r.Status.Code != 0
	})
}

func isDelivered(h *Harness, c *condition, r *msgResult) bool {
	return c.replace(h, func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()
		return r.Status.Delivered()
	})
}

func isPending(h *Harness, c *condition, r *msgResult) bool {
	h.TB.Helper()

	// Check if the transaction is recorded as pending
	if r.Status.Code != 0 {
		// Must be pending
		if r.Status.Code == errors.Pending {
			return true
		}
		h.TB.Fatal("Expected transaction to be pending")
	}

	// Check if the account lists the transaction as pending
	qr, err := h.Query().QueryPendingIds(context.Background(), r.Status.TxID.Account(), nil)
	switch {
	case err == nil:
		for _, qr := range qr.Records {
			if qr.Value.Hash() == r.Status.TxID.Hash() {
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

func succeeds(h *Harness, c *condition, r *msgResult) bool {
	h.TB.Helper()

	// Must be success
	if r.Status.Failed() {
		h.TB.Fatal("Expected transaction to succeed")
	}
	if h.VerboseConditions {
		fmt.Println(c, "✔")
	}
	return c.replace(h, func(h *Harness, c *condition, r *msgResult) bool { return true })
}

func fails(h *Harness, c *condition, r *msgResult) bool {
	h.TB.Helper()

	// Must be failure
	if !r.Status.Failed() {
		h.TB.Fatal("Expected transaction to fail")
	}
	return true
}

func failsWithCode(code errors.Status) statusPredicate {
	return func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()

		// Must be failure
		if !r.Status.Failed() {
			h.TB.Fatal("Expected transaction to fail")
		}

		if r.Status.Code != code {
			h.TB.Fatalf("Expected code %v, got %v", code, r.Status.Code)
		}
		return true
	}
}
