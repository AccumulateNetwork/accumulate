// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticLockedDeposit struct{}

var _ PrincipalValidator = (*SyntheticLockedDeposit)(nil)
var _ TransactionExecutorCleanup = (*SyntheticLockedDeposit)(nil)

func (SyntheticLockedDeposit) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticLockedDeposit
}

func (SyntheticLockedDeposit) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// Can create lite token accounts like SyntheticDepositTokens
	key, _, _ := protocol.ParseLiteTokenAddress(transaction.Header.Principal)
	return key != nil
}

func (x SyntheticLockedDeposit) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (SyntheticLockedDeposit) check(st *StateManager, tx *Delivery) (*protocol.SyntheticLockedDeposit, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticLockedDeposit)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticLockedDeposit), tx.Transaction.Body)
	}

	if body.Amount.Sign() < 0 {
		return nil, fmt.Errorf("amount can't be a negative value")
	}

	if len(body.Hash) == 0 {
		return nil, errors.BadRequest.With("hash cannot be empty")
	}

	// Validate hash length based on algorithm
	switch body.HashAlgorithm {
	case protocol.HashAlgorithmSHA256, protocol.HashAlgorithmSHA256D:
		if len(body.Hash) != 32 {
			return nil, errors.BadRequest.WithFormat("hash must be 32 bytes for %v, got %d", body.HashAlgorithm, len(body.Hash))
		}
	case protocol.HashAlgorithmHASH160:
		if len(body.Hash) != 20 {
			return nil, errors.BadRequest.WithFormat("hash must be 20 bytes for HASH160, got %d", len(body.Hash))
		}
	default:
		return nil, errors.BadRequest.WithFormat("unsupported hash algorithm: %v", body.HashAlgorithm)
	}

	if body.Expiration == nil {
		return nil, errors.BadRequest.With("expiration is required")
	}

	return body, nil
}

func (x SyntheticLockedDeposit) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Validate destination account exists or can be created (same logic as SyntheticDepositTokens)
	if st.Origin != nil {
		switch origin := st.Origin.(type) {
		case *protocol.LiteTokenAccount:
			if !origin.GetTokenUrl().Equal(body.Token) {
				return nil, fmt.Errorf("token type mismatch: want %s, got %s", origin.GetTokenUrl(), body.Token)
			}
		case *protocol.TokenAccount:
			if !origin.GetTokenUrl().Equal(body.Token) {
				return nil, fmt.Errorf("token type mismatch: want %s, got %s", origin.GetTokenUrl(), body.Token)
			}
		default:
			return nil, fmt.Errorf("invalid principal: want account type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeTokenAccount, origin.Type())
		}
	} else if keyHash, tok, err := protocol.ParseLiteTokenAddress(tx.Transaction.Header.Principal); err != nil {
		return nil, fmt.Errorf("invalid lite token account URL: %v", err)
	} else if keyHash == nil {
		return nil, errors.NotFound.WithFormat("could not find token account")
	} else if !body.Token.Equal(tok) {
		return nil, fmt.Errorf("token URL does not match lite token account URL")
	}

	// Register for automatic refund at expiration
	err = x.registerForExpiration(st, tx, body)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("register for expiration: %w", err)
	}

	// The locked deposit is recorded with a result tracking release status.
	// Tokens will be credited when ReleaseLockedOperation is executed with the correct preimage.
	// The result's ReleaseTxID will be set when released.
	result := &protocol.SyntheticLockedDepositResult{
		ReleaseTxID: nil, // Not yet released
	}
	return result, nil
}

// registerForExpiration schedules the locked deposit for automatic refund at expiration.
// This uses the same major block event system used for pending transaction expiration.
func (SyntheticLockedDeposit) registerForExpiration(st *StateManager, tx *Delivery, body *protocol.SyntheticLockedDeposit) error {
	// Load the anchor ledger to get current major block info
	var anchorLedger *protocol.AnchorLedger
	err := st.batch.Account(st.AnchorPool()).Main().GetAs(&anchorLedger)
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor ledger: %w", err)
	}

	currentMajor := anchorLedger.MajorBlockIndex
	currentMajorTime := anchorLedger.MajorBlockTime

	// Parse the major block schedule
	schedule, err := core.Cron.Parse(st.Globals.Globals.MajorBlockSchedule)
	if err != nil {
		// If parsing fails, default to next major block
		return st.recordExpiration(tx.Transaction.ID(), currentMajor+1)
	}

	// Calculate which major block the expiration falls on
	// Major blocks occur according to the cron schedule (typically every 12 hours)
	var count uint64 = 1
	max := st.Globals.Globals.Limits.PendingMajorBlocks
	if max == 0 {
		max = 14 // Default to 2 weeks worth
	}

	// Starting from the last major block time, count how many major blocks until expiration
	now := currentMajorTime
	for count < max && body.Expiration.After(schedule.Next(now)) {
		now = schedule.Next(now)
		count++
	}

	expirationMajor := currentMajor + count
	return st.recordExpiration(tx.Transaction.ID(), expirationMajor)
}

func (x SyntheticLockedDeposit) DidFail(state *ProcessTransactionState, transaction *protocol.Transaction) error {
	body, ok := transaction.Body.(*protocol.SyntheticLockedDeposit)
	if !ok {
		return fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticLockedDeposit), transaction.Body)
	}

	// Send tokens back to sender on failure
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

	return nil
}

// recordExpiration is a helper that adds a transaction to the expiration queue
// for a given major block. This is added to stateCache.
func (c *stateCache) recordExpiration(txid *url.TxID, majorBlock uint64) error {
	ledger := c.batch.Account(c.NodeUrl(protocol.Ledger))
	return ledger.Events().Major().Pending(majorBlock).Add(txid)
}
