package chain

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/smt/storage"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type StateManager struct {
	db          *state.StateDB
	chains      map[[32]byte]*stateEntry
	submissions []*submittedTx

	Sponsor        state.Chain
	SponsorUrl     *url.URL
	SponsorChainId [32]byte
}

func NewStateManager(db *state.StateDB, tx *transactions.GenTransaction) (*StateManager, error) {
	st := new(StateManager)
	st.db = db

	var err error
	st.SponsorUrl, err = url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, err
	}

	copy(st.SponsorChainId[:], st.SponsorUrl.ResourceChain())
	st.Sponsor, err = st.Load(st.SponsorChainId)
	if err == nil {
		return st, nil
	}

	if errors.Is(err, storage.ErrNotFound) {
		return st, fmt.Errorf("sponsor %q %w", st.SponsorUrl, err)
	}
	return nil, err
}

type stateEntry struct {
	record state.Chain
	dirty  bool
}

type submittedTx struct {
	url  *url.URL
	body encoding.BinaryMarshaler
}

func (m *StateManager) LoadString(s string) (state.Chain, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	return m.LoadUrl(u)
}

func (m *StateManager) LoadUrl(u *url.URL) (state.Chain, error) {
	var chainId [32]byte
	copy(chainId[:], u.ResourceChain())
	return m.Load(chainId)
}

func (m *StateManager) LoadStringAs(s string, v interface{}) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	return m.LoadUrlAs(u, v)
}

func (m *StateManager) LoadUrlAs(u *url.URL, v interface{}) error {
	var chainId [32]byte
	copy(chainId[:], u.ResourceChain())
	return m.LoadAs(chainId, v)
}

// Load loads the given chain and unmarshals it
func (m *StateManager) Load(chainId [32]byte) (state.Chain, error) {
	entry, ok := m.chains[chainId]
	if ok {
		return entry.record, nil
	}

	obj, err := m.db.GetCurrentEntry(chainId[:])
	if err != nil {
		return nil, err
	}

	record, err := unmarshalRecord(obj)
	if err != nil {
		return nil, err
	}

	entry = new(stateEntry)
	entry.record = record

	if m.chains == nil {
		m.chains = map[[32]byte]*stateEntry{}
	}
	m.chains[chainId] = entry
	return record, nil
}

func (m *StateManager) LoadAs(chainId [32]byte, v interface{}) (err error) {
	record, err := m.Load(chainId)
	if err != nil {
		return err
	}

	rv := reflect.ValueOf(v)
	rr := reflect.ValueOf(record)
	if !rr.Type().AssignableTo(rv.Type()) {
		return fmt.Errorf("want %T, got %T", v, record)
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

// Store queues a record for storage in the database
func (m *StateManager) Store(record state.Chain) {
	u, err := record.Header().ParseUrl()
	if err != nil {
		// The caller must ensure the chain URL is correct
		panic(fmt.Errorf("attempted to add an invalid chain: %v", err))
	}

	var chainId [32]byte
	copy(chainId[:], u.ResourceChain())
	entry, ok := m.chains[chainId]
	if !ok {
		if m.chains == nil {
			m.chains = map[[32]byte]*stateEntry{}
		}
		entry = new(stateEntry)
		m.chains[chainId] = entry
	}

	entry.dirty = true
	entry.record = record
}

// Submit queues a synthetic transaction for submission
func (m *StateManager) Submit(url *url.URL, body encoding.BinaryMarshaler) {
	m.submissions = append(m.submissions, &submittedTx{url, body})
}

func unmarshalRecord(obj *state.Object) (state.Chain, error) {
	header := new(state.ChainHeader)
	err := obj.As(header)
	if err != nil {
		return nil, err
	}

	var record state.Chain
	switch header.Type {
	// TODO DC, BVC, Token
	case types.ChainTypeAdi:
		record = new(state.AdiState)
	case types.ChainTypeTokenAccount:
		record = new(state.TokenAccount)
	case types.ChainTypeAnonTokenAccount:
		record = new(protocol.AnonTokenAccount)
	case types.ChainTypeTransactionReference:
		record = new(state.TxReference)
	case types.ChainTypeTransaction:
		record = new(state.Transaction)
	case types.ChainTypePendingTransaction:
		record = new(state.PendingTransaction)
	case types.ChainTypeSigSpec:
		record = new(protocol.SigSpec)
	case types.ChainTypeSigSpecGroup:
		record = new(protocol.SigSpecGroup)
	default:
		return nil, fmt.Errorf("unrecognized chain type %v", header.Type)
	}

	err = obj.As(record)
	if err != nil {
		return nil, err
	}

	return record, nil
}
