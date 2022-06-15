package database

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// SaveSnapshot writes the full state of the partition out to a file.
func (c *ChangeSet) SaveSnapshot(file io.WriteSeeker, network *config.Describe) error {
	/*synthetic := object("Account", network.Synthetic())
	subnet := network.NodeUrl()*/

	// Write the block height
	var ledger *protocol.SystemLedger
	err := c.Account(network.Ledger()).GetStateAs(&ledger)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load ledger: %w", err)
	}
	var v [8]byte
	binary.BigEndian.PutUint64(v[:], ledger.Index)
	_, err = file.Write(v[:])
	if err != nil {
		return errors.Format(errors.StatusUnknown, "write height: %w", err)
	}

	// Write the BPT root hash
	bpt := pmt.NewBPTManager(c.kvStore)
	_, err = file.Write(bpt.Bpt.RootHash[:])
	if err != nil {
		return errors.Format(errors.StatusUnknown, "write BPT root: %w", err)
	}

	// Create a section writer starting after the header
	wr, err := ioutil2.NewSectionWriter(file, -1, -1)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "create section writer: %w", err)
	}

	// Save the snapshot
	return bpt.Bpt.SaveSnapshot(wr, func(key storage.Key, hash [32]byte) ([]byte, error) {
		// Create an Account object
		account, err := c.accountByKey(key)
		if err != nil {
			return nil, err
		}

		/*// Load the main state so we can get the URL
		a, err := account.GetState()
		if err != nil {
			return nil, err
		}

		// Load the full state - preserve chains if the account is a subnet account
		state, err := account.state(true, subnet.PrefixOf(a.GetUrl()))*/

		// Load the full state - always preserve chains for now
		state, err := account.loadState(true)
		if err != nil {
			return nil, err
		}

		hasher, err := account.hashState()
		if err != nil {
			return nil, err
		}

		if !bytes.Equal(hash[:], hasher.MerkleHash()) {
			return nil, fmt.Errorf("hash does not match for %v\n", state.State.GetUrl())
		}

		/*if objectBucket(key) != synthetic {
			return state.MarshalBinary()
		}*/

		mainChainState, err := account.MainChain().Head().Get()
		if err != nil {
			return nil, err
		}
		for _, entry := range loadState2(&err, false, account.MainChain().Entries, 0, mainChainState.Count-1) {
			txn := loadState(&err, false, c.Transaction(entry).loadState)
			/*if txn.State.Delivered {
				continue
			}*/
			state.Transactions = append(state.Transactions, txn)
		}

		return state.MarshalBinary()
	})
}

// ReadSnapshot reads a snapshot file, returning the header values and a reader.
func ReadSnapshot(file ioutil2.SectionReader) (height uint64, format uint32, bptRoot []byte, rd ioutil2.SectionReader, err error) {
	// Load the header
	var bytes [40]byte
	_, err = io.ReadFull(file, bytes[:])
	if err != nil {
		return 0, 0, nil, nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Make a new section reader starting after the header
	rd, err = ioutil2.NewSectionReader(file, -1, -1)
	if err != nil {
		return 0, 0, nil, nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return binary.BigEndian.Uint64(bytes[:8]), core.SnapshotVersion1, bytes[8:], rd, nil
}

// RestoreSnapshot loads the full state of the partition from a file.
func (c *ChangeSet) RestoreSnapshot(file ioutil2.SectionReader) error {
	// Read the snapshot
	_, _, _, rd, err := ReadSnapshot(file)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Load the snapshot
	bpt := pmt.NewBPTManager(c.kvStore)
	return bpt.Bpt.LoadSnapshot(rd, func(_ storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
		state := new(fullAccountState)
		err := state.UnmarshalBinaryFrom(reader)
		if err != nil {
			return err
		}

		account := c.Account(state.State.GetUrl())
		err = account.restoreState(state)
		if err != nil {
			return err
		}

		hasher, err := account.hashState()
		if err != nil {
			return err
		}

		if !bytes.Equal(hash[:], hasher.MerkleHash()) {
			return fmt.Errorf("hash does not match for %v", state.State.GetUrl())
		}

		return nil
	})
}

func (a *Account) loadState(includeHistory bool) (*fullAccountState, error) {
	var err error
	s := new(fullAccountState)
	s.State = loadState(&err, false, a.State().Get)
	s.Pending = loadState(&err, false, a.Pending().Get)
	s.Chains = loadState1(&err, false, a.loadChains, includeHistory)
	for _, anchor := range loadState(&err, false, a.SyntheticAnchors().Get) {
		txids := loadState(&err, false, a.SyntheticForAnchor(anchor).Get)
		entry := &hashAndTxidSet{Anchor: anchor, Txids: txids}
		s.SyntheticForAnchor = append(s.SyntheticForAnchor, entry)
	}
	for _, txid := range s.Pending {
		h := txid.Hash()
		txn := loadState(&err, false, a.container.Transaction(h[:]).loadState)
		s.Transactions = append(s.Transactions, txn)
	}
	return s, errors.Wrap(errors.StatusUnknown, err)
}

func (a *Account) loadChains(includeHistory bool) ([]*chainState, error) {
	var err error
	var chains []*chainState
	for _, c := range loadState(&err, false, a.Chains().Get) {
		cs := new(chainState)
		cs.Name = c.Name
		cs.Type = c.Type
		chains = append(chains, cs)

		var chain *managed.Chain
		chain, err = a.ChainByName(c.Name)
		if err != nil {
			break
		}

		var ms *managed.MerkleState
		ms, err = chain.Head().Get()
		if err != nil {
			break
		}

		if includeHistory {
			cs.Entries = loadState2(&err, false, chain.Entries, 0, ms.Count)
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
	s.Value = loadState(&err, false, t.Value().Get)
	s.Status = loadState(&err, false, t.Status().Get)
	for _, signer := range loadState(&err, false, t.Signers().Get) {
		set := t.Signatures(signer)
		ss := new(signerAndSigSet)
		ss.Signer = signer
		ss.Version = loadState(&err, false, set.getVersion)
		ss.Signatures = loadState(&err, false, set.Get)
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
		saveState(&err, a.container.Transaction(entry.Hash[:]).restoreState, entry)
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

		chain := loadState1(&err, false, a.ChainByName, entry.Name)
		if err != nil {
			break
		}
		saveState(&err, chain.Head().Put, ms)

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

func zero[T any]() (z T) { return z }

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

func loadState2[T, A1, A2 any](lastErr *error, allowMissing bool, get func(A1, A2) (T, error), a1 A1, a2 A2) T {
	if *lastErr != nil {
		return zero[T]()
	}

	v, err := get(a1, a2)
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
