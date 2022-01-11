package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type StateManager struct {
	stateCache
	submissions []*submission

	Origin        state.Chain
	OriginUrl     *url.URL
	OriginChainId [32]byte

	Signator    creditChain
	SignatorUrl *url.URL
}

type submission struct {
	Url  *url.URL
	Body protocol.TransactionPayload
}

// NewStateManager creates a new state manager and loads the transaction's
// origin. If the origin is not found, NewStateManager returns a valid state
// manager along with a not-found error.
func NewStateManager(batch *database.Batch, nodeUrl *url.URL, tx *transactions.Envelope) (*StateManager, error) {
	m := new(StateManager)
	txid := types.Bytes(tx.GetTxHash()).AsBytes32()
	m.stateCache = *newStateCache(nodeUrl, tx.Transaction.Type(), txid, batch)
	m.OriginUrl = tx.Transaction.Origin

	copy(m.OriginChainId[:], m.OriginUrl.ResourceChain())

	// Find the origin
	var err error
	m.Origin, err = m.LoadUrl(m.OriginUrl)
	if err == nil {
		return m, nil
	}

	// If the origin doesn't exist, that might be OK
	if errors.Is(err, storage.ErrNotFound) {
		return m, fmt.Errorf("invalid origin record: %q %w", m.OriginUrl, storage.ErrNotFound)
	}
	return nil, err
}

func (m *StateManager) Reset() {
	m.stateCache.Reset()
	m.submissions = m.submissions[:0]
}

// commit writes pending records to the database.
func (m *StateManager) Commit() ([]*submission, error) {
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
	submitted := make([]*submission, 0, len(m.submissions)+len(records))
	submitted = append(submitted, m.submissions...)
	for _, record := range records {
		u, err := record.Header().ParseUrl()
		if err != nil {
			return nil, err
		}

		data, err := record.MarshalBinary()
		if err != nil {
			return nil, err
		}

		params := protocol.ChainParams{Data: data, IsUpdate: false}
		id := u.Identity()
		scc, ok := create[id.String()]
		if ok {
			scc.Chains = append(scc.Chains, params)
			continue
		}

		scc = new(protocol.SyntheticCreateChain)
		scc.Cause = m.txHash
		scc.Chains = []protocol.ChainParams{params}
		create[id.String()] = scc
		submitted = append(submitted, &submission{id, scc})
	}

	return submitted, nil
}

// Submit queues a synthetic transaction for submission.
func (m *StateManager) Submit(url *url.URL, body protocol.TransactionPayload) {
	if m.txType.IsSynthetic() {
		panic("Called stateCache.Submit from a synthetic transaction!")
	}
	m.submissions = append(m.submissions, &submission{url, body})
}
