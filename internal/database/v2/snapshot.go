package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

func (a *Account) loadState(includeHistory bool) (*fullAccountState, error) {
	var err error
	s := new(fullAccountState)
	s.State = loadState(&err, a.State().Get)
	s.Pending = loadState(&err, a.Pending().Get)
	s.Chains = loadState1(&err, a.loadChains, includeHistory)
	for _, anchor := range loadState(&err, a.SyntheticAnchors().Get) {
		txids := loadState(&err, a.SyntheticForAnchor(anchor).Get)
		entry := &hashAndTxidSet{Anchor: anchor, Txids: txids}
		s.SyntheticForAnchor = append(s.SyntheticForAnchor, entry)
	}
	for _, txid := range s.Pending {
		txn := loadState(&err, a.container.Transaction(txid.Hash()).loadState)
		s.Transactions = append(s.Transactions, txn)
	}
	return s, errors.Wrap(errors.StatusUnknown, err)
}

func (a *Account) loadChains(includeHistory bool) ([]*chainState, error) {
	var err error
	var chains []*chainState
	for _, c := range loadState(&err, a.Chains().Get) {
		cs := new(chainState)
		cs.Name = c.Name
		cs.Type = c.Type
		chains = append(chains, cs)

		var chain *Chain
		chain, err = a.ChainByName(c.Name)
		if err != nil {
			break
		}

		var ms *managed.MerkleState
		ms, err = chain.State().Get()
		if err != nil {
			break
		}

		if includeHistory {
			cs.Entries, err = chain.Entries(0, ms.Count)
			if err != nil {
				break
			}
		} else {
			cs.Count = uint64(ms.Count)
			cs.Pending = make([][]byte, len(ms.Pending))
			for i, v := range ms.Pending {
				cs.Pending[i] = v
			}
		}
	}
	return chains, err
}

func (t *Transaction) loadState() (*fullTransactionState, error) {
	var err error
	s := new(fullTransactionState)
	s.Hash = t.key[1].([32]byte)
	s.Value = loadState(&err, t.Value().Get)
	s.Status = loadState(&err, t.Status().Get)
	for _, signer := range loadState(&err, t.Signers().Get) {
		set := t.Signatures(signer)
		ss := new(signerAndSigSet)
		ss.Signer = signer
		ss.Version = loadState(&err, set.getVersion)
		ss.Signatures = loadState(&err, set.Get)
		s.Signatures = append(s.Signatures, ss)
	}
	return s, errors.Wrap(errors.StatusUnknown, err)
}

func (a *Account) restoreState(state *fullAccountState) error {
	var err error
	saveState(&err, a.State().Put, state.State)
	saveState(&err, a.Pending().Put, state.Pending)
	saveState(&err, a.restoreChains, state.Chains)
	for _, entry := range state.SyntheticForAnchor {
		saveState(&err, a.SyntheticForAnchor(entry.Anchor).Put, entry.Txids)
	}
	for _, entry := range state.Transactions {
		saveState(&err, a.container.Transaction(entry.Hash).restoreState, entry)
	}
	return err
}

func (a *Account) restoreChains(entries []*chainState) error {
	var err error
	for _, entry := range entries {
		ms := new(managed.MerkleState)
		ms.InitSha256()
		ms.Count = int64(entry.Count)
		ms.Pending = make(managed.SparseHashList, len(entry.Pending))
		for i, v := range entry.Pending {
			ms.Pending[i] = v
		}

		chain := loadState1(&err, a.ChainByName, entry.Name)
		if err != nil {
			break
		}
		saveState(&err, chain.State().Put, ms)

		for _, entry := range entry.Entries {
			err = chain.AddHash(entry, false)
			if err != nil {
				break
			}
		}
	}
	return err
}

func (t *Transaction) restoreState(s *fullTransactionState) error {
	var err error
	saveState(&err, t.Value().Put, s.Value)
	saveState(&err, t.Status().Put, s.Status)
	for _, entry := range s.Signatures {
		set := t.Signatures(entry.Signer)
		saveState(&err, set.putVersion, entry.Version)
		saveState(&err, set.putEntries, entry.Signatures)
	}
	return err
}

func loadState[T any](lastErr *error, get func() (T, error)) T {
	if *lastErr != nil {
		return zero[T]()
	}

	v, err := get()
	if err != nil {
		*lastErr = err
		return zero[T]()
	}

	return v
}

func loadState1[T, A1 any](lastErr *error, get func(A1) (T, error), a1 A1) T {
	if *lastErr != nil {
		return zero[T]()
	}

	v, err := get(a1)
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
