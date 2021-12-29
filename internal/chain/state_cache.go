package chain

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type stateCache struct {
	logger  logging.OptionalLogger
	nodeUrl *url.URL
	txType  types.TransactionType
	txHash  types.Bytes32

	batch      *database.Batch
	operations []stateOperation
	dataStores map[[32]byte]*storeDataEntry
	chains     map[[32]byte]state.Chain
	indices    map[[32]byte]*writeIndex
}

func newStateCache(nodeUrl *url.URL, txtype types.TransactionType, txid [32]byte, batch *database.Batch) *stateCache {
	c := new(stateCache)
	c.nodeUrl = nodeUrl
	c.txType = txtype
	c.txHash = txid

	c.batch = batch
	c.dataStores = map[[32]byte]*storeDataEntry{}
	c.chains = map[[32]byte]state.Chain{}
	c.indices = map[[32]byte]*writeIndex{}
	return c
}

func (c *stateCache) Commit() (DeliverMetadata, error) {
	var meta DeliverMetadata
	for _, op := range c.operations {
		err := op.Execute(c, &meta)
		if err != nil {
			return DeliverMetadata{}, err
		}
	}

	return meta, nil
}

func (c *stateCache) load(id [32]byte, r *database.Record) (state.Chain, error) {
	st, ok := c.chains[id]
	if ok {
		return st, nil
	}

	st, err := r.GetState()
	if err != nil {
		return nil, err
	}

	if c.chains == nil {
		c.chains = map[[32]byte]state.Chain{}
	}
	c.chains[id] = st
	return st, nil
}

func (c *stateCache) loadAs(id [32]byte, r *database.Record, v interface{}) (err error) {
	state, err := c.load(id, r)
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
func (c *stateCache) Load(id [32]byte) (state.Chain, error) {
	return c.load(id, c.batch.RecordByID(id[:]))
}

// LoadUrl loads a chain by URL and unmarshals it.
func (c *stateCache) LoadUrl(u *url.URL) (state.Chain, error) {
	return c.load(u.ResourceChain32(), c.batch.Record(u))
}

// LoadString loads a chain by URL and unmarshals it.
func (c *stateCache) LoadString(s string) (state.Chain, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	return c.LoadUrl(u)
}

// LoadAs loads a chain by ID and unmarshals it as a specific type.
func (c *stateCache) LoadAs(id [32]byte, v interface{}) error {
	return c.loadAs(id, c.batch.RecordByID(id[:]), v)
}

// LoadUrlAs loads a chain by URL and unmarshals it as a specific type.
func (c *stateCache) LoadUrlAs(u *url.URL, v interface{}) error {
	return c.loadAs(u.ResourceChain32(), c.batch.Record(u), v)
}

// LoadStringAs loads a chain by URL and unmarshals it as a specific type.
func (c *stateCache) LoadStringAs(s string, v interface{}) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	return c.LoadUrlAs(u, v)
}

//GetHeight loads the height of the chain
func (c *stateCache) GetHeight(u *url.URL) (uint64, error) {
	chain, err := c.batch.Record(u).Chain(protocol.MainChain)
	if err != nil {
		return 0, err
	}

	return uint64(chain.Height()), nil
}

// GetTxnState loads and unmarshals a saved synthetic transaction
func (c *stateCache) GetTxnState(txid [32]byte) (*state.Transaction, error) {
	return c.batch.Transaction(txid[:]).GetState()
}

func (c *stateCache) AddDirectoryEntry(u ...*url.URL) error {
	return AddDirectoryEntry(func(u *url.URL, key ...interface{}) Value {
		return c.RecordIndex(u, key...)
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
