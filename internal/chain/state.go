package chain

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/tendermint/tendermint/libs/log"
)

type StateManager struct {
	nodeUrl     *url.URL
	batch       *database.Batch
	operations  []stateOperation
	dataStores  map[[32]byte]*storeDataEntry
	chains      map[[32]byte]state.Chain
	indices     map[[32]byte]*writeIndex
	submissions []*SubmittedTransaction
	txHash      types.Bytes32
	txType      types.TransactionType
	logger      log.Logger

	Origin        state.Chain
	OriginUrl     *url.URL
	OriginChainId [32]byte
}

type storeDataEntry struct {
	entryHash []byte
	dataEntry []byte
}

// NewStateManager creates a new state manager and loads the transaction's
// origin. If the origin is not found, NewStateManager returns a valid state
// manager along with a not-found error.
func NewStateManager(batch *database.Batch, nodeUrl *url.URL, tx *transactions.GenTransaction) (*StateManager, error) {
	m := new(StateManager)
	m.batch = batch
	m.nodeUrl = nodeUrl
	m.chains = map[[32]byte]state.Chain{}
	m.dataStores = map[[32]byte]*storeDataEntry{}
	m.indices = map[[32]byte]*writeIndex{}
	m.txHash = types.Bytes(tx.TransactionHash()).AsBytes32()
	m.txType = tx.TransactionType()

	// The origin URL must be valid
	var err error
	m.OriginUrl, err = url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, err
	}

	copy(m.OriginChainId[:], m.OriginUrl.ResourceChain())

	// Find the origin
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

type SubmittedTransaction struct {
	url  *url.URL
	body protocol.TransactionPayload
}

func (m *StateManager) logDebug(msg string, keyVals ...interface{}) {
	if m.logger != nil {
		m.logger.Debug(msg, keyVals...)
	}
}

func (m *StateManager) load(id [32]byte, r *database.Record) (state.Chain, error) {
	st, ok := m.chains[id]
	if ok {
		return st, nil
	}

	st, err := r.GetState()
	if err != nil {
		return nil, err
	}

	if m.chains == nil {
		m.chains = map[[32]byte]state.Chain{}
	}
	m.chains[id] = st
	return st, nil
}

func (m *StateManager) loadAs(id [32]byte, r *database.Record, v interface{}) (err error) {
	state, err := m.load(id, r)
	if err != nil {
		return err
	}

	rv := reflect.ValueOf(v)
	rr := reflect.ValueOf(state)
	if !rr.Type().AssignableTo(rv.Type()) {
		return fmt.Errorf("want %T, got %T", v, state)
	}

	// Catch reflection panic
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failed to load chain: unable to write to %T", v)
		}
	}()
	rv.Elem().Set(rr.Elem())
	return nil
}

// Load loads a chain by ID and unmarshals it.
func (m *StateManager) Load(id [32]byte) (state.Chain, error) {
	return m.load(id, m.batch.RecordByID(id[:]))
}

// LoadUrl loads a chain by URL and unmarshals it.
func (m *StateManager) LoadUrl(u *url.URL) (state.Chain, error) {
	return m.load(u.ResourceChain32(), m.batch.Record(u))
}

// LoadString loads a chain by URL and unmarshals it.
func (m *StateManager) LoadString(s string) (state.Chain, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	return m.LoadUrl(u)
}

// LoadAs loads a chain by ID and unmarshals it as a specific type.
func (m *StateManager) LoadAs(id [32]byte, v interface{}) error {
	return m.loadAs(id, m.batch.RecordByID(id[:]), v)
}

// LoadUrlAs loads a chain by URL and unmarshals it as a specific type.
func (m *StateManager) LoadUrlAs(u *url.URL, v interface{}) error {
	return m.loadAs(u.ResourceChain32(), m.batch.Record(u), v)
}

// LoadStringAs loads a chain by URL and unmarshals it as a specific type.
func (m *StateManager) LoadStringAs(s string, v interface{}) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	return m.LoadUrlAs(u, v)
}

//GetHeight loads the height of the chain
func (m *StateManager) GetHeight(u *url.URL) (uint64, error) {
	chain, err := m.batch.Record(u).Chain(protocol.Main)
	if err != nil {
		return 0, err
	}

	return uint64(chain.Height()), nil
}

// GetTxnState loads and unmarshals a saved synthetic transaction
func (m *StateManager) GetTxnState(txid [32]byte) (*state.Transaction, error) {
	return m.batch.Transaction(txid[:]).GetState()
}

// Submit queues a synthetic transaction for submission.
func (m *StateManager) Submit(url *url.URL, body protocol.TransactionPayload) {
	if m.txType.IsSynthetic() {
		panic("Called StateManager.Submit from a synthetic transaction!")
	}
	m.submissions = append(m.submissions, &SubmittedTransaction{url, body})
}

type DeliverMetadata struct {
	Updated   []*url.URL
	Submitted []*SubmittedTransaction
}

func (d *DeliverMetadata) Empty() bool {
	return len(d.Updated) == 0 && len(d.Submitted) == 0
}

func (d *DeliverMetadata) Append(e DeliverMetadata) {
	d.Updated = append(d.Updated, e.Updated...)
	d.Submitted = append(d.Submitted, e.Submitted...)
}

// commit writes pending records to the database.
func (m *StateManager) Commit() (DeliverMetadata, error) {
	var meta DeliverMetadata
	for _, op := range m.operations {
		err := op.Execute(m, &meta)
		if err != nil {
			return DeliverMetadata{}, err
		}
	}

	// Group synthetic create chain transactions per identity. All of the
	// records created by a given synthetic create chain MUST belong to the same
	// routing location, so grouping by ID is safe. Since routing locations will
	// change as the network grows, we cannot guarentee that two different
	// identities will route the same, so grouping by route is not safe.

	create := map[string]*protocol.SyntheticCreateChain{}
	meta.Submitted = make([]*SubmittedTransaction, 0, len(m.submissions))
	for _, sub := range m.submissions {
		scc, ok := sub.body.(*protocol.SyntheticCreateChain)
		if !ok {
			meta.Submitted = append(meta.Submitted, sub)
			continue
		}

		id := sub.url.Identity()
		scc2, ok := create[id.String()]
		if ok {
			scc2.Chains = append(scc2.Chains, scc.Chains...)
		} else {
			create[id.String()] = scc
			meta.Submitted = append(meta.Submitted, &SubmittedTransaction{id, scc})
		}
	}

	return meta, nil
}

func (m *StateManager) AddDirectoryEntry(u ...*url.URL) error {
	return AddDirectoryEntry(func(u *url.URL, key ...interface{}) Value {
		return m.RecordIndex(u, key...)
	}, u...)
}

type Value interface {
	Get() ([]byte, error)
	Put([]byte)
}

func AddDirectoryEntry(getIndex func(*url.URL, ...interface{}) Value, u ...*url.URL) error {
	identity := u[0].Identity()
	mdi := getIndex(identity, "Directory", "Metadata")
	md := new(protocol.DirectoryIndexMetadata)
	data, err := mdi.Get()
	if err == nil {
		err = md.UnmarshalBinary(data)
	}
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("failed to load metadata: %v", err)
	}

	for _, u := range u {
		getIndex(identity, "Directory", md.Count).Put([]byte(u.String()))
		md.Count++
	}

	data, err = md.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}

	mdi.Put(data)
	return nil
}
