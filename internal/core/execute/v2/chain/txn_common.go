// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func loadCreditAccount(batch *database.Batch, u *url.URL, errMsg string) (protocol.AccountWithCredits, error) {
	account, err := batch.Account(u).Main().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load account: %w", err)
	}

	switch account := account.(type) {
	case protocol.AccountWithCredits:
		return account, nil

	case *protocol.LiteTokenAccount:
		var credits protocol.AccountWithCredits
		err = batch.Account(account.Url.RootIdentity()).Main().GetAs(&credits)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load lite identity: %w", err)
		}
		return credits, nil

	default:
		return nil, errors.BadRequest.WithFormat("%s: want a signer, got %v", errMsg, account.Type())
	}
}
