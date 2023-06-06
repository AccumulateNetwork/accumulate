// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CollectOptions struct {
	Predicate func(database.Record) (bool, error)

	Metrics *CollectMetrics
}

type CollectMetrics struct {
	Messages struct {
		Count      int
		Collecting int
	}
}

func (db *Database) Collect(file io.WriteSeeker, partition *url.URL, opts *CollectOptions) error {
	if opts == nil {
		opts = new(CollectOptions)
	}

	// Start the snapshot
	w, err := snapshot.Create(file)
	if err != nil {
		return errors.UnknownError.WithFormat("open snapshot: %w", err)
	}

	// Write the header
	err = db.writeSnapshotHeader(w, partition, opts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Collect the BPT
	err = db.collectBPT(w, opts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Collect accounts
	messages := new(hashSet)
	err = db.collectAccounts(w, messages, opts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Collect messages
	err = db.collectMessages(w, messages.Hashes, opts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Write the record index
	err = w.WriteIndex()
	if err != nil {
		return errors.UnknownError.WithFormat("write index: %w", err)
	}

	return nil
}

func (db *Database) writeSnapshotHeader(w *snapshot.Writer, partition *url.URL, opts *CollectOptions) error {
	header := new(snapshot.Header)

	// Load the BPT root hash
	batch := db.Begin(false)
	defer batch.Discard()
	var err error
	header.RootHash, err = batch.BPT().GetRootHash()
	if err != nil {
		return errors.UnknownError.WithFormat("get root hash: %w", err)
	}

	// Load the ledger
	if partition != nil {
		err = batch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&header.SystemLedger)
		if err != nil {
			return errors.UnknownError.WithFormat("load system ledger: %w", err)
		}
	}

	// Write the header
	err = w.WriteHeader(header)
	if err != nil {
		return errors.UnknownError.WithFormat("write header: %w", err)
	}

	return nil
}

func (db *Database) collectAccounts(w *snapshot.Writer, messages *hashSet, opts *CollectOptions) error {
	// Open a records section
	records, err := w.OpenRecords()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Iterate over the BPT and collect accounts
	batch := db.Begin(false)
	defer batch.Discard()
	it := batch.IterateAccounts()
	for {
		account, ok := it.Next()
		if !ok {
			break
		}

		// Check if the caller wants to skip this account
		if opts.Predicate != nil {
			ok, err := opts.Predicate(account)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			if !ok {
				continue
			}
		}

		// Collect the account's records
		err = records.Collect(account, snapshot.CollectOptions{
			Walk: database.WalkOptions{
				IgnoreIndices: true,
			},
			Predicate: opts.Predicate,
		})
		if err != nil {
			return errors.UnknownError.WithFormat("collect %v: %w", account.Url(), err)
		}

		// Collect message hashes from all the message chains
		err = collectMessageHashes(account, messages, opts)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	if it.Err() != nil {
		return errors.UnknownError.Wrap(it.Err())
	}

	err = records.Close()
	return errors.UnknownError.Wrap(err)
}

func (db *Database) collectMessages(w *snapshot.Writer, messages [][32]byte, opts *CollectOptions) error {
	// Open a records section
	records, err := w.OpenRecords()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	batch := db.Begin(false)
	defer batch.Discard()
	for i, hash := range messages {
		if opts.Metrics != nil {
			opts.Metrics.Messages.Collecting = i
		}

		// Check if the caller wants to skip this message
		message := batch.newMessage(messageKey{Hash: hash})
		if opts.Predicate != nil {
			ok, err := opts.Predicate(message)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			if !ok {
				continue
			}
		}

		// Collect the message's records
		err = records.Collect(message, snapshot.CollectOptions{
			Walk: database.WalkOptions{
				IgnoreIndices: true,
			},
			Predicate: opts.Predicate,
		})
		if err != nil {
			return errors.UnknownError.WithFormat("collect %x: %w", hash, err)
		}

		// Collect the transaction status (which is the only part of the
		// transaction entity that is still used by exec v2)
		err = records.Collect(batch.newTransaction(transactionKey{Hash: hash}).newStatus(), snapshot.CollectOptions{
			Walk: database.WalkOptions{
				IgnoreIndices: true,
			},
			Predicate: opts.Predicate,
		})
		if err != nil {
			return errors.UnknownError.WithFormat("collect %x status: %w", hash, err)
		}
	}

	err = records.Close()
	return errors.UnknownError.Wrap(err)
}

func (db *Database) collectBPT(w *snapshot.Writer, opts *CollectOptions) error {
	batch := db.Begin(false)
	defer batch.Discard()

	// Check if the caller wants to skip the BPT
	if opts.Predicate != nil {
		ok, err := opts.Predicate(batch.BPT())
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if !ok {
			return nil
		}
	}

	wr, err := w.OpenRaw(snapshot.SectionTypeBPT)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Iterate over the BPT and collect hashes
	it := batch.BPT().Iterate(1000)
	for {
		entries, ok := it.Next()
		if !ok {
			break
		}

		for _, entry := range entries {
			_, err = wr.Write(entry.Key[:])
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			_, err = wr.Write(entry.Value[:])
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}
	}
	if it.Err() != nil {
		return errors.UnknownError.Wrap(it.Err())
	}

	err = wr.Close()
	return errors.UnknownError.Wrap(err)
}

type RestoreOptions struct {
	BatchRecordLimit int
	SkipHashCheck    bool
	Predicate        func(*snapshot.RecordEntry, database.Value) (bool, error)
}

func (db *Database) Restore(file ioutil.SectionReader, opts *RestoreOptions) error {
	if opts == nil {
		opts = new(RestoreOptions)
		opts.BatchRecordLimit = 50_000
	}

	rd, err := snapshot.Open(file)
	if err != nil {
		return errors.UnknownError.WithFormat("open snapshot: %w", err)
	}

	hashes, err := readBptSnapshot(rd, opts)
	if err != nil {
		return errors.UnknownError.WithFormat("load hashes: %w", err)
	}

	batch := db.Begin(true)

	// Wrap this in a function so it uses the latest value of the variable
	defer func() { batch.Discard() }()

	var count int
	for i, s := range rd.Sections {
		if s.Type() != snapshot.SectionTypeRecords {
			continue
		}

		rd, err := rd.OpenRecords(i)
		if err != nil {
			return errors.UnknownError.WithFormat("open record section (%d): %w", i, err)
		}

		for {
			if opts.BatchRecordLimit != 0 && count > 0 && count%opts.BatchRecordLimit == 0 {
				err = batch.Commit()
				if err != nil {
					return errors.UnknownError.WithFormat("commit changes: %w", err)
				}
				batch = db.Begin(true)
			}
			count++

			entry, err := rd.Read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return errors.UnknownError.WithFormat("read next record: %w", err)
			}

			v, err := values.Resolve[database.Value](batch, entry.Key)
			if err != nil {
				return errors.UnknownError.WithFormat("resolve %v: %w", entry.Key, err)
			}

			if opts.Predicate != nil {
				ok, err := opts.Predicate(entry, v)
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
				if !ok {
					continue
				}
			}

			err = v.LoadBytes(entry.Value, true)
			if err != nil {
				return errors.UnknownError.WithFormat("restore %v: %w", entry.Key, err)
			}
		}
	}

	err = batch.Commit()
	if err != nil {
		return errors.UnknownError.WithFormat("commit changes: %w", err)
	}

	if opts.SkipHashCheck {
		return nil
	}

	// We can't check an account's hash until its records are written
	batch = db.Begin(false)
	for it := batch.IterateAccounts(); ; {
		account, ok := it.Next()
		if !ok {
			if it.Err() != nil {
				return errors.UnknownError.Wrap(it.Err())
			}
			break
		}

		hash, err := account.Hash()
		if err != nil {
			return errors.UnknownError.WithFormat("calculate account hash: %w", err)
		}
		kh := account.Key().Hash()
		if hashes[kh] != hash {
			return errors.InvalidRecord.WithFormat("account %v hash does not match", account.Url())
		}
		delete(hashes, kh)
	}

	for kh := range hashes {
		u, err := batch.getAccountUrl(record.NewKey(storage.Key(kh)))
		switch {
		case err == nil:
			return errors.InvalidRecord.WithFormat("missing BPT entry for %v", u)
		case errors.Is(err, errors.NotFound):
			return errors.InvalidRecord.WithFormat("missing BPT entry for %x", kh)
		default:
			return errors.UnknownError.Wrap(err)
		}
	}

	// We can't revert the changes at this point (depending on the batch record
	// limit) so we might as well do this after committing
	rh, err := batch.BPT().GetRootHash()
	if err != nil {
		return errors.UnknownError.WithFormat("get root hash: %w", err)
	}
	if rd.Header.RootHash != rh {
		return errors.InvalidRecord.WithFormat("root hash does not match")
	}
	return nil
}

func readBptSnapshot(snap *snapshot.Reader, opts *RestoreOptions) (map[[32]byte][32]byte, error) {
	if opts.SkipHashCheck {
		return nil, nil
	}

	hashes := map[[32]byte][32]byte{}
	var bpt *ioutil.Segment[snapshot.SectionType, *snapshot.SectionType]
	for _, s := range snap.Sections {
		if s.Type() == snapshot.SectionTypeBPT {
			bpt = s
			break
		}
	}
	if bpt == nil {
		return nil, errors.InvalidRecord.WithFormat("missing BPT section")
	}
	if bpt.Size()%64 != 0 {
		return nil, errors.InvalidRecord.WithFormat("invalid BPT section: want size to be a multiple of %d, got %d", 64, bpt.Size())
	}
	rd, err := bpt.Open()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	for {
		var b [64]byte
		_, err := io.ReadFull(rd, b[:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				return hashes, nil
			}
			return nil, err
		}

		hashes[*(*[32]byte)(b[:32])] = *(*[32]byte)(b[32:])
	}
}

type hashSet struct {
	seen   map[[32]byte]bool
	Hashes [][32]byte
}

func (s *hashSet) Has(h [32]byte) bool {
	return s.seen[h]
}

func (s *hashSet) Add(h [32]byte) {
	if s.seen == nil {
		s.seen = map[[32]byte]bool{}
	}
	if s.seen[h] {
		return
	}
	s.seen[h] = true
	s.Hashes = append(s.Hashes, h)
}

func collectMessageHashes(a *Account, hashes *hashSet, opts *CollectOptions) error {
	chains, err := a.Chains().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load chains index: %w", err)
	}
	for _, chain := range chains {
		if chain.Type != merkle.ChainTypeTransaction {
			continue
		}

		c, err := a.ChainByName(chain.Name)
		if err != nil {
			return errors.InvalidRecord.Wrap(err)
		}

		// Check if the caller wants to skip this chain
		if opts.Predicate != nil {
			ok, err := opts.Predicate(c)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			if !ok {
				continue
			}
			ok, err = opts.Predicate(c.Inner())
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			if !ok {
				continue
			}
		}

		head, err := c.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load %s chain head: %w", c.Name(), err)
		}
		if head.Count == 0 {
			return nil
		}

		entries, err := c.Inner().GetRange(0, head.Count)
		if err != nil {
			return errors.UnknownError.WithFormat("load %s chain entries: %w", c.Name(), err)
		}
		for _, h := range entries {
			hashes.Add(*(*[32]byte)(h))
		}
		if opts.Metrics != nil {
			opts.Metrics.Messages.Count = len(hashes.Hashes)
		}
	}
	return nil
}
