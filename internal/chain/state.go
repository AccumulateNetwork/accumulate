package chain

import (
	"fmt"

	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type StateManager struct {
	stateCache

	Origin    protocol.Account
	OriginUrl *url.URL

	Signator    protocol.Signer
	SignatorUrl *url.URL
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
	records, err := m.stateCache.Commit()
	if err != nil {
		return nil, err
	}

	// Group synthetic create chain transactions per identity. All of the
	// records created by a given synthetic create chain MUST belong to the same
	// routing location, so grouping by ID is safe. Since routing locations will
	// change as the network grows, we cannot guarantee that two different
	// identities will route the same, so grouping by route is not safe.

	create := map[string]*protocol.SyntheticCreateChain{}
	for _, record := range records {
		data, err := record.MarshalBinary()
		if err != nil {
			return nil, err
		}

		params := protocol.ChainParams{Data: data, IsUpdate: false}
		id := record.GetUrl().RootIdentity()
		scc, ok := create[id.String()]
		if ok {
			scc.Chains = append(scc.Chains, params)
			continue
		}

		scc = new(protocol.SyntheticCreateChain)
		scc.Chains = []protocol.ChainParams{params}
		create[id.String()] = scc
		m.Submit(id, scc)
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

	swo, ok := body.(protocol.SynthTxnWithOrigin)
	if ok {
		swo.SetSyntheticOrigin(m.txHash[:], m.OriginUrl)
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
	var book *protocol.KeyBook
	err := m.LoadUrlAs(u, &book)
	if err != nil {
		return fmt.Errorf("invalid key book %q: %v", u, err)
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
