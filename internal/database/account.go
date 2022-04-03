package database

import (
	"bytes"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Account manages a record.
type Account struct {
	batch *Batch
	key   accountBucket
}

// ensureChain ensures that the account's metadata includes the given chain.
func (r *Account) ensureChain(newChain protocol.ChainMetadata) error {
	// Load the current metadata, if any
	meta, err := r.GetObject()
	switch {
	case err == nil:
		// Already exists
	case errors.Is(err, storage.ErrNotFound):
		meta.Type = protocol.ObjectTypeAccount
	default:
		return err
	}

	err = meta.AddChain(newChain.Name, newChain.Type)
	if err != nil {
		return err
	}

	r.batch.putValue(r.key.Object(), meta)
	return nil
}

// GetObject loads the object metadata.
func (r *Account) GetObject() (*protocol.Object, error) {
	meta := new(protocol.Object)
	err := r.batch.getValuePtr(r.key.Object(), meta, &meta, true)
	return meta, err
}

// GetState loads the record state.
func (r *Account) GetState() (protocol.Account, error) {
	return r.batch.getAccountState(r.key.State(), nil)
}

// GetStateAs loads the record state and unmarshals into the given value. In
// most cases `state` should be a double pointer.
func (r *Account) GetStateAs(state interface{}) error {
	return r.batch.getAccountStateAs(r.key.State(), nil, state)
}

// PutState stores the record state and adds the record to the BPT (as a hash).
func (r *Account) PutState(state protocol.Account) error {
	// Does the record state have a URL?
	if state.GetUrl() == nil {
		return errors.New("invalid URL: empty")
	}

	// Is this the right URL - does it match the record's key?
	if account(state.GetUrl()) != r.key {
		return fmt.Errorf("mismatched url: key is %X, URL is %v", r.key.objectBucket, state.GetUrl())
	}

	// Make sure the key book is set
	account, ok := state.(protocol.FullAccount)
	if ok && len(account.GetAuth().Authorities) == 0 {
		return fmt.Errorf("missing key book")
	}

	// Store the state
	r.batch.putValue(r.key.State(), state)
	return nil
}

func (r *Account) chain(name string, writable bool) (*Chain, error) {
	return newChain(r.batch.store, r.key.Chain(name), writable)
}

// Chain returns a chain manager for the given chain.
func (r *Account) Chain(name string, typ protocol.ChainType) (*Chain, error) {
	err := r.ensureChain(protocol.ChainMetadata{Name: name, Type: typ})
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

// stateHashes returns a hasher populated with hashes of all of the account's
// states.
func (r *Account) stateHashes() (hash.Hasher, error) {
	obj, err := r.GetObject()
	if err != nil {
		return nil, fmt.Errorf("load object metadata: %w", err)
	}

	state, err := r.GetState()
	if err != nil {
		return nil, fmt.Errorf("load account state: %w", err)
	}

	data, err := state.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal account state: %w", err)
	}

	hasher := make(hash.Hasher, 0, len(obj.Chains)+1)
	hasher.AddBytes(data)

	for _, chainMeta := range obj.Chains {
		chain, err := r.ReadChain(chainMeta.Name)
		if err != nil {
			return nil, fmt.Errorf("load account chain: %w", err)
		}

		hasher.AddHash((*[32]byte)(chain.Anchor()))
	}

	return hasher, nil
}

// PutBpt writes the record's BPT entry.
func (r *Account) PutBpt() error {
	hasher, err := r.stateHashes()
	if err != nil {
		return err
	}

	hash := *(*[32]byte)(hasher.MerkleHash())
	r.batch.putBpt(r.key.Object(), hash)
	return nil
}

// StateReceipt returns a Merkle receipt for the account state in the BPT.
func (r *Account) StateReceipt() (*managed.Receipt, error) {
	hasher, err := r.stateHashes()
	if err != nil {
		return nil, err
	}

	rBPT, err := r.batch.BptReceipt(r.key.Object(), *(*[32]byte)(hasher.MerkleHash()))
	if err != nil {
		return nil, err
	}

	rState := hasher.Receipt(0, len(hasher)-1)
	if !bytes.Equal(rState.MDRoot, rBPT.Element) {
		return nil, errors.New("bpt entry does not match account state")
	}

	receipt, err := rState.Combine(rBPT)
	if err != nil {
		return nil, fmt.Errorf("combine receipt: %w", err)
	}

	return receipt, nil
}
