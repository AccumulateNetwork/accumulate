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

type CreateDataAccount struct{ txnCreate }

var _ chain.SignerValidator = (*CreateDataAccount)(nil)

func (CreateDataAccount) Type() protocol.TransactionType {
	return protocol.TransactionTypeCreateDataAccount
}

func (x CreateDataAccount) Process(batch *database.Batch, ctx *TransactionContext) (*protocol.TransactionStatus, error) {
	body, ok := ctx.transaction.Body.(*protocol.CreateDataAccount)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateDataAccount), ctx.transaction.Body)
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

	//create the data account
	account := new(protocol.DataAccount)
	account.Url = body.Url

	err = x.setAuth(batch, ctx, account)
	if err != nil {
		return nil, err
	}

	err = ctx.storeAccount(batch, mustNotExist, account)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", account.Url, err)
	}
	return nil, nil
}
