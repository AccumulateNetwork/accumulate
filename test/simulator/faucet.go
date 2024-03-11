// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type simFaucet Simulator

func (s *simFaucet) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	err := (*Simulator)(s).DatabaseFor(account).Update(func(batch *database.Batch) error {
		var acct protocol.AccountWithTokens
		err := batch.Account(account).Main().GetAs(&acct)
		switch {
		case err == nil:
			if !acct.GetTokenUrl().Equal(protocol.AcmeUrl()) {
				return errors.BadRequest.WithFormat("%v is not an ACME account", account)
			}

			acct.CreditTokens(big.NewInt(10 * protocol.AcmePrecision))
			return nil

		case !errors.Is(err, errors.NotFound):
			return err
		}

		_, tok, _ := protocol.ParseLiteTokenAddress(account)
		if tok == nil {
			return err
		}
		if !tok.Equal(protocol.AcmeUrl()) {
			return errors.BadRequest.WithFormat("%v is not an ACME account", account)
		}

		lid := new(protocol.LiteIdentity)
		lid.Url = account.RootIdentity()
		err = batch.Account(lid.Url).Main().Put(lid)
		if err != nil {
			return err
		}

		lta := new(protocol.LiteTokenAccount)
		lta.Url = account
		lta.Balance = *big.NewInt(10 * protocol.AcmePrecision)
		lta.TokenUrl = protocol.AcmeUrl()
		return batch.Account(lta.Url).Main().Put(lta)
	})
	if err != nil {
		return nil, err
	}
	return &api.Submission{
		Status: &protocol.TransactionStatus{
			Code: errors.Delivered,
			TxID: account.WithTxID([32]byte{1}),
		},
		Success: true,
	}, nil
}
