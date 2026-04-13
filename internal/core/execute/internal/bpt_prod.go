// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !debug

package internal

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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
	hashState(&err, &hasher, false, a.hashPending)        // Add a merkle hash of transactions
	return hasher, err
}

func (a *observedAccount) hashSecondaryState() (hash.Hasher, error) {
	var hasher hash.Hasher

	// Add the directory list
	var err error
	var dirHasher hash.Hasher
	for _, u := range loadState(&err, false, a.Directory().Get) {
		dirHasher.AddUrl(u)
	}
	hasher.AddValue(dirHasher)

	// Add scheduled events
	u := a.Url()
	if _, ok := protocol.ParsePartitionUrl(u); ok && u.PathEqual(protocol.Ledger) {
		// For backwards compatibility, don't add the hash if the BPT is empty
		hash := loadState(&err, false, a.Events().BPT().GetRootHash)
		if hash != [32]byte{} {
			hasher.AddHash2(hash)
		}
	}

	return hasher, err
}

// hashChains returns a merkle hash of the DAG root of every chain in
// alphabetical order.
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
			hasher.AddHash((*[32]byte)(chain.CurrentState().Anchor()))
		}
	}
	return hasher, err
}

// hashPending returns a merkle hash of the transaction hash and status of
// every pending transaction and every synthetic transaction waiting for an
// anchor.
func (a *observedAccount) hashPending() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher

	for _, txid := range loadState(&err, false, a.Pending().Get) {
		// V1 BPT logic for pending transactions
		v1 := a.batch.Transaction2(txid.Hash())
		isV1 := loadState(&err, true, v1.Main().Get) != nil
		if isV1 {
			hashState(&err, &hasher, false, v1.Main().Get)
			hashState(&err, &hasher, false, v1.Status().Get)
		}

		// V2 BPT logic for pending transactions
		if !isV1 {
			// If the transaction is not a V1 transaction, add its hash directly
			hasher.AddTxID(txid)
		}
		a.hashPendingV2(&err, &hasher, txid)
	}

	// If the account is a page, look on the book for pending transactions
	page, ok := loadState(&err, true, a.Main().Get).(*protocol.KeyPage)
	if !ok {
		return hasher, err
	}
	for _, txid := range loadState(&err, false, a.batch.Account(page.GetAuthority()).Pending().Get) {
		a.hashPendingV2(&err, &hasher, txid)
	}

	return hasher, err
}

func (a *observedAccount) hashPendingV2(err *error, hasher *hash.Hasher, txid *url.TxID) {
	txn := a.Transaction(txid.Hash())

	// Validator signatures
	for _, sig := range loadState(err, true, txn.ValidatorSignatures().Get) {
		hasher.AddHash((*[32]byte)(sig.Hash()))
	}

	// Credit payments
	for _, hash := range loadState(err, true, txn.Payments().Get) {
		hasher.AddHash2(hash)
	}

	// Authority votes
	for _, entry := range loadState(err, true, txn.Votes().Get) {
		hashValue(err, hasher, entry)
	}

	// Active signatures
	for _, entry := range loadState(err, true, txn.Signatures().Get) {
		hashValue(err, hasher, entry)
	}
}

func hashState[T any](lastErr *error, hasher *hash.Hasher, allowMissing bool, get func() (T, error)) {
	if *lastErr != nil {
		return
	}

	v, err := get()
	switch {
	case err == nil:
		hashValue(lastErr, hasher, v)
	case allowMissing && errors.Is(err, errors.NotFound):
		hasher.AddHash(new([32]byte))
	default:
		*lastErr = err
	}
}

func hashValue(lastErr *error, hasher *hash.Hasher, v any) {
	if *lastErr != nil {
		return
	}

	switch v := v.(type) {
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
		h := storage.MakeKey(v)
		hasher.AddHash2(h)
	}
}
