package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Account manages a record.
type Account struct {
	batch *Batch
	key   accountBucket
	url   *url.URL
}

// ensureMetadata ensures that the account's metadata is up to date.
func (r *Account) ensureMetadata(cb func(obj *protocol.Object) error) error {
	// Load the current metadata, if any
	meta, err := r.GetObject()
	var found bool
	switch {
	case err == nil:
		found = true
	case errors.Is(err, storage.ErrNotFound):
		meta.Type = protocol.ObjectTypeAccount
	default:
		return err
	}

	if cb != nil {
		err = cb(meta)
		if err != nil {
			return err
		}
	} else if found {
		// Already exists, nothing to do
		return nil
	}

	r.batch.putValue(r.key.Object(), meta)
	return nil
}

// ensureChain ensures that the account's metadata includes the given chain.
func (r *Account) ensureChain(newChain protocol.ChainMetadata) error {
	return r.ensureMetadata(func(obj *protocol.Object) error {
		return obj.AddChain(newChain.Name, newChain.Type)
	})
}

// GetObject loads the object metadata.
func (r *Account) GetObject() (*protocol.Object, error) {
	meta := new(protocol.Object)
	err := r.batch.getValuePtr(r.key.Object(), meta, &meta, true)
	if err != nil {
		err = errors.Wrap(errors.StatusUnknownError, err)
	}
	return meta, err
}

// GetState loads the record state.
func (r *Account) GetState() (protocol.Account, error) {
	state, err := r.batch.getAccountState(r.key.State(), nil)
	if err == nil {
		return state, nil
	}
	if r.url == nil && !errors.Is(err, errors.StatusNotFound) {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return nil, errors.FormatWithCause(errors.StatusNotFound, err, "account %v not found", r.url)
}

// GetStateAs loads the record state and unmarshals into the given value. In
// most cases `state` should be a double pointer.
func (r *Account) GetStateAs(state interface{}) error {
	err := r.batch.getAccountStateAs(r.key.State(), nil, state)
	if err == nil {
		return nil
	}
	if r.url == nil && !errors.Is(err, errors.StatusNotFound) {
		return errors.Wrap(errors.StatusUnknownError, err)
	}
	return errors.FormatWithCause(errors.StatusNotFound, err, "account %v not found", r.url)
}

// PutState stores the record state.
func (r *Account) PutState(state protocol.Account) error {
	// Does the record state have a URL?
	if state.GetUrl() == nil {
		return errors.New(errors.StatusInternalError, "invalid URL: empty")
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

	// Ensure metadata exists
	err := r.ensureMetadata(nil)
	if err != nil {
		return err
	}

	// Store the state
	r.batch.putValue(r.key.State(), state)
	return r.putBpt()
}

func (r *Account) pending() (*protocol.TxIdSet, error) {
	s := new(protocol.TxIdSet)
	err := r.batch.getValuePtr(r.key.Index("Pending"), s, &s, true)
	return s, err
}

func (r *Account) Pending() (*protocol.TxIdSet, error) {
	s, err := r.pending()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	return s, nil
}

func (r *Account) AddPending(txid *url.TxID) error {
	s, err := r.pending()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	s.Add(txid)
	r.batch.putValue(r.key.Index("Pending"), s)
	return r.putBpt()
}

func (r *Account) RemovePending(txid *url.TxID) error {
	s, err := r.pending()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	s.Remove(txid)
	r.batch.putValue(r.key.Index("Pending"), s)
	return r.putBpt()
}

func (r *Account) chain(name string, writable bool) (*Chain, error) {
	return newChain(r, r.key.Chain(name), writable)
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

func (r *Account) getSyntheticForAnchor(anchor [32]byte) (*protocol.TxIdSet, error) {
	v := new(protocol.TxIdSet)
	err := r.batch.getValuePtr(r.key.SyntheticForAnchor(anchor), v, &v, true)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	return v, nil
}

func (r *Account) AddSyntheticForAnchor(anchor [32]byte, txid *url.TxID) error {
	set, err := r.getSyntheticForAnchor(anchor)
	if err != nil {
		return err
	}

	set.Add(txid)
	r.batch.putValue(r.key.SyntheticForAnchor(anchor), set)
	return nil
}

func (r *Account) SyntheticForAnchor(anchor [32]byte) ([]*url.TxID, error) {
	set, err := r.getSyntheticForAnchor(anchor)
	if err != nil {
		return nil, err
	}

	return set.Entries, nil
}
