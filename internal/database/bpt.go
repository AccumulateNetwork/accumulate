package database

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
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

	return b.saveSnapshot(file, ledger.Index, func(key [32]byte, state *accountState, account *Account, transactions, signatures map[[32]byte]bool) error {
		// Load transactions for system chains
		if key != anchorLedgerKey {
			return nil
		}

		var err error
		for _, h := range loadState(&err, false, account.AnchorSequenceChain().allEntries) {
			transactions[h.Bytes32()] = true
		}
		return err
	})
}

// SaveFactomSnapshot is a special version of SaveSnapshot for creating a
// pre-genesis snapshot for Factom data entries.
func (b *Batch) SaveFactomSnapshot(file io.WriteSeeker) error {
	return b.saveSnapshot(file, 0, func(key [32]byte, state *accountState, account *Account, transactions, signatures map[[32]byte]bool) error {
		return nil
	})
}

// SaveSnapshot writes the full state of the partition out to a file.
func (b *Batch) saveSnapshot(file io.WriteSeeker, height uint64, extra func(key [32]byte, state *accountState, account *Account, transactions, signatures map[[32]byte]bool) error) error {
	wr := snapshot.NewWriter(file)

	// Write the header
	sw, err := wr.Open(snapshot.SectionTypeHeader)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "open header section: %w", err)
	}

	header := new(snapshotHeader)
	header.Version = core.SnapshotVersion1
	header.Height = height
	header.RootHash = *(*[32]byte)(b.BptRoot())
	_, err = header.WriteTo(sw)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "write header section: %w", err)
	}
	err = sw.Close()
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "close header section: %w", err)
	}

	// Write accounts
	sw, err = wr.Open(snapshot.SectionTypeAccounts)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "open accounts section: %w", err)
	}

	// Save the snapshot
	txnMap := map[[32]byte]bool{}
	sigMap := map[[32]byte]bool{}
	bpt := pmt.NewBPTManager(b.kvstore)
	err = bpt.Bpt.SaveSnapshot(sw, func(key storage.Key, hash [32]byte) ([]byte, error) {
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

		// Queue transactions and signatures
		// TODO: We need to be more selective than this
		for _, txid := range state.Pending {
			txnMap[txid.Hash()] = true
		}
		for _, h := range loadState(&err, false, account.MainChain().allEntries) {
			txnMap[h.Copy().Bytes32()] = true
		}
		for _, h := range loadState(&err, false, account.ScratchChain().allEntries) {
			txnMap[h.Copy().Bytes32()] = true
		}
		for _, h := range loadState(&err, false, account.SignatureChain().allEntries) {
			sigMap[h.Copy().Bytes32()] = true
		}

		// Load transactions for system chains
		err = extra(key, state, account, txnMap, sigMap)
		if err != nil {
			return nil, err
		}

		b := loadState(&err, false, state.MarshalBinary)
		return b, err
	})
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = sw.Close()
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "close accounts section: %w", err)
	}

	// Write transactions (in order)
	if len(txnMap) > 0 {
		sw, err := wr.Open(snapshot.SectionTypeTransactions)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "open transactions section: %w", err)
		}
		txns := make([]*transactionState, 0, len(txnMap))
		for txid, ok := range txnMap {
			if !ok {
				continue
			}
			s, err := b.Transaction(txid[:]).loadState() //nolint:rangevarref
			if err != nil {
				return errors.Wrap(errors.StatusUnknownError, err)
			}
			s.hash = txid
			txns = append(txns, s)
		}
		sort.Slice(txns, func(i, j int) bool { return bytes.Compare(txns[i].hash[:], txns[j].hash[:]) <= 0 })
		// TODO: Do this in a way that allows for random-access
		data, err := (&transactionSection{Transactions: txns}).MarshalBinary()
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "marshal transactions section: %w", err)
		}
		_, err = sw.Write(data)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "write transactions section: %w", err)
		}

		err = sw.Close()
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "close transactions section: %w", err)
		}
	}

	// Write signatures (in order)
	if len(sigMap) > 0 {
		sw, err := wr.Open(snapshot.SectionTypeSignatures)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "open signatures section: %w", err)
		}
		sigList := make([][32]byte, 0, len(sigMap))
		for h, ok := range sigMap {
			if !ok {
				continue
			}
			sigList = append(sigList, h)
		}
		sort.Slice(sigList, func(i, j int) bool { return bytes.Compare(sigList[i][:], sigList[j][:]) <= 0 })
		sigs := make([]protocol.Signature, len(sigList))
		for i, h := range sigList {
			s, err := b.Transaction(h[:]).Main().Get() //nolint:rangevarref
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "load signature: %w", err)
			}
			if s.Signature == nil {
				return errors.Format(errors.StatusInternalError, "signature %x is not a signature", h)
			}
			sigs[i] = s.Signature
		}
		// TODO: Do this in a way that allows for random-access
		data, err := (&signatureSection{Signatures: sigs}).MarshalBinary()
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "marshal signatures section: %w", err)
		}
		_, err = sw.Write(data)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "write signatures section: %w", err)
		}

		err = sw.Close()
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "close signatures section: %w", err)
		}
	}

	return nil
}

// OpenSnapshot reads a snapshot file, returning the header values and a reader.
func OpenSnapshot(file ioutil2.SectionReader) (*snapshotHeader, *snapshot.Reader, error) {
	r := snapshot.NewReader(file)
	s, err := r.Next()
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	if s.Type() != snapshot.SectionTypeHeader {
		return nil, nil, errors.Format(errors.StatusBadRequest, "bad first section: expected %v, got %v", snapshot.SectionTypeHeader, s.Type())
	}

	sr, err := s.Open()
	if err != nil {
		return nil, nil, errors.Format(errors.StatusUnknownError, "open header section: %w", err)
	}

	header := new(snapshotHeader)
	_, err = header.ReadFrom(sr)
	if err != nil {
		return nil, nil, errors.Format(errors.StatusUnknownError, "read header: %w", err)
	}

	return header, r, nil
}

func readSnapshot(file ioutil2.SectionReader, process func(typ snapshot.SectionType, rd ioutil2.SectionReader) error) error {
	// Read the snapshot header
	h, r, err := OpenSnapshot(file)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}
	if h.Version != 1 {
		return errors.Format(errors.StatusBadRequest, "expected version 1, got %d", h.Version)
	}

	for {
		s, err := r.Next()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, io.EOF):
			return nil
		default:
			return errors.Format(errors.StatusUnknownError, "read next section header: %w", err)
		}

		sr, err := s.Open()
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "open section: %w", err)
		}

		err = process(s.Type(), sr)
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}
}

// RestoreSnapshot loads the full state of the partition from a file.
func RestoreSnapshot(db Beginner, file ioutil2.SectionReader, network *config.Describe) error {
	err := readSnapshot(file, func(typ snapshot.SectionType, rd ioutil2.SectionReader) error {
		switch typ {
		case snapshot.SectionTypeAccounts:
			err := pmt.ReadSnapshot(rd, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
				state := new(accountState)
				err := state.UnmarshalBinaryFrom(reader)
				if err != nil {
					return err
				}

				// Restore each account with a separate batch
				batch := db.Begin(true)
				defer batch.Discard()

				account := batch.Account(state.Main.GetUrl())
				err = account.safeRestoreState(key, hash, state)
				if err != nil {
					return errors.Wrap(errors.StatusUnknownError, err)
				}

				err = batch.Commit()
				return errors.Wrap(errors.StatusUnknownError, err)
			})
			return errors.Wrap(errors.StatusUnknownError, err)

		case snapshot.SectionTypeTransactions:
			s := new(transactionSection)
			err := s.UnmarshalBinaryFrom(rd)
			if err != nil {
				return errors.Format(errors.StatusEncodingError, "unmarshal transaction section: %w", err)
			}

			batch := db.Begin(true)
			defer batch.Discard()

			start := time.Now()
			for i, s := range s.Transactions {
				if i > 0 && i%10000 == 0 {
					d := time.Since(start)
					batch.logger.Info("Restored transactions", "module", "restore", "count", i, "duration", d, "per-second", float64(i)/d.Seconds())

					// Create a new batch
					err = batch.Commit()
					if err != nil {
						return errors.Wrap(errors.StatusUnknownError, err)
					}
					batch = db.Begin(true)
					defer batch.Discard()
				}

				hash := s.Transaction.GetHash()
				err = batch.Transaction(hash).restoreState(s)
				if err != nil {
					return fmt.Errorf("transaction %X: %w", hash[:4], err)
				}
			}

			d := time.Since(start)
			batch.logger.Info("Restored transactions", "module", "restore", "count", len(s.Transactions), "duration", d, "per-second", float64(len(s.Transactions))/d.Seconds())
			err = batch.Commit()
			return errors.Wrap(errors.StatusUnknownError, err)

		case snapshot.SectionTypeSignatures:
			s := new(signatureSection)
			err := s.UnmarshalBinaryFrom(rd)
			if err != nil {
				return errors.Format(errors.StatusEncodingError, "unmarshal signature section: %w", err)
			}
			batch := db.Begin(true)
			defer batch.Discard()

			start := time.Now()
			for i, s := range s.Signatures {
				if i > 0 && i%10000 == 0 {
					d := time.Since(start)
					batch.logger.Info("Restored signatures", "module", "restore", "count", i, "duration", d, "per-second", float64(i)/d.Seconds())

					// Create a new batch
					err = batch.Commit()
					if err != nil {
						return errors.Wrap(errors.StatusUnknownError, err)
					}
					batch = db.Begin(true)
					defer batch.Discard()
				}

				err = batch.Transaction(s.Hash()).Main().Put(&SigOrTxn{Signature: s})
				if err != nil {
					return errors.Format(errors.StatusEncodingError, "store signature: %w", err)
				}
			}

			d := time.Since(start)
			batch.logger.Info("Restored signatures", "module", "restore", "count", len(s.Signatures), "duration", d, "per-second", float64(len(s.Signatures))/d.Seconds())
			err = batch.Commit()
			return errors.Wrap(errors.StatusUnknownError, err)

		default:
			return errors.Format(errors.StatusBadRequest, "unknown snapshot section type %v", typ)
		}
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

func (b *Batch) ImportFactomSnapshot(file ioutil2.SectionReader, includeAccount func(account protocol.Account) (bool, error), includeTransaction func(transaction *protocol.Transaction) (bool, error)) error {
	err := readSnapshot(file, func(typ snapshot.SectionType, rd ioutil2.SectionReader) error {
		switch typ {
		case snapshot.SectionTypeAccounts:
			err := pmt.ReadSnapshot(rd, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
				state := new(accountState)
				err := state.UnmarshalBinaryFrom(reader)
				if err != nil {
					return err
				}

				ok, err := includeAccount(state.Main)
				if !ok || err != nil {
					return errors.Wrap(errors.StatusUnknownError, err)
				}

				account := b.Account(state.Main.GetUrl())
				err = account.safeRestoreState(key, hash, state)
				return errors.Wrap(errors.StatusUnknownError, err)
			})
			return errors.Wrap(errors.StatusUnknownError, err)

		case snapshot.SectionTypeTransactions:
			s := new(transactionSection)
			err := s.UnmarshalBinaryFrom(rd)
			if err != nil {
				return errors.Format(errors.StatusEncodingError, "unmarshal transaction section: %w", err)
			}

			for _, s := range s.Transactions {
				ok, err := includeTransaction(s.Transaction)
				if err != nil {
					return errors.Wrap(errors.StatusUnknownError, err)
				}
				if !ok {
					continue
				}

				hash := s.Transaction.GetHash()
				err = b.Transaction(hash).restoreState(s)
				if err != nil {
					return errors.Format(errors.StatusUnknownError, "transaction %X: %w", hash[:4], err)
				}
			}
			return nil

		case snapshot.SectionTypeSignatures:
			return errors.Format(errors.StatusBadRequest, "unexpected signatures section")

		default:
			return errors.Format(errors.StatusBadRequest, "unknown snapshot section type %v", typ)
		}
	})
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return errors.Wrap(errors.StatusUnknownError, err)
}
