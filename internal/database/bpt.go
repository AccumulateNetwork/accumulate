package database

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// putBpt adds an entry to the list of pending BPT updates.
func (b *Batch) putBpt(key storage.Key, hash [32]byte) {
	if b.done {
		panic("attempted to use a commited or discarded batch")
	}
	if b.bptEntries == nil {
		panic("attempted to update the BPT after committing the BPT")
	}

	b.bptEntries[key] = hash
}

// commitBpt commits pending BPT updates.
func (b *Batch) commitBpt() error {
	bpt := pmt.NewBPTManager(b.kvstore)

	for k, v := range b.bptEntries {
		bpt.InsertKV(k, v)
	}

	err := bpt.Bpt.Update()
	if err != nil {
		return err
	}

	b.bptEntries = nil
	return nil
}

// BptRoot returns the root of the BPT. BptRoot panics if there are any
// uncommitted BPT changes.
func (b *Batch) BptRoot() []byte {
	if len(b.bptEntries) > 0 {
		panic("attempted to get BPT root with uncommitted changes")
	}
	bpt := pmt.NewBPTManager(b.kvstore)
	return bpt.Bpt.RootHash[:]
}

// BptReceipt builds a BPT receipt for the given key.
func (b *Batch) BptReceipt(key storage.Key, value [32]byte) (*managed.Receipt, error) {
	if len(b.bptEntries) > 0 {
		return nil, errors.New(errors.StatusInternalError, "cannot generate a BPT receipt when there are uncommitted BPT entries")
	}

	bpt := pmt.NewBPTManager(b.kvstore)
	receipt := bpt.Bpt.GetReceipt(key)
	if receipt == nil {
		return nil, errors.NotFound("BPT key %v not found", key)
	}

	return receipt, nil
}

func (h *snapshotHeader) WriteTo(wr io.Writer) (int64, error) {
	b, err := h.MarshalBinary()
	if err != nil {
		return 0, errors.Format(errors.StatusEncodingError, "marshal: %w", err)
	}

	var v [8]byte
	binary.BigEndian.PutUint64(v[:], uint64(len(b)))
	n, err := wr.Write(v[:])
	if err != nil {
		return int64(n), errors.Format(errors.StatusEncodingError, "write length: %w", err)
	}

	m, err := wr.Write(b)
	if err != nil {
		return int64(n + m), errors.Format(errors.StatusEncodingError, "write data: %w", err)
	}

	return int64(n + m), nil
}

func (h *snapshotHeader) ReadFrom(rd io.Reader) (int64, error) {
	var v [8]byte
	n, err := io.ReadFull(rd, v[:])
	if err != nil {
		return int64(n), errors.Format(errors.StatusEncodingError, "read length: %w", err)
	}

	l := binary.BigEndian.Uint64(v[:])
	b := make([]byte, l)
	m, err := io.ReadFull(rd, b)
	if err != nil {
		return int64(n + m), errors.Format(errors.StatusEncodingError, "read data: %w", err)
	}

	err = h.UnmarshalBinary(b)
	if err != nil {
		return int64(n + m), errors.Format(errors.StatusEncodingError, "unmarshal: %w", err)
	}

	return int64(n + m), nil
}

// SaveSnapshot writes the full state of the partition out to a file.
func (b *Batch) SaveSnapshot(file io.WriteSeeker, network *config.Describe) error {
	var ledger *protocol.SystemLedger
	err := b.Account(network.Ledger()).GetStateAs(&ledger)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load ledger: %w", err)
	}

	// This must match how the model constructs the key
	anchorLedgerKey := storage.MakeKey("Account", network.AnchorPool())

	return b.saveSnapshot(file, ledger.Index, func(key [32]byte, state *accountState, account *Account) error {
		// Load transactions for system chains
		if key != anchorLedgerKey {
			return nil
		}

		var err error
		state.Transactions = append(state.Transactions, loadState(&err, false, account.AnchorSequenceChain().stateOfTransactionsOnChain)...)
		return err
	})
}

// SaveFactomSnapshot is a special version of SaveSnapshot for creating a
// pre-genesis snapshot for Factom data entries.
func (b *Batch) SaveFactomSnapshot(file io.WriteSeeker) error {
	return b.saveSnapshot(file, 0, func([32]byte, *accountState, *Account) error { return nil })
}

// SaveSnapshot writes the full state of the partition out to a file.
func (b *Batch) saveSnapshot(file io.WriteSeeker, height uint64, extra func(key [32]byte, state *accountState, account *Account) error) error {
	// Write the header
	bpt := pmt.NewBPTManager(b.kvstore)
	header := new(snapshotHeader)
	header.Version = core.SnapshotVersion1
	header.Height = height
	header.RootHash = *(*[32]byte)(b.BptRoot())

	_, err := header.WriteTo(file)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "write header: %w", err)
	}

	// Create a section writer starting after the header
	wr, err := ioutil2.NewSectionWriter(file, -1, -1)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "create section writer: %w", err)
	}

	// Save the snapshot
	return bpt.Bpt.SaveSnapshot(wr, func(key storage.Key, hash [32]byte) ([]byte, error) {
		// Create an Account object
		u, err := b.getAccountUrl(record.Key{key})
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
		account := b.Account(u)

		// Check the hash
		hasher, err := account.hashState()
		if err != nil {
			return nil, err
		}

		if !bytes.Equal(hash[:], hasher.MerkleHash()) {
			return nil, fmt.Errorf("hash does not match for %v", u)
		}

		/*// Load the full state - preserve chains if the account is a partition account
		state, err := account.state(true, partition.PrefixOf(a.GetUrl()))*/

		// Load the full state - always preserve chains for now
		state, err := account.loadState(true)
		if err != nil {
			return nil, err
		}

		/*if objectBucket(key) != synthetic {
			return state.MarshalBinary()
		}*/

		// Load transactions and signatures
		state.Transactions = append(state.Transactions, loadState(&err, false, account.MainChain().stateOfTransactionsOnChain)...)
		state.Transactions = append(state.Transactions, loadState(&err, false, account.ScratchChain().stateOfTransactionsOnChain)...)
		state.Signatures = append(state.Signatures, loadState(&err, false, account.SignatureChain().stateOfSignaturesOnChain)...)

		// Load transactions for system chains
		err = extra(key, state, account)
		if err != nil {
			return nil, err
		}

		b := loadState(&err, false, state.MarshalBinary)
		return b, err
	})
}

// ReadSnapshotHeader reads a snapshot file, returning the header values and a reader.
func ReadSnapshotHeader(file ioutil2.SectionReader) (*snapshotHeader, int64, error) {
	header := new(snapshotHeader)
	n, err := header.ReadFrom(file)
	if err != nil {
		return nil, 0, errors.Format(errors.StatusUnknownError, "read header: %w", err)
	}

	return header, n, nil
}

func readSnapshot(file ioutil2.SectionReader, process func(key storage.Key, hash [32]byte, state *accountState) error) error {
	// Read the snapshot header
	_, _, err := ReadSnapshotHeader(file)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Make a new section reader starting after the header
	rd, err := ioutil2.NewSectionReader(file, -1, -1)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Load the snapshot
	err = pmt.ReadSnapshot(rd, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
		state := new(accountState)
		err := state.UnmarshalBinaryFrom(reader)
		if err != nil {
			return err
		}

		return process(key, hash, state)
	})
	return errors.Wrap(errors.StatusUnknownError, err)
}

// RestoreSnapshot loads the full state of the partition from a file.
func RestoreSnapshot(db Beginner, file ioutil2.SectionReader, network *config.Describe) error {
	err := readSnapshot(file, func(key storage.Key, hash [32]byte, state *accountState) error {
		// Restore each account with a separate transaction
		batch := db.Begin(true)
		defer batch.Discard()

		account := batch.Account(state.Main.GetUrl())
		err := account.safeRestoreState(key, hash, state)
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}

		err = batch.Commit()
		return errors.Wrap(errors.StatusUnknownError, err)
	})
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	batch := db.Begin(true)
	defer batch.Discard()

	// Rebuild the synthetic transaction index index
	record := batch.Account(network.Synthetic())
	synthIndexChain, err := record.MainChain().Index().Get()
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load synthetic index chain: %w", err)
	}

	entries, err := synthIndexChain.Entries(0, synthIndexChain.Height())
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load synthetic index chain entries: %w", err)
	}

	for i, data := range entries {
		entry := new(protocol.IndexEntry)
		err = entry.UnmarshalBinary(data)
		if err != nil {
			return errors.Format(errors.StatusInternalError, "unmarshal synthetic index chain entry %d: %w", i, err)
		}

		err = batch.SystemData(network.PartitionId).SyntheticIndexIndex(entry.BlockIndex).Put(uint64(i))
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "store synthetic transaction index index %d for block: %w", i, err)
		}
	}

	err = batch.Commit()
	return errors.Wrap(errors.StatusUnknownError, err)
}

func (b *Batch) ImportFactomSnapshot(file ioutil2.SectionReader, include func(account protocol.Account) (bool, error)) error {
	bpt := pmt.NewBPTManager(b.kvstore)

	err := readSnapshot(file, func(key storage.Key, hash [32]byte, state *accountState) error {
		ok, err := include(state.Main)
		if !ok || err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}

		bpt.Bpt.Insert(key, hash)
		account := b.Account(state.Main.GetUrl())
		err = account.safeRestoreState(key, hash, state)
		return errors.Wrap(errors.StatusUnknownError, err)
	})
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = bpt.Bpt.Update()
	return errors.Wrap(errors.StatusUnknownError, err)
}
