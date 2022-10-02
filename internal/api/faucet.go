// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Faucet struct {
	SubmitModule
}

var _ FaucetModule = Faucet{}

func (f Faucet) Faucet(ctx context.Context, account *url.URL, opts SubmitOptions) (*Submission, error) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.FaucetUrl
	txn.Body = &protocol.AcmeFaucet{Url: account}
	env := new(protocol.Envelope)
	env.Transaction = []*protocol.Transaction{txn}
	sig, err := new(signing.Builder).
		UseFaucet().
		UseSimpleHash().
		Initiate(txn)
	if err != nil {
		return nil, errors.Wrap(errors.StatusInternalError, err)
	}
	env.Signatures = append(env.Signatures, sig)

	return f.Submit(ctx, env, opts)
}
