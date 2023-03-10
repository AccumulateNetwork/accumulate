// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package internal

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type databaseObserver struct{}

var _ database.Observer = databaseObserver{}

type observedAccount struct {
	*database.Account
	batch *database.Batch
}

func NewDatabaseObserver() database.Observer {
	return databaseObserver{}
}

func (databaseObserver) DidChangeAccount(batch *database.Batch, account *database.Account) (hash.Hasher, error) {
	a := observedAccount{account, batch}
	hasher, err := a.hashState()
	return hasher, errors.UnknownError.Wrap(err)
}

// hashState returns a merkle hash of the account's main state, chains, and
// transactions.
func (a *observedAccount) hashState() (hash.Hasher, error) {
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
func (a *observedAccount) hashChains() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, chainMeta := range loadState(&err, false, a.Chains().Get) {
		chain := loadState1(&err, false, a.GetChainByName, chainMeta.Name)
		if err != nil {
			break
		}

		if chain.CurrentState().Count == 0 {
			hasher.AddHash(new([32]byte))
		} else {
			hasher.AddHash((*[32]byte)(chain.CurrentState().GetMDRoot()))
		}
	}
	return hasher, err
}

// hashTransactions returns a merkle hash of the transaction hash and status of
// every pending transaction and every synthetic transaction waiting for an
// anchor.
func (a *observedAccount) hashTransactions() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, txid := range loadState(&err, false, a.Pending().Get) {
		h := txid.Hash()
		hashState(&err, &hasher, false, a.batch.Transaction(h[:]).GetState)
		hashState(&err, &hasher, false, a.batch.Transaction(h[:]).GetStatus)
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

func (a *observedAccount) hashSecondaryState() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, u := range loadState(&err, false, a.Directory().Get) {
		hasher.AddUrl(u)
	}
	// Hash the hash to allow for future expansion
	dirHash := hasher.MerkleHash()
	return hash.Hasher{dirHash}, err
}

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

func zero[T any]() (z T) { return z }

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
