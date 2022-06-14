package database

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func (c *ChangeSet) BptRoot() []byte {
	bpt := pmt.NewBPTManager(c.store)
	return bpt.Bpt.RootHash[:]
}

// BptReceipt builds a BPT receipt for the given key.
func (c *ChangeSet) BptReceipt(key storage.Key, value [32]byte) (*managed.Receipt, error) {
	if len(c.bptEntries) > 0 {
		return nil, errors.New(errors.StatusInternalError, "cannot generate a BPT receipt when there are uncommitted BPT entries")
	}

	bpt := pmt.NewBPTManager(c.store)
	receipt := bpt.Bpt.GetReceipt(key)
	if receipt == nil {
		return nil, errors.NotFound("BPT key %v not found", key)
	}

	return receipt, nil
}

// StateReceipt returns a Merkle receipt for the account state in the BPT.
func (a *Account) StateReceipt() (*managed.Receipt, error) {
	hasher, err := a.hashState()
	if err != nil {
		return nil, err
	}

	rBPT, err := a.container.BptReceipt(a.key.Hash(), *(*[32]byte)(hasher.MerkleHash()))
	if err != nil {
		return nil, err
	}

	rState := hasher.Receipt(0, len(hasher)-1)
	if !bytes.Equal(rState.Anchor, rBPT.Start) {
		return nil, errors.New(errors.StatusInternalError, "bpt entry does not match account state")
	}

	receipt, err := rState.Combine(rBPT)
	if err != nil {
		return nil, fmt.Errorf("combine receipt: %w", err)
	}

	return receipt, nil
}

func (c *ChangeSet) updateBPT(store storage.KeyValueTxn) error {
	bpt := pmt.NewBPTManager(store)

	// For each modified account
	for key, account := range c.account {
		if !account.isDirty() {
			continue
		}

		// Hash the account
		hash, err := account.hashState()
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

func (a *Account) hashState() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	hashState(&err, &hasher, true, a.State().Get)
	hashState(&err, &hasher, false, a.hashChains)
	hashState(&err, &hasher, false, func() (hash.Hasher, error) { return a.hashTransactions() })
	return hasher, err
}

func (a *Account) hashChains() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, chainMeta := range loadState(&err, false, a.Chains().Get) {
		chain := loadState1(&err, false, a.ChainByName, chainMeta.Name)
		if err != nil {
			break
		}

		state := loadState(&err, false, chain.State().Get)
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

func (a *Account) hashTransactions() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, txid := range loadState(&err, false, a.Pending().Get) {
		h := txid.Hash()
		hashState(&err, &hasher, false, a.container.Transaction(h[:]).Value().Get)
		hashState(&err, &hasher, false, a.container.Transaction(h[:]).Status().Get)
	}
	for _, anchor := range loadState(&err, false, a.SyntheticAnchors().Get) {
		hasher.AddHash(&anchor) //nolint:rangevarref
		for _, txid := range loadState(&err, false, a.SyntheticForAnchor(anchor).Get) {
			h := txid.Hash()
			hashState(&err, &hasher, false, a.container.Transaction(h[:]).Value().Get)
			hashState(&err, &hasher, false, a.container.Transaction(h[:]).Status().Get)
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
