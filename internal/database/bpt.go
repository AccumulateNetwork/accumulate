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
	/*synthetic := object("Account", network.Synthetic())
	partition := network.NodeUrl()*/

	// Write the header
	var ledger *protocol.SystemLedger
	err := b.Account(network.Ledger()).GetStateAs(&ledger)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load ledger: %w", err)
	}

	bpt := pmt.NewBPTManager(b.kvstore)
	header := new(snapshotHeader)
	header.Version = core.SnapshotVersion1
	header.Height = ledger.Index
	header.RootHash = *(*[32]byte)(b.BptRoot())

	_, err = header.WriteTo(file)
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

		txns, err := account.MainChain().stateOfTransactionsOnChain()
		if err != nil {
			return nil, err
		}

		state.Transactions = append(state.Transactions, txns...)

		return state.MarshalBinary()
	})
}

// ReadSnapshot reads a snapshot file, returning the header values and a reader.
func ReadSnapshot(file ioutil2.SectionReader) (*snapshotHeader, int64, error) {
	header := new(snapshotHeader)
	n, err := header.ReadFrom(file)
	if err != nil {
		return nil, 0, errors.Format(errors.StatusUnknownError, "read header: %w", err)
	}

	return header, n, nil
}

// RestoreSnapshot loads the full state of the partition from a file.
func (b *Batch) RestoreSnapshot(file ioutil2.SectionReader) error {
	// Read the snapshot
	_, _, err := ReadSnapshot(file)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Make a new section reader starting after the header
	rd, err := ioutil2.NewSectionReader(file, -1, -1)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Load the snapshot
	bpt := pmt.NewBPTManager(b.kvstore)
	return bpt.Bpt.LoadSnapshot(rd, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
		state := new(accountState)
		err := state.UnmarshalBinaryFrom(reader)
		if err != nil {
			return err
		}

		account := b.Account(state.Main.GetUrl())
		if account.key.Hash() != key {
			return errors.Format(errors.StatusInternalError, "hash key %x does not match URL %v", key, state.Main.GetUrl())
		}

		err = account.restoreState(state)
		if err != nil {
			return err
		}

		// Check the hash
		hasher, err := account.hashState()
		if err != nil {
			return err
		}

		if !bytes.Equal(hash[:], hasher.MerkleHash()) {
			return fmt.Errorf("hash does not match for %v", state.Main.GetUrl())
		}

		return nil
	})
}
