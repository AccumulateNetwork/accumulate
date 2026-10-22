// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ReclaimLockedDeposit refunds a locked deposit that expired without being
// released, to its sender. It exists for locks whose expiry event fired before
// v2-kourou, when the event skipped every delivered deposit and was erased, so
// nothing would ever refund them (#4613). It applies the same rule as the
// expiry event, RefundLockedDeposit, so a lock is refunded at most once.
//
// The principal is the deposit's recipient, which is where the deposit's
// status lives. Anyone may submit a reclaim: it cannot name a payee, its only
// effect is the refund the protocol promises the sender at expiration, and the
// submitter only chooses when, after the expiration, that happens.
type ReclaimLockedDeposit struct{}

var _ PrincipalValidator = (*ReclaimLockedDeposit)(nil)
var _ SignerValidator = (*ReclaimLockedDeposit)(nil)
var _ SignerCanSignValidator = (*ReclaimLockedDeposit)(nil)

func (ReclaimLockedDeposit) Type() protocol.TransactionType {
	return protocol.TransactionTypeReclaimLockedDeposit
}

func (ReclaimLockedDeposit) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// A lite recipient's account is created by the release, so it may not
	// exist
	key, _, _ := protocol.ParseLiteTokenAddress(transaction.Header.Principal)
	return key != nil
}

func (ReclaimLockedDeposit) SignerCanSign(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer) (fallback bool, err error) {
	// Anyone may submit a reclaim, so a lite identity may sign for a principal
	// that is not its own
	if _, ok := signer.(*protocol.LiteIdentity); ok {
		return false, nil
	}

	// Fallback
	return true, nil
}

func (ReclaimLockedDeposit) AuthorityIsAccepted(AuthDelegate, *database.Batch, *protocol.Transaction, *protocol.AuthoritySignature) (fallback bool, err error) {
	// Anyone may submit a reclaim
	return false, nil
}

func (ReclaimLockedDeposit) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	// Anyone may submit a reclaim
	return true, false, nil
}

func (x ReclaimLockedDeposit) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	// Validate only performs basic checks on the transaction body.
	// Database-dependent checks are done in Execute.
	_, err := x.check(st, tx)
	return nil, err
}

func (ReclaimLockedDeposit) check(st *StateManager, tx *Delivery) (*protocol.ReclaimLockedDeposit, error) {
	// Not enabled until Kourou
	if !st.Globals.ExecutorVersion.V2KourouEnabled() {
		return nil, errors.BadRequest.WithFormat("unknown transaction type %v", tx.Transaction.Body.Type())
	}

	body, ok := tx.Transaction.Body.(*protocol.ReclaimLockedDeposit)
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid payload: want %T, got %T", new(protocol.ReclaimLockedDeposit), tx.Transaction.Body)
	}

	if body.LockedTxID == nil {
		return nil, errors.BadRequest.With("locked transaction ID is required")
	}

	return body, nil
}

func (x ReclaimLockedDeposit) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Load the locked deposit
	var lockedMsg messaging.MessageWithTransaction
	err = st.batch.Message(body.LockedTxID.Hash()).Main().GetAs(&lockedMsg)
	if err != nil {
		return nil, errors.NotFound.WithFormat("load locked transaction: %w", err)
	}

	lockedTx := lockedMsg.GetTransaction()
	if lockedTx == nil {
		return nil, errors.NotFound.With("locked transaction body is nil")
	}

	lockedBody, ok := lockedTx.Body.(*protocol.SyntheticLockedDeposit)
	if !ok {
		return nil, errors.BadRequest.WithFormat("referenced transaction is not a locked deposit: got %T", lockedTx.Body)
	}

	// The reclaim is addressed to the deposit's recipient, so that it executes
	// on the partition that holds the deposit's status
	if !tx.Transaction.Header.Principal.Equal(lockedTx.Header.Principal) {
		return nil, errors.BadRequest.WithFormat("principal must be the locked deposit's recipient, %v", lockedTx.Header.Principal)
	}

	// Load the locked deposit's status
	status, err := st.batch.Transaction(body.LockedTxID.HashSlice()).Status().Get()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.UnknownError.WithFormat("load locked deposit status: %w", err)
	}

	// The expiration is measured from the block's time, the system ledger's
	// timestamp, as the release and the expiry event measure it
	var ledger *protocol.SystemLedger
	err = st.LoadUrlAs(st.NodeUrl(protocol.Ledger), &ledger)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	// Refund the sender, if the deposit is refundable
	err = RefundLockedDeposit(st.batch, ledger.Timestamp, body.LockedTxID, lockedBody, status, &st.State)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return nil, nil
}
