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

type CreateMinedIssuanceAccount struct{}

var _ SignerValidator = (*CreateMinedIssuanceAccount)(nil)

func (CreateMinedIssuanceAccount) Type() protocol.TransactionType {
	return protocol.TransactionTypeCreateMinedIssuanceAccount
}

func (CreateMinedIssuanceAccount) AuthorityIsAccepted(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, sig *protocol.AuthoritySignature) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateMinedIssuanceAccount)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateMinedIssuanceAccount), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).AuthorityIsAccepted(delegate, batch, transaction, sig)
}

func (CreateMinedIssuanceAccount) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateMinedIssuanceAccount)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateMinedIssuanceAccount), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).TransactionIsReady(delegate, batch, transaction)
}

func (x CreateMinedIssuanceAccount) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (CreateMinedIssuanceAccount) check(st *StateManager, tx *Delivery) (*protocol.CreateMinedIssuanceAccount, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateMinedIssuanceAccount)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.CreateMinedIssuanceAccount), tx.Transaction.Body)
	}

	if body.Url == nil {
		return nil, errors.BadRequest.WithFormat("account URL is missing")
	}

	if body.TokenUrl == nil {
		return nil, errors.BadRequest.WithFormat("token URL is missing")
	}

	if body.TopNSize == 0 {
		return nil, errors.BadRequest.WithFormat("topNSize must be greater than 0")
	}

	if body.SubmissionWindow == 0 {
		return nil, errors.BadRequest.WithFormat("submissionWindow must be greater than 0")
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
	if !body.TokenUrl.Equal(protocol.AcmeUrl()) {
		// If the issuer and principal are not local to each other, we would need proof
		if !tx.Transaction.Header.Principal.LocalTo(body.TokenUrl) {
			return nil, errors.BadRequest.With("token issuer must be local or ACME")
		}
	}

	return body, nil
}

func (CreateMinedIssuanceAccount) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := CreateMinedIssuanceAccount{}.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Create the mined issuance account
	account := &protocol.MinedIssuanceAccount{
		Url:                 body.Url,
		TokenUrl:            body.TokenUrl,
		TotalRewardPool:     body.TotalRewardPool,
		RewardsPerWinner:    body.RewardsPerWinner,
		TopNSize:            body.TopNSize,
		SubmissionWindow:    body.SubmissionWindow,
		CurrentEpoch:        nil, // No current epoch initially
		EpochHistory:        []*protocol.MiningEpoch{},
		TotalEpochs:         0,
		TotalMinersRewarded: 0,
	}

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
