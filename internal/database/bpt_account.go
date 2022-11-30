// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

func (a *Account) VerifyHash(hash []byte) error {
	hasher, err := a.hashState()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !bytes.Equal(hash[:], hasher.MerkleHash()) {
		return errors.Conflict.WithFormat("hash does not match")
	}
	return nil
}

// PutBpt writes the record's BPT entry.
func (a *Account) putBpt() error {
	// Ensure the URL state is populated
	_, err := a.getUrl().Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		err = a.getUrl().Put(a.Url())
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	default:
		return errors.UnknownError.Wrap(err)
	}

	hasher, err := a.hashState()
	if err != nil {
		return err
	}

	hash := *(*[32]byte)(hasher.MerkleHash())
	a.parent.putBpt(a.key.Hash(), hash)
	return nil
}

// BptReceipt builds a BPT receipt for the account.
func (a *Account) BptReceipt() (*managed.Receipt, error) {
	if a.IsDirty() {
		return nil, errors.InternalError.With("cannot generate a BPT receipt when there are uncommitted changes")
	}

	bpt := pmt.NewBPTManager(a.parent.kvstore)
	receipt := bpt.Bpt.GetReceipt(a.key.Hash())
	if receipt == nil {
		return nil, errors.NotFound.WithFormat("BPT key %v not found", a.key.Hash())
	}

	return receipt, nil
}

// StateReceipt returns a Merkle receipt for the account state in the BPT.
func (a *Account) StateReceipt() (*managed.Receipt, error) {
	hasher, err := a.hashState()
	if err != nil {
		return nil, err
	}

	rBPT, err := a.BptReceipt()
	if err != nil {
		return nil, err
	}

	rState := hasher.Receipt(0, len(hasher)-1)
	if !bytes.Equal(rState.Anchor, rBPT.Start) {
		return nil, errors.InternalError.With("bpt entry does not match account state")
	}

	receipt, err := rState.Combine(rBPT)
	if err != nil {
		return nil, fmt.Errorf("combine receipt: %w", err)
	}

	return receipt, nil
}

// hashState returns a merkle hash of the account's main state, chains, and
// transactions.
func (a *Account) hashState() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	hashState(&err, &hasher, true, a.Main().Get)          // Add a simple hash of the main state
	hashState(&err, &hasher, false, a.hashSecondaryState) // Add a merkle hash of the Secondary State which is a list of accounts contained by the adi
	hashState(&err, &hasher, false, a.hashChains)         // Add a merkle hash of chains
	hashState(&err, &hasher, false, a.hashTransactions)   // Add a merkle hash of transactions
	return hasher, err
}

// hashChains returns a merkle hash of the DAG root of every chain in alphabetic
// order.
func (a *Account) hashChains() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, chainMeta := range loadState(&err, false, a.Chains().Get) {
		chain := loadState1(&err, false, a.GetChainByName, chainMeta.Name)
		if err != nil {
			break
		}

		if chain.head.Count == 0 {
			hasher.AddHash(new([32]byte))
		} else {
			hasher.AddHash((*[32]byte)(chain.head.GetMDRoot()))
		}
	}
	return hasher, err
}

// hashTransactions returns a merkle hash of the transaction hash and status of
// every pending transaction and every synthetic transaction waiting for an
// anchor.
func (a *Account) hashTransactions() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, txid := range loadState(&err, false, a.Pending().Get) {
		h := txid.Hash()
		hashState(&err, &hasher, false, a.parent.Transaction(h[:]).GetState)
		hashState(&err, &hasher, false, a.parent.Transaction(h[:]).GetStatus)
	}

	// // TODO Include this
	// for _, anchor := range loadState(&err, false, a.SyntheticAnchors().Get) {
	// 	hasher.AddHash(&anchor) //nolint:rangevarref
	// 	for _, txid := range loadState(&err, false, a.SyntheticForAnchor(anchor).Get) {
	// 		h := txid.Hash()
	// 		hashState(&err, &hasher, false, a.batch.Transaction(h[:]).GetState)
	// 		hashState(&err, &hasher, false, a.batch.Transaction(h[:]).GetStatus)
	// 	}
	// }

	return hasher, err
}

func zero[T any]() (z T) { return z }

func hashState[T any](lastErr *error, hasher *hash.Hasher, allowMissing bool, get func() (T, error)) {
	if *lastErr != nil {
		return
	}

	v, err := get()
	switch {
	case err == nil:
		// Ok
	case allowMissing && errors.Is(err, errors.NotFound):
		hasher.AddHash(new([32]byte))
		return
	default:
		*lastErr = err
		return
	}

	switch v := interface{}(v).(type) {
	case interface{ MerkleHash() []byte }:
		hasher.AddValue(v)
	case interface{ GetHash() []byte }:
		hasher.AddHash((*[32]byte)(v.GetHash()))
	case encoding.BinaryValue:
		data, err := v.MarshalBinary()
		if err != nil {
			*lastErr = err
			return
		}
		hasher.AddBytes(data)
	default:
		panic(fmt.Errorf("unhashable type %T", v))
	}
}

func loadState[T any](lastErr *error, allowMissing bool, get func() (T, error)) T {
	if *lastErr != nil {
		return zero[T]()
	}

	v, err := get()
	if allowMissing && errors.Is(err, errors.NotFound) {
		return zero[T]()
	}
	if err != nil {
		*lastErr = err
		return zero[T]()
	}

	return v
}

func loadState1[T, A1 any](lastErr *error, allowMissing bool, get func(A1) (T, error), a1 A1) T {
	if *lastErr != nil {
		return zero[T]()
	}

	v, err := get(a1)
	if allowMissing && errors.Is(err, errors.NotFound) {
		return zero[T]()
	}
	if err != nil {
		*lastErr = err
		return zero[T]()
	}

	return v
}

func (a *Account) hashSecondaryState() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, u := range loadState(&err, false, a.Directory().Get) {
		hasher.AddUrl(u)
	}
	// Hash the hash to allow for future expansion
	dirHash := hasher.MerkleHash()
	return hash.Hasher{dirHash}, err
}
