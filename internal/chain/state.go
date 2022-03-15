package chain

import (
	"errors"
	"fmt"

	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type StateManager struct {
	stateCache

	Origin        protocol.Account
	OriginUrl     *url.URL
	OriginChainId [32]byte

	Signator    creditChain
	SignatorUrl *url.URL
}

// NewStateManager creates a new state manager and loads the transaction's
// origin. If the origin is not found, NewStateManager returns a valid state
// manager along with a not-found error.
func NewStateManager(batch *database.Batch, nodeUrl *url.URL, env *protocol.Envelope) (*StateManager, error) {
	m := new(StateManager)
	txid := types.Bytes(env.GetTxHash()).AsBytes32()
	m.stateCache = *newStateCache(nodeUrl, env.Transaction.Type(), txid, batch)
	m.OriginUrl = env.Transaction.Origin

	copy(m.OriginChainId[:], m.OriginUrl.AccountID())

	// Find the origin
	var err error
	m.Origin, err = m.LoadUrl(m.OriginUrl)
	switch {
	case err == nil:
		// Found the origin
		return m, nil

	case errors.Is(err, storage.ErrNotFound):
		// Origin is missing
		m.Origin = nil
		return m, nil
		// return m, fmt.Errorf("invalid origin record: %q %w", m.OriginUrl, storage.ErrNotFound)

	default:
		// Unknown error
		return nil, err
	}
}

func (m *StateManager) Reset() {
	m.stateCache.Reset()
	m.blockState = BlockState{}
}

// commit writes pending records to the database.
func (m *StateManager) Commit() error {
	records, err := m.stateCache.Commit()
	if err != nil {
		return err
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
			return err
		}

		params := protocol.ChainParams{Data: data, IsUpdate: false}
		id := record.Header().Url.RootIdentity()
		scc, ok := create[id.String()]
		if ok {
			scc.Chains = append(scc.Chains, params)
			continue
		}

		scc = new(protocol.SyntheticCreateChain)
		scc.Cause = m.txHash
		scc.Chains = []protocol.ChainParams{params}
		create[id.String()] = scc
		m.Submit(id, scc)
	}

	return nil
}

// Submit queues a synthetic transaction for submission.
func (m *StateManager) Submit(url *url.URL, body protocol.TransactionBody) {
	if m.txType.IsSynthetic() {
		panic("Called stateCache.Submit from a synthetic transaction!")
	}

	txn := new(protocol.Transaction)
	txn.Origin = url
	txn.Body = body
	m.blockState.DidProduceTxn(txn)
}

func (m *StateManager) AddValidator(pubKey ed25519.PubKey) {
	m.blockState.ValidatorsUpdates = append(m.blockState.ValidatorsUpdates, abci.ValidatorUpdate{
		PubKey:  pubKey,
		Enabled: true,
	})
}

func (m *StateManager) DisableValidator(pubKey ed25519.PubKey) {
	// You can't really remove validators as far as I can see, but you can set the voting power to 0
	m.blockState.ValidatorsUpdates = append(m.blockState.ValidatorsUpdates, abci.ValidatorUpdate{
		PubKey:  pubKey,
		Enabled: false,
	})
}

func (m *StateManager) setKeyBook(account protocol.Account, u *url.URL) error {
	if u == nil {
		account.Header().KeyBook = m.Origin.Header().KeyBook
		return nil
	}

	book := new(protocol.KeyBook)
	err := m.LoadUrlAs(u, book)
	if err != nil {
		return fmt.Errorf("invalid key book %q: %v", u, err)
	}

	account.Header().KeyBook = u
	return nil
}
