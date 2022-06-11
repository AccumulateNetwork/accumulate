package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func (c *ChangeSet) updateBPT(store storage.KeyValueTxn) error {
	bpt := pmt.NewBPTManager(store)

	// For each modified account
	for key, account := range c.account {
		if !account.isDirty() {
			continue
		}

		// Hash the account
		hash, err := account.hashState(c)
		if err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}

		// Update the BPT entry
		bpt.InsertKV(key, *(*[32]byte)(hash.MerkleHash()))
	}

	err := bpt.Bpt.Update()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (a *Account) hashState(cs *ChangeSet) (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	hashState(&err, &hasher, true, a.State().Get)
	hashState(&err, &hasher, false, a.hashChains)
	hashState(&err, &hasher, false, func() (hash.Hasher, error) { return a.hashTransactions(cs) })
	return hasher, err
}

func (a *Account) hashChains() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, chainMeta := range loadState(&err, a.Chains().Get) {
		chain := loadState1(&err, a.ChainByName, chainMeta.Name)
		if err != nil {
			break
		}

		state := loadState(&err, chain.State().Get)
		if err != nil {
			break
		}

		if state.Count == 0 {
			hasher.AddHash(new([32]byte))
		} else {
			hasher.AddHash((*[32]byte)(state.GetMDRoot()))
		}
	}
	return hasher, err
}

func (a *Account) hashTransactions(cs *ChangeSet) (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, txid := range loadState(&err, a.Pending().Get) {
		hashState(&err, &hasher, false, cs.Transaction(txid.Hash()).Value().Get)
		hashState(&err, &hasher, false, cs.Transaction(txid.Hash()).Status().Get)
	}
	for _, anchor := range loadState(&err, a.SyntheticAnchors().Get) {
		hasher.AddHash(&anchor) //nolint:rangevarref
		for _, txid := range loadState(&err, a.SyntheticForAnchor(anchor).Get) {
			hashState(&err, &hasher, false, cs.Transaction(txid.Hash()).Value().Get)
			hashState(&err, &hasher, false, cs.Transaction(txid.Hash()).Status().Get)
		}
	}
	return hasher, err
}

func hashState[T any](lastErr *error, hasher *hash.Hasher, allowMissing bool, get func() (T, error)) {
	if *lastErr != nil {
		return
	}

	v, err := get()
	switch {
	case err == nil:
		// Ok
	case allowMissing && errors.Is(err, errors.StatusNotFound):
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
