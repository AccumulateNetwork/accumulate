// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SendTokens struct{}

func (SendTokens) Type() protocol.TransactionType { return protocol.TransactionTypeSendTokens }

func (x SendTokens) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (SendTokens) check(st *StateManager, tx *Delivery) (*protocol.SendTokens, error) {
	body, ok := tx.Transaction.Body.(*protocol.SendTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SendTokens), tx.Transaction.Body)
	}

	for i, to := range body.To {
		if to.Url == nil {
			return nil, errors.BadRequest.WithFormat("output %d is missing recipient URL", i)
		}
		if to.Amount.Sign() < 0 {
			return nil, fmt.Errorf("amount can't be a negative value")
		}
	}

	// Validate hashlock if present
	if tx.Transaction.Header.HashLock != nil {
		lock := tx.Transaction.Header.HashLock
		if len(lock.Hash) == 0 {
			return nil, errors.BadRequest.With("hashlock hash cannot be empty")
		}
		// Validate hash length based on algorithm
		switch lock.HashAlgorithm {
		case protocol.HashAlgorithmSHA256, protocol.HashAlgorithmSHA256D:
			if len(lock.Hash) != 32 {
				return nil, errors.BadRequest.WithFormat("hashlock hash must be 32 bytes for %v, got %d", lock.HashAlgorithm, len(lock.Hash))
			}
		case protocol.HashAlgorithmHASH160:
			if len(lock.Hash) != 20 {
				return nil, errors.BadRequest.WithFormat("hashlock hash must be 20 bytes for HASH160, got %d", len(lock.Hash))
			}
		default:
			return nil, errors.BadRequest.WithFormat("unsupported hashlock algorithm: %v", lock.HashAlgorithm)
		}
		if lock.Expiration == nil {
			return nil, errors.BadRequest.With("hashlock expiration is required")
		}
		// Minimum 10 minutes in the future
		if time.Until(*lock.Expiration) < 10*time.Minute {
			return nil, errors.BadRequest.With("hashlock expiration must be at least 10 minutes in the future")
		}
		// Maximum 30 days
		if time.Until(*lock.Expiration) > 30*24*time.Hour {
			return nil, errors.BadRequest.With("hashlock expiration cannot exceed 30 days")
		}
	}

	return body, nil
}

func (x SendTokens) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	var account protocol.AccountWithTokens
	switch origin := st.Origin.(type) {
	case *protocol.TokenAccount:
		account = origin
	case *protocol.LiteTokenAccount:
		account = origin
	default:
		return nil, fmt.Errorf("invalid principal: want %v or %v, got %v", protocol.AccountTypeTokenAccount, protocol.AccountTypeLiteTokenAccount, st.Origin.Type())
	}

	//now check to see if we can transact
	//really only need to provide one input...
	//now check to see if the account is good to send tokens from
	total := new(big.Int)
	for _, to := range body.To {
		total.Add(total, &to.Amount)
	}

	if !account.DebitTokens(total) {
		return nil, fmt.Errorf("insufficient balance: have %v, want %v", account.TokenBalance(), total)
	}
	err = st.Update(account)
	if err != nil {
		return nil, fmt.Errorf("failed to update account %v: %v", account.GetUrl(), err)
	}

	for _, to := range body.To {
		if tx.Transaction.Header.HashLock != nil {
			// Produce locked deposit when hashlock is present
			locked := new(protocol.SyntheticLockedDeposit)
			locked.Token = account.GetTokenUrl()
			locked.Amount = to.Amount
			locked.Sender = tx.Transaction.Header.Principal
			locked.HashAlgorithm = tx.Transaction.Header.HashLock.HashAlgorithm
			locked.Hash = tx.Transaction.Header.HashLock.Hash
			locked.Expiration = tx.Transaction.Header.HashLock.Expiration
			// Check if sender is token issuer
			if issuer, ok := st.Origin.(*protocol.TokenIssuer); ok {
				locked.IsIssuer = issuer.GetUrl().Equal(account.GetTokenUrl())
			}
			st.Submit(to.Url, locked)
		} else {
			// Normal deposit
			deposit := new(protocol.SyntheticDepositTokens)
			deposit.Token = account.GetTokenUrl()
			deposit.Amount = to.Amount
			st.Submit(to.Url, deposit)
		}
	}

	// Is the account locked?
	lockable, ok := account.(protocol.LockableAccount)
	if !ok || lockable.GetLockHeight() == 0 {
		return nil, nil // No
	}

	entry, err := indexing.LoadIndexEntryFromEnd(st.batch.Account(st.AnchorPool()).MajorBlockChain(), 1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load major block index: %w", err)
	}

	// If there's no entry there's no major block so we cannot have reached the
	// threshold
	if entry == nil {
		return nil, errors.NotAllowed.WithFormat("account is locked until major block %d (currently at 0)", lockable.GetLockHeight())
	}

	if entry.BlockIndex < lockable.GetLockHeight() {
		return nil, errors.NotAllowed.WithFormat("account is locked until major block %d (currently at %d)", lockable.GetLockHeight(), entry.BlockIndex)
	}

	return nil, nil
}
