package chain

import (
	"errors"
	"fmt"

	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type StateManager struct {
	stateCache

	Origin    protocol.Account
	OriginUrl *url.URL

	Signator    protocol.Signer
	SignatorUrl *url.URL
}

func LoadStateManager(batch *database.Batch, nodeUrl *url.URL, principal protocol.Account, transaction *protocol.Transaction, status *protocol.TransactionStatus, logger log.Logger) (*StateManager, error) {
	var signer protocol.Signer
	err := batch.Account(status.Initiator).GetStateAs(&signer)
	switch {
	case err == nil:
		// Found it
		return NewStateManager(batch, nodeUrl, status.Initiator, signer, principal, transaction, logger), nil

	case !errors.Is(err, storage.ErrNotFound):
		// Unknown error
		return nil, fmt.Errorf("load signer: %w", err)

	case transaction.Header.Principal.LocalTo(status.Initiator):
		// If the signer is local, it must exist
		return nil, fmt.Errorf("load signer: %w", err)
	}

	signer, ok := status.GetSigner(status.Initiator)
	if !ok {
		// This should never happen
		return nil, fmt.Errorf("transaction signer set does not include the initiator")
	}

	return NewStateManager(batch, nodeUrl, status.Initiator, signer, principal, transaction, logger), nil
}

// NewStateManager creates a new state manager and loads the transaction's
// origin. If the origin is not found, NewStateManager returns a valid state
// manager along with a not-found error.
func NewStateManager(batch *database.Batch, nodeUrl, signerUrl *url.URL, signer protocol.Signer, principal protocol.Account, transaction *protocol.Transaction, logger log.Logger) *StateManager {
	txid := types.Bytes(transaction.GetHash()).AsBytes32()
	m := new(StateManager)
	m.SignatorUrl = signerUrl
	m.Signator = signer
	m.OriginUrl = transaction.Header.Principal
	m.Origin = principal
	m.stateCache = *newStateCache(nodeUrl, transaction.Body.Type(), txid, batch)
	m.logger.L = logger
	return m
}

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

	return &m.state, nil
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

	m.state.DidProduceTxn(url, body)
}

func (m *StateManager) AddValidator(pubKey ed25519.PubKey) {
	m.state.ValidatorsUpdates = append(m.state.ValidatorsUpdates, ValidatorUpdate{
		PubKey:  pubKey,
		Enabled: true,
	})
}

func (m *StateManager) DisableValidator(pubKey ed25519.PubKey) {
	// You can't really remove validators as far as I can see, but you can set the voting power to 0
	m.state.ValidatorsUpdates = append(m.state.ValidatorsUpdates, ValidatorUpdate{
		PubKey:  pubKey,
		Enabled: false,
	})
}

func (m *StateManager) AddAuthority(account protocol.FullAccount, u *url.URL) error {
	if m.OriginUrl.LocalTo(u) {
		var book *protocol.KeyBook
		err := m.LoadUrlAs(u, &book)
		if err != nil {
			return fmt.Errorf("invalid key book %q: %v", u, err)
		}
	}

	account.GetAuth().AddAuthority(u)
	return nil
}

func (m *StateManager) InheritAuth(account protocol.FullAccount) error {
	if !account.GetUrl().RootIdentity().Equal(m.OriginUrl.RootIdentity()) {
		return fmt.Errorf("cannot inherit from principal: belongs to a different root identity")
	}

	principal, ok := m.Origin.(protocol.FullAccount)
	if !ok {
		return fmt.Errorf("cannot inherit from principal: not a full account")
	}

	// Inherit auth from the principal
	auth := account.GetAuth()
	*auth = *principal.GetAuth()
	return nil
}

func (m *StateManager) SetAuth(account protocol.FullAccount, mainKeyBook, managerKeyBook *url.URL) error {
	var err error
	if mainKeyBook == nil {
		err = m.InheritAuth(account)
	} else {
		err = m.AddAuthority(account, mainKeyBook)
	}
	if err != nil {
		return err
	}

	if managerKeyBook != nil {
		err = m.AddAuthority(account, managerKeyBook)
		if err != nil {
			return err
		}
	}

	return nil
}
