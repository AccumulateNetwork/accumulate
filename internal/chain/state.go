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
	submissions []*SubmittedTransaction

	Origin        state.Chain
	OriginUrl     *url.URL
	OriginChainId [32]byte

	Signator    creditChain
	SignatorUrl *url.URL
}

type storeDataEntry struct {
	entryHash []byte
	dataEntry []byte
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

// commit writes pending records to the database.
func (m *StateManager) Commit() (DeliverMetadata, error) {
	meta, err := m.stateCache.Commit()
	if err != nil {
		return meta, err
	}

	// Group synthetic create chain transactions per identity. All of the
	// records created by a given synthetic create chain MUST belong to the same
	// routing location, so grouping by ID is safe. Since routing locations will
	// change as the network grows, we cannot guarentee that two different
	// identities will route the same, so grouping by route is not safe.

	create := map[string]*protocol.SyntheticCreateChain{}
	submitted := make([]*SubmittedTransaction, 0, len(m.submissions)+len(meta.Submitted))
	submitted = append(submitted, m.submissions...)
	for _, sub := range meta.Submitted {
		scc, ok := sub.Body.(*protocol.SyntheticCreateChain)
		if !ok {
			submitted = append(submitted, sub)
			continue
		}

		id := sub.Url.Identity()
		scc2, ok := create[id.String()]
		if ok {
			scc2.Chains = append(scc2.Chains, scc.Chains...)
		} else {
			create[id.String()] = scc
			submitted = append(submitted, &SubmittedTransaction{id, scc})
		}
	}

	meta.Submitted = submitted
	return meta, nil
}

// Submit queues a synthetic transaction for submission.
func (m *StateManager) Submit(url *url.URL, body protocol.TransactionPayload) {
	if m.txType.IsSynthetic() {
		panic("Called stateCache.Submit from a synthetic transaction!")
	}
	m.submissions = append(m.submissions, &SubmittedTransaction{url, body})
}
