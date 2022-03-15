package database

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Account manages a record.
type Account struct {
	batch *Batch
	key   accountBucket
}

// ensureObject ensures that the record's object metadata is up to date.
func (r *Account) ensureObject(addChains ...protocol.ChainMetadata) (*protocol.ObjectMetadata, error) {
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

		if meta.Chains[i].Equal(&chain) {
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
func (r *Account) GetObject() (*protocol.ObjectMetadata, error) {
	meta := new(protocol.ObjectMetadata)
	err := r.batch.getAs(r.key.Object(), meta)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// GetState loads the record state.
func (r *Account) GetState() (protocol.Account, error) {
	data, err := r.batch.store.Get(r.key.State())
	if err != nil {
		return nil, err
	}

	state, err := protocol.UnmarshalAccount(data)
	if err != nil {
		return nil, err
	}

	return state, nil
}

// GetStateAs loads the record state and unmarshals into the given value.
func (r *Account) GetStateAs(state protocol.Account) error {
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
func (r *Account) PutState(accountState protocol.Account) error {
	// Does the record state have a URL?
	if accountState.Header().Url == nil {
		return errors.New("invalid URL: empty")
	}

	// Is this the right URL - does it match the record's key?
	if account(accountState.Header().Url) != r.key {
		return fmt.Errorf("mismatched url: key is %X, URL is %v", r.key.objectBucket, accountState.Header().Url)
	}

	// Make sure the key book is set
	switch accountState.(type) {
	case *protocol.LiteTokenAccount, *protocol.LiteDataAccount,
		*protocol.KeyBook, *protocol.KeyPage:
		// Empty key book is OK
	default:
		if accountState.Header().KeyBook == nil {
			return fmt.Errorf("missing key book")
		}
	}

	// Marshal the state
	stateData, err := accountState.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal state: %v", err)
	}

	// Store the state
	return r.batch.store.Put(r.key.State(), stateData)
}

// PutBpt writes the record's BPT entry.
func (r *Account) PutBpt(hash [32]byte) {
	r.batch.bpt.Bpt.Insert(r.key.Object(), hash)
}

func (r *Account) chain(name string, writable bool) (*Chain, error) {
	return newChain(r.batch.store, r.key.Chain(name), writable)
}

// Chain returns a chain manager for the given chain.
func (r *Account) Chain(name string, typ protocol.ChainType) (*Chain, error) {
	_, err := r.ensureObject(protocol.ChainMetadata{Name: name, Type: typ})
	if err != nil {
		return nil, err
	}
	return r.chain(name, true)
}

// IndexChain returns a chain manager for the index chain of the given chain.
func (r *Account) IndexChain(name string, major bool) (*Chain, error) {
	return r.Chain(protocol.IndexChain(name, major), protocol.ChainTypeIndex)
}

// ReadChain returns a read-only chain manager for the given chain.
func (r *Account) ReadChain(name string) (*Chain, error) {
	return r.chain(name, false)
}

// ReadIndexChain returns a read-only chain manager for the index chain of the given chain.
func (r *Account) ReadIndexChain(name string, major bool) (*Chain, error) {
	return r.chain(protocol.IndexChain(name, major), false)
}

// Index returns a value that can read or write an index value.
func (r *Account) Index(key ...interface{}) *Value {
	return &Value{r.batch, r.key.Index(key...)}
}

// Data returns a data chain manager for the data chain.
func (r *Account) Data() (*Data, error) {
	chain, err := r.Chain(protocol.DataChain, protocol.ChainTypeData)
	if err != nil {
		return nil, err
	}

	return &Data{r.batch, r.key, chain}, nil
}
