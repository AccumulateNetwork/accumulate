// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateMiningTokenAccount struct{}

var _ SignerValidator = (*CreateMiningTokenAccount)(nil)

func (CreateMiningTokenAccount) Type() protocol.TransactionType {
	return protocol.TransactionTypeCreateMiningTokenAccount
}

func (CreateMiningTokenAccount) AuthorityIsAccepted(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, sig *protocol.AuthoritySignature) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateMiningTokenAccount)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateMiningTokenAccount), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).AuthorityIsAccepted(delegate, batch, transaction, sig)
}

func (CreateMiningTokenAccount) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateMiningTokenAccount)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateMiningTokenAccount), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).TransactionIsReady(delegate, batch, transaction)
}

func (x CreateMiningTokenAccount) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (CreateMiningTokenAccount) check(st *StateManager, tx *Delivery) (*protocol.CreateMiningTokenAccount, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateMiningTokenAccount)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.CreateMiningTokenAccount), tx.Transaction.Body)
	}

	if body.Url == nil {
		return nil, errors.BadRequest.WithFormat("account URL is missing")
	}

	if body.TokenUrl == nil {
		return nil, errors.BadRequest.WithFormat("token URL is missing")
	}

	if body.MinerADI == nil {
		return nil, errors.BadRequest.WithFormat("miner ADI URL is missing")
	}

	for _, u := range body.Authorities {
		if u == nil {
			return nil, errors.BadRequest.WithFormat("authority URL is nil")
		}
	}

	err := originIsParent(tx, body.Url)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Validate that the token issuer exists
	// This follows the same pattern as CreateTokenAccount
	if !body.TokenUrl.Equal(protocol.AcmeUrl()) {
		// If the issuer and principal are not local to each other, we would need proof
		// For simplicity, we'll only allow ACME tokens or local tokens for now
		if !tx.Transaction.Header.Principal.LocalTo(body.TokenUrl) {
			return nil, errors.BadRequest.With("token issuer must be local or ACME")
		}
	}

	return body, nil
}

func (CreateMiningTokenAccount) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := CreateMiningTokenAccount{}.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Create the mining token account
	account := &protocol.MiningTokenAccount{
		Url:                body.Url,
		TokenUrl:           body.TokenUrl,
		MinerADI:           body.MinerADI,
		ActiveEpoch:        0, // Start with no active epoch
		TotalSubmissions:   0,
		AutoParticipate:    body.AutoParticipate != nil && *body.AutoParticipate,
		MaxCreditsPerEpoch: 0, // Default to unlimited
	}

	if body.MaxCreditsPerEpoch != nil {
		account.MaxCreditsPerEpoch = *body.MaxCreditsPerEpoch
	}

	// Initialize balance to zero
	account.Balance.SetInt64(0)
	account.TotalRewards.SetInt64(0)

	err = st.Create(account)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Add authorities if specified
	if len(body.Authorities) > 0 {
		err = updateAccountAuth(st, account, func(auth *protocol.AccountAuth) error {
			for _, authority := range body.Authorities {
				auth.Authorities = append(auth.Authorities, &protocol.AuthorityEntry{Url: authority})
			}
			return nil
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	return nil, nil
}
