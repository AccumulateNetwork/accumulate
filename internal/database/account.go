package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func UpdateAccount[T protocol.Account](batch *Batch, url *url.URL, fn func(T) error) (T, error) {
	record := batch.Account(url).Main()

	var account T
	err := record.GetAs(&account)
	if err != nil {
		return account, errors.Format(errors.StatusUnknownError, "load %v: %w", url, err)
	}

	err = fn(account)
	if err != nil {
		return account, errors.Wrap(errors.StatusUnknownError, err)
	}

	err = record.Put(account)
	if err != nil {
		return account, errors.Format(errors.StatusUnknownError, "store %v: %w", url, err)
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
			if len(acc.GetUrl().String()) > protocol.AccountUrlMaxLength {
				return errors.Wrap(errors.StatusBadUrlLength, fmt.Errorf("url specified exceeds maximum character length: %s", acc.GetUrl().String()))
			}
			err = protocol.IsValidAccountPath(acc.GetUrl().Path)
			if err != nil {
				return errors.Format(errors.StatusBadRequest, "invalid path: %w", err)
			}
		case errors.Is(err, errors.StatusNotFound):
			// The main state is unset so there's nothing to check
		default:
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// Ensure the synthetic anchors index is up to date
	for k, set := range a.syntheticForAnchor {
		if !set.IsDirty() {
			continue
		}

		err := a.SyntheticAnchors().Add(k.Anchor)
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// If anything has changed, update the BPT entry
	err := a.putBpt()
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "update BPT entry for %v: %w", a.Url(), err)
	}

	// Do the normal commit stuff
	err = a.baseCommit()
	return errors.Wrap(errors.StatusUnknownError, err)
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

func (r *Account) AddSyntheticForAnchor(anchor [32]byte, txid *url.TxID) error {
	return r.SyntheticForAnchor(anchor).Add(txid)
}

func (r *Account) GetSyntheticForAnchor(anchor [32]byte) ([]*url.TxID, error) {
	return r.SyntheticForAnchor(anchor).Get()
}
