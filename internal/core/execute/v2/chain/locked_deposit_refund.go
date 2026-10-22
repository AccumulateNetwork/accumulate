// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// RefundLockedDeposit is the one rule for refunding a locked deposit (#4578):
// the deposit must be delivered, not released, and past its Expiration at now.
// It marks the deposit's status Expired, so that a release, the expiry event
// and a reclaim are all refused from then on, and produces the refund of the
// deposit's tokens to its sender. The expiry event and ReclaimLockedDeposit
// both apply it (#4613), so a lock is refunded at most once, and only ever to
// the sender.
//
// A deposit that cannot be refunded is refused with errors.NotReady if it has
// not expired yet, errors.Expired if it was already refunded, and
// errors.BadRequest if it was released or was never delivered. The status may
// be nil if the deposit has none.
func RefundLockedDeposit(batch *database.Batch, now time.Time, txid *url.TxID, body *protocol.SyntheticLockedDeposit, status *protocol.TransactionStatus, state *ProcessTransactionState) error {
	// Only a delivered deposit holds the funds. One that was refunded is
	// marked expired, and a released one records the release.
	switch {
	case status == nil:
		return errors.BadRequest.With("locked deposit has not been delivered")
	case status.Code == errors.Expired:
		return errors.Expired.With("locked deposit has been refunded")
	case status.Code != errors.Delivered:
		return errors.BadRequest.WithFormat("locked deposit has not been delivered: %v", status.Code)
	}
	if result, ok := status.Result.(*protocol.SyntheticLockedDepositResult); ok && result.ReleaseTxID != nil {
		return errors.BadRequest.With("locked deposit has already been released")
	}

	// Until the deposit has expired it can still be released
	if !now.After(*body.Expiration) {
		return errors.NotReady.With("locked deposit has not expired")
	}

	// Mark the deposit refunded, which refuses any later release, refund or
	// reclaim
	status.Code = errors.Expired
	err := batch.Transaction(txid.HashSlice()).Status().Put(status)
	if err != nil {
		return errors.UnknownError.WithFormat("update locked deposit status: %w", err)
	}

	ProduceLockedDepositRefund(state, body)
	return nil
}

// ProduceLockedDepositRefund produces the transaction that returns a locked
// deposit's tokens to its sender: a deposit of the tokens, or a burn if the
// sender is the issuer.
func ProduceLockedDepositRefund(state *ProcessTransactionState, body *protocol.SyntheticLockedDeposit) {
	if body.IsIssuer {
		refund := new(protocol.SyntheticBurnTokens)
		refund.Amount = body.Amount
		refund.IsRefund = true
		state.DidProduceTxn(body.Sender, refund)
	} else {
		refund := new(protocol.SyntheticDepositTokens)
		refund.Token = body.Token
		refund.Amount = body.Amount
		refund.IsRefund = true
		state.DidProduceTxn(body.Sender, refund)
	}
}
