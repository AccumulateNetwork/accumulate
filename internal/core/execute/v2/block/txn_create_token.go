// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateToken struct{ txnCreate }

var _ chain.SignerValidator = (*CreateToken)(nil)

func (CreateToken) Type() protocol.TransactionType { return protocol.TransactionTypeCreateToken }

func (x CreateToken) Process(batch *database.Batch, ctx *TransactionContext) (*protocol.TransactionStatus, error) {
	body, ok := ctx.transaction.Body.(*protocol.CreateToken)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateToken), ctx.transaction.Body)
	}

	if body.Url == nil {
		return nil, errors.BadRequest.WithFormat("account URL is missing")
	}

	for _, u := range body.Authorities {
		if u == nil {
			return nil, errors.BadRequest.WithFormat("authority URL is nil")
		}
	}

	err := x.checkCreateAdiAccount(batch, ctx, body.Url)
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

	err = x.setAuth(batch, ctx, token)
	if err != nil {
		return nil, err
	}

	err = ctx.storeAccount(batch, mustNotExist, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", token.Url, err)
	}
	return nil, nil
}
