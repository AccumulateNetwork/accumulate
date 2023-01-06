// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type StateManager struct {
	stateCache

	Origin    protocol.Account
	OriginUrl *url.URL
}

func LoadStateManager(net *config.Describe, globals *core.GlobalValues, batch *database.Batch, principal protocol.Account, transaction *protocol.Transaction, status *protocol.TransactionStatus, logger log.Logger) (*StateManager, error) {
	if !transaction.Body.Type().IsUser() {
		// Don't check the signer
		return NewStateManager(net, globals, batch, principal, transaction, logger), nil
	}

	var signer protocol.Signer
	err := batch.Account(status.Initiator).GetStateAs(&signer)
	switch {
	case err == nil:
		// Found it
		return NewStateManager(net, globals, batch, principal, transaction, logger), nil

	case !errors.Is(err, storage.ErrNotFound):
		// Unknown error
		return nil, fmt.Errorf("load signer: %w", err)

	case transaction.Header.Principal.LocalTo(status.Initiator):
		// If the signer is local, it must exist
		return nil, fmt.Errorf("load signer: %w", err)
	}

	_, ok := status.GetSigner(status.Initiator)
	if !ok {
		// This should never happen
		return nil, fmt.Errorf("transaction signer set does not include the initiator")
	}

	return NewStateManager(net, globals, batch, principal, transaction, logger), nil
}

// NewStateManager creates a new state manager and loads the transaction's
// origin. If the origin is not found, NewStateManager returns a valid state
// manager along with a not-found error.
func NewStateManager(net *config.Describe, globals *core.GlobalValues, batch *database.Batch, principal protocol.Account, transaction *protocol.Transaction, logger log.Logger) *StateManager {
	txid := *(*[32]byte)(transaction.GetHash())
	m := new(StateManager)
	m.OriginUrl = transaction.Header.Principal
	m.Origin = principal
	m.stateCache = *newStateCache(net, globals, transaction.Body.Type(), txid, batch)
	m.logger.L = logger
	return m
}

func (m *StateManager) GetBatch() *database.Batch { return m.batch }
func (m *StateManager) GetHash() []byte           { return m.txHash[:] }

// commit writes pending records to the database.
func (m *StateManager) Commit() (*ProcessTransactionState, error) {
	_, err := m.stateCache.Commit()
	if err != nil {
		return nil, err
	}

	err = m.batch.Commit()
	if err != nil {
		return nil, err
	}

	return &m.State, nil
}

func (m *StateManager) Discard() {
	m.batch.Discard()
}

// Submit queues a synthetic transaction for submission.
func (m *StateManager) Submit(url *url.URL, body protocol.TransactionBody) {
	if m.txType.IsSynthetic() {
		panic("Called stateCache.Submit from a synthetic transaction!")
	}
	if url == nil {
		panic("No destination URL specified!")
	}

	m.State.DidProduceTxn(url, body)
}

func (m *StateManager) AddAuthority(account protocol.FullAccount, authority *url.URL) error {
	if account.GetUrl().LocalTo(authority) {
		var book *protocol.KeyBook
		err := m.LoadUrlAs(authority, &book)
		if err != nil {
			return errors.UnknownError.WithFormat("load %q: %w", authority, err)
		}
	}

	// TODO Check the proof if the authority is remote

	account.GetAuth().AddAuthority(authority)
	return nil
}

func (m *StateManager) InheritAuth(account protocol.FullAccount) error {
	if !account.GetUrl().RootIdentity().Equal(m.OriginUrl.RootIdentity()) {
		return errors.BadRequest.With("cannot inherit from principal: belongs to a different root identity")
	}

	principal, ok := m.Origin.(protocol.FullAccount)
	if !ok {
		return errors.BadRequest.With("cannot inherit from principal: not a full account")
	}

	// Inherit auth from the principal
	auth := account.GetAuth()
	*auth = *principal.GetAuth()
	return nil
}

func (m *StateManager) SetAuth(account protocol.FullAccount, authorities []*url.URL) error {
	switch {
	case len(authorities) > 0:
		// If the user specified a list of authorities, use them

	case len(account.GetAuth().Authorities) > 0:
		// If the account already has an authority, there's nothing to do
		return nil

	default:
		// Otherwise, inherit
		err := m.InheritAuth(account)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	for _, authority := range authorities {
		if authority == nil {
			return errors.BadRequest.WithFormat("authority URL is nil")
		}
		err := m.AddAuthority(account, authority)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	return nil
}
