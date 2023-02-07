// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SendTokens struct{}

func (SendTokens) Type() protocol.TransactionType { return protocol.TransactionTypeSendTokens }

func (SendTokens) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SendTokens{}).Validate(st, tx)
}

func (SendTokens) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SendTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SendTokens), tx.Transaction.Body)
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
		if to.Amount.Sign() < 0 {
			return nil, fmt.Errorf("amount can't be a negative value")
		}
		total.Add(total, &to.Amount)
	}

	if !account.DebitTokens(total) {
		return nil, fmt.Errorf("insufficient balance: have %v, want %v", account.TokenBalance(), total)
	}
	err := st.Update(account)
	if err != nil {
		return nil, fmt.Errorf("failed to update account %v: %v", account.GetUrl(), err)
	}

	m := make(map[[32]byte]bool)
	for i, to := range body.To {
		if to.Url == nil {
			return nil, errors.BadRequest.WithFormat("output %d is missing recipient URL", i)
		}
		id := to.Url.AccountID32()
		_, ok := m[id]
		if !ok {
			m[id] = true
		} else {
			return nil, fmt.Errorf("duplicate recipient passed in request")
		}
		deposit := new(protocol.SyntheticDepositTokens)
		deposit.Token = account.GetTokenUrl()
		deposit.Amount = to.Amount
		st.Submit(to.Url, deposit)
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
