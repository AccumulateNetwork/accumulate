// Copyright 2022 The Accumulate Authors
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

type Condition func(*Harness) bool

func Txn(id *url.TxID) *condTxn { return &condTxn{id} }

func (c condTxn) IsDelivered() Condition { return c.status(isDelivered) }
func (c condTxn) IsPending() Condition   { return c.status(isPending) }
func (c condTxn) Succeeds() Condition    { return c.status(succeeds) }
func (c condTxn) Fails() Condition       { return c.status(fails) }

func (c condTxn) Produced() condProduced { return condProduced(c) }

func (c condProduced) IsDelivered() Condition { return c.status(isDelivered) }
func (c condProduced) IsPending() Condition   { return c.status(isPending) }
func (c condProduced) Succeeds() Condition    { return c.status(succeeds) }
func (c condProduced) Fails() Condition       { return c.status(fails) }

type condTxn struct{ id *url.TxID }
type condProduced struct{ id *url.TxID }

func (c condTxn) status(predicate func(h *Harness, status *protocol.TransactionStatus) bool) Condition {
	return func(h *Harness) bool {
		h.tb.Helper()
		r, err := h.Query().QueryTransaction(context.Background(), c.id, nil)
		switch {
		case err == nil:
			r.Status.TxID = c.id
			return predicate(h, r.Status)
		case errors.Is(err, errors.NotFound):
			return false
		default:
			require.NoError(h.tb, err)
			panic("not reached")
		}
	}
}

func (c condProduced) status(predicate func(sim *Harness, status *protocol.TransactionStatus) bool) Condition {
	var produced []*api.TxIDRecord
	return func(h *Harness) bool {
		h.tb.Helper()

		if produced == nil {
			r, err := h.Query().QueryTransaction(context.Background(), c.id, nil)
			switch {
			case err == nil:
				produced = r.Produced.Records
			case errors.Is(err, errors.NotFound):
				return false
			default:
				require.NoError(h.tb, err)
				panic("not reached")
			}
		}

		if len(produced) == 0 {
			h.tb.Fatalf("%x did not produce transactions", c.id)
		}

		for _, r := range produced {
			h.tb.Helper()
			r, err := h.Query().QueryTransaction(context.Background(), r.Value, nil)
			switch {
			case err == nil:
				r.Status.TxID = r.TxID
				if !predicate(h, r.Status) {
					return false
				}
			case errors.Is(err, errors.NotFound):
				return false
			default:
				require.NoError(h.tb, err)
				panic("not reached")
			}
		}

		return true
	}
}

func isDelivered(h *Harness, status *protocol.TransactionStatus) bool {
	h.tb.Helper()
	return status.Delivered()
}

func isPending(h *Harness, status *protocol.TransactionStatus) bool {
	h.tb.Helper()

	if status.Code == 0 {
		return false
	}
	if status.Code != errors.Pending {
		h.tb.Fatal("Expected transaction to be pending")
	}
	return true
}

func succeeds(h *Harness, status *protocol.TransactionStatus) bool {
	h.tb.Helper()

	if !status.Delivered() {
		return false
	}
	if status.Failed() {
		h.tb.Fatal("Expected transaction to succeeed")
	}
	return true
}

func fails(h *Harness, status *protocol.TransactionStatus) bool {
	h.tb.Helper()

	if !status.Delivered() {
		return false
	}
	if !status.Failed() {
		h.tb.Fatal("Expected transaction to fail")
	}
	return true
}
