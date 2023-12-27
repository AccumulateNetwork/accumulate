// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build debug

package internal

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (databaseObserver) DidChangeAccount(batch *database.Batch, account *database.Account) (hash.Hasher, error) {
	a := observedAccount{account, batch}
	hasher, err := a.hashState()

	h := hasher.Hash()
	return hash.Hasher{h[:]}, errors.UnknownError.Wrap(err)
}

// hashState returns a merkle hash of the account's main state, chains, and
// transactions.
func (a *observedAccount) hashState() (hashable, error) {
	var err error
	var h hashSet
	loadAndHashValue(&err, &h, true, a.Main().Get, "main")        // Add a simple hash of the main state
	hashComplexState(&err, &h, a.hashSecondaryState, "secondary") // Add a merkle hash of the Secondary State which is a list of accounts contained by the adi
	hashComplexState(&err, &h, a.hashChains, "chains")            // Add a merkle hash of chains
	hashComplexState(&err, &h, a.hashPending, "pending")          // Add a merkle hash of transactions
	return &h, err
}

func (a *observedAccount) hashSecondaryState(h *hashSet) error {
	// Add the directory list
	var err error
	dirHasher := h.Child("directory")
	for _, u := range loadState(&err, false, a.Directory().Get) {
		hashUrl(dirHasher, u, u)
	}

	// Add scheduled events
	u := a.Url()
	if _, ok := protocol.ParsePartitionUrl(u); ok && u.PathEqual(protocol.Ledger) {
		// For backwards compatibility, don't add the hash if the BPT is empty
		hash := loadState(&err, false, a.Events().BPT().GetRootHash)
		if hash != [32]byte{} {
			h.Add(rawHash(hash), "events")
		}
	}

	return err
}

// hashChains returns a merkle hash of the DAG root of every chain in
// alphabetical order.
func (a *observedAccount) hashChains(hs *hashSet) error {
	var err error
	for _, chainMeta := range loadState(&err, false, a.Chains().Get) {
		chain := loadState1(&err, false, a.GetChainByName, chainMeta.Name)
		if err != nil {
			break
		}

		if chain.CurrentState().Count == 0 {
			hs.Add(rawHash{}, chainMeta.Name)
		} else {
			hs.Add(rawHash(*(*[32]byte)(chain.CurrentState().Anchor())), chainMeta.Name)
		}
	}
	return err
}

// hashPending returns a merkle hash of the transaction hash and status of
// every pending transaction and every synthetic transaction waiting for an
// anchor.
func (a *observedAccount) hashPending(h *hashSet) error {
	var err error
	// var hasher hash.Hasher

	for _, txid := range loadState(&err, false, a.Pending().Get) {
		// V1 BPT logic for pending transactions
		v1 := a.batch.Transaction2(txid.Hash())
		isV1 := loadState(&err, true, v1.Main().Get) != nil
		if isV1 {
			loadAndHashValue(&err, h, false, v1.Main().Get, stringers{txid, "main"})
			loadAndHashValue(&err, h, false, v1.Status().Get, stringers{txid, "status"})
		}

		// V2 BPT logic for pending transactions
		if !isV1 {
			// If the transaction is not a V1 transaction, add its hash directly
			hashTxID(h, txid, txid)
		}
		a.hashPendingV2(&err, h, txid)
	}

	// If the account is a page, look on the book for pending transactions
	page, ok := loadState(&err, true, a.Main().Get).(*protocol.KeyPage)
	if !ok {
		return err
	}
	for _, txid := range loadState(&err, false, a.batch.Account(page.GetAuthority()).Pending().Get) {
		a.hashPendingV2(&err, h, txid)
	}

	return err
}

func (a *observedAccount) hashPendingV2(err *error, h *hashSet, txid *url.TxID) {
	txn := a.Transaction(txid.Hash())

	// Validator signatures
	for i, sig := range loadState(err, true, txn.ValidatorSignatures().Get) {
		h.Add(rawHash(*(*[32]byte)(sig.Hash())), stringers{txid, "signature", i})
	}

	// Credit payments
	for i, hash := range loadState(err, true, txn.Payments().Get) {
		h.Add(rawHash(hash), stringers{txid, "payment", i})
	}

	// Authority votes
	for i, entry := range loadState(err, true, txn.Votes().Get) {
		hashValue(err, h, entry, stringers{txid, "vote", i})
	}

	// Active signatures
	for i, entry := range loadState(err, true, txn.Signatures().Get) {
		hashValue(err, h, entry, stringers{txid, "active", i})
	}
}

func loadAndHashValue[T encoding.BinaryValue](lastErr *error, h *hashSet, allowMissing bool, get func() (T, error), memo any) {
	if *lastErr != nil {
		return
	}

	v, err := get()
	switch {
	case err == nil:
		hashValue(&err, h, v, memo)
	case allowMissing && errors.Is(err, errors.NotFound):
		h.Add(rawHash{}, memo)
	default:
		*lastErr = err
	}
}

func hashComplexState(lastErr *error, h *hashSet, hash func(*hashSet) error, memo string) {
	if *lastErr != nil {
		return
	}

	err := hash(h.Child(memo))
	if err != nil {
		*lastErr = err
	}
}

func hashValue(lastErr *error, h *hashSet, v encoding.BinaryValue, memo any) {
	data, err := v.MarshalBinary()
	if err != nil {
		*lastErr = err
		return
	}

	hashBytes(h, data, memo)
}

func hashUrl(h *hashSet, v *url.URL, memo any) {
	if v == nil {
		h.Add(rawHash{}, memo)
	} else {
		hashString(h, v.String(), memo)
	}
}

func hashUrl2(h *hashSet, v *url.URL, memo any) {
	if v == nil {
		h.Add(rawHash{}, memo)
	} else {
		h.Add(rawHash(v.Hash32()), memo)
	}
}

func hashTxID(h *hashSet, v *url.TxID, memo any) {
	if v == nil {
		h.Add(rawHash{}, memo)
	} else {
		u := v.Hash()
		x := hash.Combine(u[:], v.Account().Hash())
		h.Add(rawHash(*(*[32]byte)(x)), memo)
	}
}

func hashString(h *hashSet, v string, memo any) {
	hashBytes(h, []byte(v), memo)
}

func hashBytes(h *hashSet, v []byte, memo any) {
	h.Add(rawHash(sha256.Sum256(v)), memo)
}
