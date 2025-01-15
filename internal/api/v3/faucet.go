// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Faucet implements [api.Faucet].
//
// Faucet aggregates faucet transactions and sends them once per block to ensure
// the correct ordering of signatures. Otherwise, the transactions could be
// reordered and fail due to timestamp/nonce errors. When a user submits a
// faucet request, the faucet appends the transaction and signature to the
// current batch, which is submitted to the network after receiving a block
// event.
type Faucet struct {
	logger    logging.OptionalLogger
	account   *url.URL
	token     *url.URL
	precision uint64
	amount    uint64
	issue     bool
	submitter api.Submitter

	signingKey      build.Signer
	signerUrl       *url.URL
	signerVersion   uint64
	signerTimestamp signing.Timestamp
}

// FaucetParams are the parameters for a [Faucet].
type FaucetParams struct {
	Logger    log.Logger
	Account   *url.URL
	Key       build.Signer
	Submitter api.Submitter
	Querier   api.Querier
	Events    api.EventService
	Amount    uint64
}

var _ api.Faucet = (*Faucet)(nil)

// NewFaucet creates a new Faucet with the given parameters.
//
// Callers must call [Faucet.Stop] or cancel the context when the faucet is no
// longer needed. Otherwise, NewFaucet will leak goroutines.
func NewFaucet(ctx context.Context, params FaucetParams) (*Faucet, error) {
	f := new(Faucet)
	f.logger.Set(params.Logger)
	f.account = params.Account
	f.signingKey = params.Key
	f.submitter = params.Submitter

	if params.Amount == 0 {
		f.amount = 10
	} else {
		f.amount = params.Amount
	}

	// Load the token type
	q := api.Querier2{Querier: params.Querier}
	r, err := q.QueryAccount(ctx, f.account, nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load account %v: %w", f.account, err)
	}
	switch account := r.Account.(type) {
	case *protocol.TokenIssuer:
		f.precision = account.Precision
		f.issue = true
		f.token = account.Url
	case protocol.AccountWithTokens:
		var issuer *protocol.TokenIssuer
		_, err = q.QueryAccountAs(ctx, account.GetTokenUrl(), nil, &issuer)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load issuer %v: %w", account.GetTokenUrl(), err)
		}
		f.precision = issuer.Precision
		f.token = account.GetTokenUrl()
	default:
		return nil, errors.UnknownError.WithFormat("cannot send tokens from %v (%v)", f.account, account.Type())
	}

	// Get the key hash
	pkh, ok := params.Key.Address().GetPublicKeyHash()
	if !ok {
		return nil, errors.BadRequest.With("key does not have a hash")
	}

	// Find the signer
	results, err := q.SearchForPublicKeyHash(ctx, params.Account, &api.PublicKeyHashSearchQuery{PublicKeyHash: pkh})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("find %x in %v: %w", pkh[:4], params.Account, err)
	}
	if results.Total == 0 {
		return nil, errors.NotFound.WithFormat("could not find %x in %v", pkh[:4], params.Account)
	}

	// Extract the signer parameters
	signer := results.Records[0]
	f.signerUrl = signer.Signer
	f.signerVersion = signer.Version
	f.signerTimestamp = (*signing.TimestampFromVariable)(&signer.Entry.LastUsedOn)

	return f, nil
}

// Type returns [api.ServiceTypeFaucet].
func (f *Faucet) Type() api.ServiceType { return api.ServiceTypeFaucet }

// ServiceAddress returns `/acc-svc/facuet:{token}`.
func (f *Faucet) ServiceAddress() *api.ServiceAddress { return f.Type().AddressForUrl(f.token) }

// Stop does nothing.
func (f *Faucet) Stop() {}

// Faucet implements [api.Faucet.Faucet].
func (f *Faucet) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	b := build.Transaction().For(f.account)
	if f.issue {
		b = b.IssueTokens(f.amount, f.precision).
			To(account).
			FinishTransaction()
	} else {
		b = b.SendTokens(f.amount, f.precision).
			To(account).
			FinishTransaction()
	}

	env, err := b.SignWith(f.signerUrl).
		Signer(f.signingKey).
		Version(f.signerVersion).
		Timestamp(f.signerTimestamp).
		Done()
	if err != nil {
		return nil, err
	}

	// Submit the envelope
	subs, err := f.submitter.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		f.logger.Error("Failed to submit", "err", err)
	}
	for _, sub := range subs {
		if sub.Status.Error == nil {
			f.logger.Info("Submitted", "submission", sub)
		} else {
			f.logger.Error("Submission failed", "err", sub.Status.Error, "submission", sub)
		}
	}

	if len(subs) == 0 {
		return nil, errors.InternalError.With("failed to submit")
	}
	return subs[0], nil
}
