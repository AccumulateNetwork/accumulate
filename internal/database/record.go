package database

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

// Record manages a record.
type Record struct {
	batch *Batch
	key   recordBucket
}

// ensureObject ensures that the record's object metadata is up to date.
func (r *Record) ensureObject(addChains ...protocol.ChainMetadata) (*protocol.ObjectMetadata, error) {
	// Load the current metadata, if any
	meta := new(protocol.ObjectMetadata)
	err := r.batch.getAs(r.key.Object(), meta)
	switch {
	case err == nil:
		// Already exists
	case errors.Is(err, storage.ErrNotFound):
		meta.Type = protocol.ObjectTypeAccount
	default:
		return nil, err
	}

	if len(addChains) == 0 {
		return meta, nil
	}

	// Check for existing chains
	existing := map[string]int{}
	for i, chain := range meta.Chains {
		existing[chain.Name] = i
	}

	// Add new chains
	origLen := len(meta.Chains)
	for _, chain := range addChains {
		i, ok := existing[chain.Name]
		if !ok {
			existing[chain.Name] = len(meta.Chains)
			meta.Chains = append(meta.Chains, chain)
			continue
		}

		if meta.Chains[i] == chain {
			continue
		}

		if i <= origLen {
			return nil, fmt.Errorf("cannot alter metadata for chain %s", chain.Name)
		}
		return nil, fmt.Errorf("attempted to add chain %s multiple times with different types", chain.Name)
	}

	if len(meta.Chains) == origLen {
		return meta, nil
	}

	err = r.batch.putAs(r.key.Object(), meta)
	if err != nil {
		return nil, err
	}

	return meta, nil
}

// GetObject loads the object metadata.
func (r *Record) GetObject() (*protocol.ObjectMetadata, error) {
	meta := new(protocol.ObjectMetadata)
	err := r.batch.getAs(r.key.Object(), meta)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// GetState loads the record state.
func (r *Record) GetState() (state.Chain, error) {
	data, err := r.batch.store.Get(r.key.State())
	if err != nil {
		return nil, err
	}

	state, err := protocol.UnmarshalChain(data)
	if err != nil {
		return nil, err
	}

	return state, nil
}

// GetStateAs loads the record state and unmarshals into the given value.
func (r *Record) GetStateAs(state state.Chain) error {
	data, err := r.batch.store.Get(r.key.State())
	if err != nil {
		return err
	}

	err = state.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	return nil
}

// PutState stores the record state and adds the record to the BPT (as a hash).
func (r *Record) PutState(recordState state.Chain) error {
	// Does the record state have a URL?
	if recordState.Header().ChainUrl == "" {
		return errors.New("invalid URL: empty")
	}

	// Is the URL valid?
	u, err := recordState.Header().ParseUrl()
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	// Is this the right URL - does it match the record's key?
	if record(u) != r.key {
		return fmt.Errorf("invalid URL: %v", err)
	}

	// Make sure the key book is set
	switch recordState.(type) {
	case *protocol.LiteTokenAccount, *protocol.LiteDataAccount,
		*protocol.KeyBook, *protocol.KeyPage:
		// Empty key book is OK
	default:
		if recordState.Header().KeyBook == "" {
			return fmt.Errorf("missing key book")
		}
	}

	// Marshal the state
	stateData, err := recordState.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal state: %v", err)
	}

	// Store the state
	r.batch.store.Put(r.key.State(), stateData)
	return nil
}

// PutBpt writes the record's BPT entry.
func (r *Record) PutBpt(hash [32]byte) {
	r.batch.bpt.Bpt.Insert(r.key.Object(), hash)
}

func (r *Record) chain(name string, writable bool) (*Chain, error) {
	return newChain(r.batch.store, r.key.Chain(name), writable)
}

// Chain returns a chain manager for the given chain.
func (r *Record) Chain(name string, typ protocol.ChainType) (*Chain, error) {
	_, err := r.ensureObject(protocol.ChainMetadata{Name: name, Type: typ})
	if err != nil {
		return nil, err
	}
	return r.chain(name, true)
}

// ReadChain returns a read-only chain manager for the given chain.
func (r *Record) ReadChain(name string) (*Chain, error) {
	return r.chain(name, false)
}

// Index returns a value that can read or write an index value.
func (r *Record) Index(key ...interface{}) *Value {
	return &Value{r.batch, r.key.Index(key...)}
}

// Data returns a data chain manager for the data chain.
func (r *Record) Data() (*Data, error) {
	chain, err := r.Chain(protocol.DataChain, protocol.ChainTypeData)
	if err != nil {
		return nil, err
	}

	return &Data{r.batch, r.key, chain}, nil
}
