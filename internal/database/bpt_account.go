package database

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
)

// loadState returns the fully realized account loadState.
func (a *Account) loadState(preserveChains bool) (*accountState, error) {
	s := new(accountState)

	// Load main state
	var err error
	s.Main = loadState(&err, true, a.Main().Get)

	// Load chain state
	for _, c := range loadState(&err, false, a.Chains().Get) {
		chain, err := a.ReadChain(c.Name)
		if err != nil {
			return nil, fmt.Errorf("load %s chain state: %w", c.Name, err)
		}

		ms1 := chain.CurrentState()
		ms2 := new(merkleState)
		ms2.Name = c.Name
		ms2.Type = c.Type
		s.Chains = append(s.Chains, ms2)

		if preserveChains {
			ms2.Entries, err = chain.Entries(0, ms1.Count)
			if err != nil {
				return nil, fmt.Errorf("load %s chain entries: %w", c.Name, err)
			}
		} else {
			ms2.Count = uint64(ms1.Count)
			ms2.Pending = make([][]byte, len(ms1.Pending))
			for i, v := range ms1.Pending {
				if len(v) == 0 {
					continue
				}
				ms2.Pending[i] = v
			}
		}
	}

	// Load transaction state
	for _, h := range loadState(&err, false, a.Pending().Get) {
		h := h.Hash()
		state, err := a.batch.Transaction(h[:]).loadState() //nolint:rangevarref
		if err != nil {
			return nil, err
		}
		s.Pending = append(s.Pending, state)
	}

	return s, nil
}

func (a *Account) stateOfTransactionsOnChain(name string) ([]*transactionState, error) {
	chain, err := a.ReadChain(name)
	if err != nil {
		return nil, fmt.Errorf("load main chain: %w", err)
	}

	// TODO We need to be more selective than this
	state := make([]*transactionState, chain.Height())
	for i := range state {
		hash, err := chain.Entry(int64(i))
		if err != nil {
			return nil, fmt.Errorf("load %s chain entry %d: %w", name, i, err)
		}

		state[i], err = a.batch.Transaction(hash).loadState()
		if err != nil {
			return nil, err
		}
	}

	return state, nil
}

func (a *Account) restoreState(s *accountState) error {
	// Store main state
	var err error
	saveState(&err, a.Main().Put, s.Main)

	// Store pending transaction list
	for _, p := range s.Pending {
		saveStateN(&err, a.Pending().Add, p.Transaction.ID())
	}

	// Store chain state
	for _, c := range s.Chains {
		head := new(managed.MerkleState)
		head.Count = int64(c.Count)
		head.Pending = make(managed.SparseHashList, len(c.Pending))
		for i, v := range c.Pending {
			if len(v) > 0 {
				head.Pending[i] = v
			}
		}
		mgr, err := a.Chain(c.Name, c.Type)
		if err != nil {
			return fmt.Errorf("store %s chain head: %w", c.Name, err)
		}
		err = mgr.merkle.Head().Put(head)
		if err != nil {
			return fmt.Errorf("store %s chain head: %w", c.Name, err)
		}
		for i, entry := range c.Entries {
			err := mgr.AddEntry(entry, false)
			if err != nil {
				return fmt.Errorf("store %s chain entry %d: %w", c.Name, c.Count+uint64(i), err)
			}
		}
	}

	// Store transaction state
	txns := make([]*transactionState, 0, len(s.Pending)+len(s.Transactions))
	txns = append(txns, s.Pending...)
	txns = append(txns, s.Transactions...)
	for _, p := range txns {
		hash := p.Transaction.GetHash()
		if len(p.State.Signers) != len(p.Signatures) {
			return fmt.Errorf("transaction %X state is invalid: %d signers and %d signatures", hash[:4], len(p.State.Signers), len(p.Signatures))
		}

		record := a.batch.Transaction(hash)
		err := record.PutState(&SigOrTxn{Transaction: p.Transaction})
		if err != nil {
			return fmt.Errorf("store transaction %X: %w", hash[:4], err)
		}

		err = record.PutStatus(p.State)
		if err != nil {
			return fmt.Errorf("store transaction %X status: %w", hash[:4], err)
		}

		for i, set := range p.Signatures {
			signer := p.State.Signers[i].GetUrl()
			err = record.getSignatures(signer).Put(set)
			if err != nil {
				return fmt.Errorf("store transaction %X signers %v: %w", hash[:4], signer, err)
			}
		}
	}

	return nil
}

// MerkleHash calculates the Merkle DAG root hash.
func (s *merkleState) MerkleHash() []byte {
	ms := new(managed.MerkleState)
	ms.InitSha256()
	ms.Count = int64(s.Count)
	ms.Pending = make(managed.SparseHashList, len(s.Pending))
	for i, v := range s.Pending {
		if len(v) > 0 {
			ms.Pending[i] = v
		}
	}
	for _, entry := range s.Entries {
		ms.AddToMerkleTree(entry)
	}

	return ms.GetMDRoot()
}

// PutBpt writes the record's BPT entry.
func (a *Account) putBpt() error {
	hasher, err := a.hashState()
	if err != nil {
		return err
	}

	hash := *(*[32]byte)(hasher.MerkleHash())
	a.batch.putBpt(a.key.Hash(), hash)
	return nil
}

// BptReceipt builds a BPT receipt for the account.
func (a *Account) BptReceipt() (*managed.Receipt, error) {
	if a.IsDirty() {
		return nil, errors.New(errors.StatusInternalError, "cannot generate a BPT receipt when there are uncommitted changes")
	}

	bpt := pmt.NewBPTManager(a.batch.store)
	receipt := bpt.Bpt.GetReceipt(a.key.Hash())
	if receipt == nil {
		return nil, errors.NotFound("BPT key %v not found", a.key.Hash())
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
		return nil, errors.New(errors.StatusInternalError, "bpt entry does not match account state")
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
		chain := loadState1(&err, false, a.ReadChain, chainMeta.Name)
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

func zero[T any]() (z T) { return z }

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

func loadState[T any](lastErr *error, allowMissing bool, get func() (T, error)) T {
	if *lastErr != nil {
		return zero[T]()
	}

	v, err := get()
	if allowMissing && errors.Is(err, errors.StatusNotFound) {
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
	if allowMissing && errors.Is(err, errors.StatusNotFound) {
		return zero[T]()
	}
	if err != nil {
		*lastErr = err
		return zero[T]()
	}

	return v
}

func saveState[T any](lastErr *error, put func(T) error, v T) {
	if *lastErr != nil || any(v) == nil {
		return
	}

	err := put(v)
	if err != nil {
		*lastErr = err
	}
}

func saveStateN[T any](lastErr *error, put func(...T) error, v ...T) {
	if *lastErr != nil || any(v) == nil {
		return
	}

	err := put(v...)
	if err != nil {
		*lastErr = err
	}
}

func (a *Account) hashSecondaryState() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	for _, u := range loadState(&err, false, a.Directory().Get) {
		hasher.AddUrl(u)
	}
	return hasher, err
}
