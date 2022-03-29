package chain

import (
	"fmt"

	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type StateManager struct {
	stateCache

	Origin    protocol.Account
	OriginUrl *url.URL

	Signator    creditChain
	SignatorUrl *url.URL
}

type SynthTxnWithOrigin interface {
	SetSyntheticOrigin(txid []byte, source *url.URL)
}

// NewStateManager creates a new state manager and loads the transaction's
// origin. If the origin is not found, NewStateManager returns a valid state
// manager along with a not-found error.
func NewStateManager(makeBatch func() *database.Batch, nodeUrl, signerUrl *url.URL, signer protocol.SignerAccount, principal protocol.Account, transaction *protocol.Transaction) *StateManager {
	txid := types.Bytes(transaction.GetHash()).AsBytes32()
	m := new(StateManager)
	m.SignatorUrl = signerUrl
	m.Signator = signer
	m.OriginUrl = transaction.Header.Principal
	m.Origin = principal
	m.stateCache = *newStateCache(nodeUrl, transaction.Body.Type(), txid, makeBatch())
	return m
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
		scc.Chains = []protocol.ChainParams{params}
		create[id.String()] = scc
		m.Submit(id, scc)
	}

	return m.batch.Commit()
}

func (m *StateManager) Discard() {
	m.batch.Discard()
}

// Submit queues a synthetic transaction for submission.
func (m *StateManager) Submit(url *url.URL, body protocol.TransactionBody) {
	if m.txType.IsSynthetic() && body.GetType() != protocol.TransactionTypeSyntheticReceipt {
		panic("Called stateCache.Submit from a synthetic transaction!")
	}
	if url == nil {
		panic("No destination URL specified!")
	}

	swo, ok := body.(SynthTxnWithOrigin)
	if ok {
		swo.SetSyntheticOrigin(m.txHash[:], m.OriginUrl)
	}

	m.blockState.DidProduceTxn(url, body)
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

	var book *protocol.KeyBook
	err := m.LoadUrlAs(u, &book)
	if err != nil {
		return fmt.Errorf("invalid key book %q: %v", u, err)
	}

	account.Header().KeyBook = u
	return nil
}
