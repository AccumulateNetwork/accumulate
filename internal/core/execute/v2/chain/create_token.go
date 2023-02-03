// Copyright 2023 The Accumulate Authors
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

type CreateToken struct{}

var _ SignerValidator = (*CreateToken)(nil)

func (CreateToken) Type() protocol.TransactionType { return protocol.TransactionTypeCreateToken }

func (CreateToken) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, md SignatureValidationMetadata) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateToken)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateToken), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).SignerIsAuthorized(delegate, batch, transaction, signer, md)
}

func (CreateToken) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateToken)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateToken), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).TransactionIsReady(delegate, batch, transaction, status)
}

func (CreateToken) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (CreateToken{}).Validate(st, tx)
}

func (CreateToken) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateToken)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateToken), tx.Transaction.Body)
	}

	if body.Url == nil {
		return nil, errors.BadRequest.WithFormat("account URL is missing")
	}

	for _, u := range body.Authorities {
		if u == nil {
			return nil, errors.BadRequest.WithFormat("authority URL is nil")
		}
	}

	err := checkCreateAdiAccount(st, body.Url)
	if err != nil {
		return nil, err
	}

	if body.Precision > 18 {
		return nil, fmt.Errorf("precision must be in range 0 to 18")
	}

	if body.SupplyLimit != nil && body.SupplyLimit.Sign() < 0 {
		return nil, fmt.Errorf("supply limit can't be a negative value")
	}

	token := new(protocol.TokenIssuer)
	token.Url = body.Url
	token.Precision = body.Precision
	token.SupplyLimit = body.SupplyLimit
	token.Symbol = body.Symbol
	token.Properties = body.Properties

	err = st.SetAuth(token, body.Authorities)
	if err != nil {
		return nil, err
	}

	err = st.Create(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", token.Url, err)
	}
	return nil, nil
}
