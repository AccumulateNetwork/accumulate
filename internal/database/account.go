package database

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

func (r *Account) url() *url.URL {
	return r.key[1].(*url.URL)
}

func (a *Account) Commit() error {
	if !a.IsDirty() {
		return nil
	}

	meta, err := a.Object().Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}
	if meta.Type == protocol.ObjectTypeUnknown {
		meta.Type = protocol.ObjectTypeAccount
	}

	for _, c := range a.chains {
		if !c.IsDirty() {
			continue
		}

		err = c.Commit()
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	err = a.putBpt()
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Do the normal commit stuff
	err = a.baseCommit()
	return errors.Wrap(errors.StatusUnknownError, err)
}

func (a *Account) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) >= 2 && key[0] == "Chain" {
		name, ok := key[1].(string)
		if ok {
			return a.chain(name), key[2:], nil
		}
	}

	return a.baseResolve(key)
}

// GetObject loads the object metadata.
func (r *Account) GetObject() (*protocol.Object, error) {
	return r.Object().Get()
}

// GetState loads the record state.
func (r *Account) GetState() (protocol.Account, error) {
	return r.Main().Get()
}

// GetStateAs loads the record state and unmarshals into the given value. In
// most cases `state` should be a double pointer.
func (r *Account) GetStateAs(state interface{}) error {
	return r.Main().GetAs(state)
}

// PutState stores the record state.
func (r *Account) PutState(state protocol.Account) error {
	// Does the record state have a URL?
	if state.GetUrl() == nil {
		return errors.New(errors.StatusInternalError, "invalid URL: empty")
	}

	// Is this the right URL - does it match the record's key?
	if !r.url().Equal(state.GetUrl()) {
		return fmt.Errorf("mismatched url: key is %v, URL is %v", r.url(), state.GetUrl())
	}

	// Make sure the key book is set
	account, ok := state.(protocol.FullAccount)
	if ok && len(account.GetAuth().Authorities) == 0 {
		return fmt.Errorf("missing key book")
	}

	// Store the state
	err := r.Main().Put(state)
	return errors.Wrap(errors.StatusUnknownError, err)
}

func (r *Account) GetPending() (*protocol.TxIdSet, error) {
	v, err := r.Pending().Get()
	if err != nil {
		return nil, err
	}
	return &protocol.TxIdSet{Entries: v}, nil
}

func (r *Account) AddPending(txid *url.TxID) error {
	return r.Pending().Add(txid)
}

func (r *Account) RemovePending(txid *url.TxID) error {
	return r.Pending().Remove(txid)
}

func (a *Account) ensureChain(name string, typ managed.ChainType) error {
	meta, err := a.Object().Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = meta.AddChain(name, typ)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return a.Object().Put(meta)
}

// Chain returns a chain manager for the given chain.
func (r *Account) Chain(name string, typ protocol.ChainType) (*Chain, error) {
	err := r.ensureChain(name, typ)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return newChain(r, r.chain(name), true)
}

// IndexChain returns a chain manager for the index chain of the given chain.
func (r *Account) IndexChain(name string, major bool) (*Chain, error) {
	return r.Chain(protocol.IndexChain(name, major), protocol.ChainTypeIndex)
}

func (r *Account) chain(name string) *managed.Chain {
	name = strings.ToLower(name)
	key := r.key.Append("Chain", name)
	return getOrCreateMap(&r.chains, key, func() *managed.Chain {
		return managed.NewChain(r.batch.logger.L, r.batch.recordStore, key, markPower, name, "account %[2]s chain %[4]s")
	})
}

// ReadChain returns a read-only chain manager for the given chain.
func (r *Account) ReadChain(name string) (*Chain, error) {
	return newChain(r, r.chain(name), false)
}

// ReadIndexChain returns a read-only chain manager for the index chain of the given chain.
func (r *Account) ReadIndexChain(name string, major bool) (*Chain, error) {
	return r.ReadChain(protocol.IndexChain(name, major))
}

func (r *Account) AddSyntheticForAnchor(anchor [32]byte, txid *url.TxID) error {
	return r.SyntheticForAnchor(anchor).Add(txid)
}

func (r *Account) GetSyntheticForAnchor(anchor [32]byte) ([]*url.TxID, error) {
	return r.SyntheticForAnchor(anchor).Get()
}
