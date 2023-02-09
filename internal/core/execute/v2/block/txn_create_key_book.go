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

type CreateKeyBook struct{ txnCreate }

var _ chain.SignerValidator = (*CreateKeyBook)(nil)

func (CreateKeyBook) Type() protocol.TransactionType { return protocol.TransactionTypeCreateKeyBook }

func (x CreateKeyBook) Process(batch *database.Batch, ctx *TransactionContext) (*protocol.TransactionStatus, error) {
	body, ok := ctx.transaction.Body.(*protocol.CreateKeyBook)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyBook), ctx.transaction.Body)
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

	switch len(body.PublicKeyHash) {
	case 0:
		return nil, errors.BadRequest.WithFormat("public key hash is missing")
	case 32:
		// Ok
	default:
		return nil, errors.BadRequest.WithFormat("public key hash length is invalid")
	}

	book := new(protocol.KeyBook)
	book.Url = body.Url
	book.AddAuthority(body.Url)
	book.PageCount = 1

	err = x.setAuth(batch, ctx, book)
	if err != nil {
		return nil, err
	}

	page := new(protocol.KeyPage)
	page.Version = 1
	page.Url = protocol.FormatKeyPageUrl(body.Url, 0)

	key := new(protocol.KeySpec)
	key.PublicKeyHash = body.PublicKeyHash
	page.Keys = []*protocol.KeySpec{key}

	err = ctx.storeAccount(batch, mustNotExist, book, page)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", book.Url, err)
	}
	return nil, nil
}
