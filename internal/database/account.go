// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (b *Batch) Account(u *url.URL) *Account {
	return b.getAccount(u.StripExtras())
}

func (b *Batch) AccountTransaction(id *url.TxID) *AccountTransaction {
	return b.Account(id.Account()).Transaction(id.Hash())
}

// Hash retrieves or calculates the state hash of the account.
func (a *Account) Hash() ([32]byte, error) {
	// TODO Retrieve from the BPT
	h, err := a.parent.observer.DidChangeAccount(a.parent, a)
	return *(*[32]byte)(h.MerkleHash()), err
}

func UpdateAccount[T protocol.Account](batch *Batch, url *url.URL, fn func(T) error) (T, error) {
	record := batch.Account(url).Main()

	var account T
	err := record.GetAs(&account)
	if err != nil {
		return account, errors.UnknownError.WithFormat("load %v: %w", url, err)
	}

	err = fn(account)
	if err != nil {
		return account, errors.UnknownError.Wrap(err)
	}

	err = record.Put(account)
	if err != nil {
		return account, errors.UnknownError.WithFormat("store %v: %w", url, err)
	}

	return account, nil
}

func (r *Account) Url() *url.URL {
	return r.key[1].(*url.URL)
}

func (a *Account) Commit() error {
	if !a.IsDirty() {
		return nil
	}

	if fieldIsDirty(a.main) {
		acc, err := a.Main().Get()
		switch {
		case err == nil:
			// Strip the URL of user info, query, and fragment
			u := acc.GetUrl()
			if !u.StripExtras().Equal(u) {
				acc.StripUrl()
				u = acc.GetUrl()

				err = a.Main().Put(acc)
				if err != nil {
					return errors.BadRequest.WithFormat("strip url: %w", err)
				}
			}

			if len(u.String()) > protocol.AccountUrlMaxLength {
				return errors.BadUrlLength.Wrap(fmt.Errorf("url specified exceeds maximum character length: %s", u.String()))
			}

			err = protocol.IsValidAccountPath(u.Path)
			if err != nil {
				return errors.BadRequest.WithFormat("invalid path: %w", err)
			}

		case errors.Is(err, errors.NotFound):
			// The main state is unset so there's nothing to check
		default:
			return errors.UnknownError.WithFormat("load state: %w", err)
		}
	}

	// Ensure chains are added to the Chains index
	var chains []*protocol.ChainMetadata
	for _, c := range a.dirtyChains() {
		chains = append(chains, &protocol.ChainMetadata{
			Name: c.name,
			Type: c.typ,
		})
	}
	err := a.Chains().Add(chains...)
	if err != nil {
		return errors.UnknownError.WithFormat("update chains index: %w", err)
	}

	// Ensure the synthetic anchors index is up to date
	for k, set := range a.syntheticForAnchor {
		if !set.IsDirty() {
			continue
		}

		err := a.SyntheticAnchors().Add(k.Anchor)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// If anything has changed, update the BPT entry
	err = a.putBpt()
	if err != nil {
		return errors.UnknownError.WithFormat("update BPT entry for %v: %w", a.Url(), err)
	}

	// Do the normal commit stuff
	err = a.baseCommit()
	return errors.UnknownError.Wrap(err)
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
		return errors.InternalError.With("invalid URL: empty")
	}

	// Is this the right URL - does it match the record's key?
	if !r.Url().Equal(state.GetUrl()) {
		return fmt.Errorf("mismatched url: key is %v, URL is %v", r.Url(), state.GetUrl())
	}

	// Make sure the key book is set
	account, ok := state.(protocol.FullAccount)
	if ok && len(account.GetAuth().Authorities) == 0 {
		return fmt.Errorf("missing key book")
	}

	// Store the state
	err := r.Main().Put(state)
	return errors.UnknownError.Wrap(err)
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

func (r *Account) AddSyntheticForAnchor(anchor [32]byte, txid *url.TxID) error {
	return r.SyntheticForAnchor(anchor).Add(txid)
}

func (r *Account) GetSyntheticForAnchor(anchor [32]byte) ([]*url.TxID, error) {
	return r.SyntheticForAnchor(anchor).Get()
}

func compareSignatureByKey(a, b protocol.KeySignature) int {
	return bytes.Compare(a.GetPublicKey(), b.GetPublicKey())
}
