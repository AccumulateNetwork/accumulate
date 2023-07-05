// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"io"
	"os"
	"strings"

	"github.com/dgraph-io/badger"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const collectIndexTxnPrefix = "txn."
const collectIndexRecordPrefix = "rec."

type CollectOptions struct {
	BuildIndex bool
	Predicate  func(database.Record) (bool, error)

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

	// Open a temporary badger DB for indexing hashes
	dir, err := os.MkdirTemp("", "accumulate-collect-snapshot-*.db")
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	defer func(dir string) {
		_ = os.RemoveAll(dir)
	}(dir)

	index, err := badger.Open(badger.DefaultOptions(dir))
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	defer func() {
		_ = index.Close()
	}()

	// Collect accounts
	err = db.collectAccounts(w, index, opts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Collect messages
	err = db.collectMessages(w, index, opts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Write the record index
	err = writeSnapshotIndex(w, index, opts)
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

func (db *Database) collectAccounts(w *snapshot.Writer, index *badger.DB, opts *CollectOptions) error {
	// Open a records section
	records, err := w.OpenRecords()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Iterate over the BPT and collect accounts
	batch := db.Begin(false)
	defer batch.Discard()
	it := batch.IterateAccounts()
	close, copts := collectOptions(index, opts)
	for it.Next() {
		account := it.Value()

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
		err = records.Collect(account, copts)
		if err != nil {
			return errors.UnknownError.WithFormat("collect %v: %w", account.Url(), err)
		}

		// Collect message hashes from all the message chains
		err = collectMessageHashes(account, index, opts)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	if it.Err() != nil {
		return errors.UnknownError.Wrap(it.Err())
	}

	err = close()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = records.Close()
	return errors.UnknownError.Wrap(err)
}

func (db *Database) collectMessages(w *snapshot.Writer, index *badger.DB, opts *CollectOptions) error {
	// Open a records section
	records, err := w.OpenRecords()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	indexTxn := index.NewTransaction(false)
	defer indexTxn.Discard()
	it := indexTxn.NewIterator(badger.IteratorOptions{
		Prefix: []byte(collectIndexTxnPrefix),
	})
	defer it.Close()

	batch := db.Begin(false)
	defer batch.Discard()

	close, copts := collectOptions(index, opts)

	for it.Rewind(); it.Valid(); it.Next() {
		hash := *(*[32]byte)(it.Item().Key()[len(collectIndexTxnPrefix):])
		if opts.Metrics != nil {
			opts.Metrics.Messages.Collecting++
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
		err = records.Collect(message, copts)
		if err != nil {
			return errors.UnknownError.WithFormat("collect %x: %w", hash, err)
		}

		// Collect the transaction status (which is the only part of the
		// transaction entity that is still used by exec v2)
		err = records.Collect(batch.newTransaction(transactionKey{Hash: hash}).newStatus(), copts)
		if err != nil {
			return errors.UnknownError.WithFormat("collect %x status: %w", hash, err)
		}
	}

	err = close()
	if err != nil {
		return errors.UnknownError.Wrap(err)
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
	for it.Next() {
		for _, entry := range it.Value() {
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

	Metrics *RestoreMetrics
}

type RestoreMetrics struct {
	Records struct {
		Restoring int
	}
}

// postRestorer is used by records that need to execute some logic after being
// restore.
type postRestorer interface {
	postRestore() error
}

func (db *Database) Restore(file ioutil.SectionReader, opts *RestoreOptions) error {
	if opts == nil {
		opts = new(RestoreOptions)
	}
	if opts.BatchRecordLimit == 0 {
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

			if opts.Metrics != nil {
				opts.Metrics.Records.Restoring = count
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

			if v, ok := v.(postRestorer); ok {
				err = v.postRestore()
				if err != nil {
					return errors.UnknownError.WithFormat("restore %v: %w", entry.Key, err)
				}
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
	it := batch.IterateAccounts()
	for it.Next() {
		account := it.Value()

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
	if it.Err() != nil {
		return errors.UnknownError.Wrap(it.Err())
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

func collectMessageHashes(a *Account, index *badger.DB, opts *CollectOptions) error {
	wb := index.NewWriteBatch()
	chains, err := a.Chains().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load chains index: %w", err)
	}
	for _, chain := range chains {
		if chain.Type != merkle.ChainTypeTransaction {
			continue
		}

		isSig := strings.EqualFold(chain.Name, "signature")
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
			err = wb.Set(append([]byte(collectIndexTxnPrefix), h...), []byte{})
			if err != nil {
				return errors.UnknownError.WithFormat("record %s chain entry: %w", c.Name(), err)
			}

			if !isSig {
				continue
			}

			msg, err := a.parent.newMessage(messageKey{*(*[32]byte)(h)}).Main().Get()
			if err != nil {
				return errors.UnknownError.WithFormat("load %s chain entry: %w", c.Name(), err)
			}
			if msg, ok := msg.(messaging.MessageForTransaction); ok {
				h := msg.GetTxID().Hash()
				err = wb.Set(append([]byte(collectIndexTxnPrefix), h[:]...), []byte{})
				if err != nil {
					return errors.UnknownError.WithFormat("record %s chain entry: %w", c.Name(), err)
				}
			}
		}
		if opts.Metrics != nil {
			opts.Metrics.Messages.Count += len(entries)
		}
	}

	err = wb.Flush()
	return errors.UnknownError.Wrap(err)
}

func writeSnapshotIndex(w *snapshot.Writer, index *badger.DB, opts *CollectOptions) error {
	if !opts.BuildIndex {
		return nil
	}

	x, err := w.OpenIndex()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	indexTxn := index.NewTransaction(false)
	defer indexTxn.Discard()
	it := indexTxn.NewIterator(badger.IteratorOptions{
		Prefix: []byte(collectIndexRecordPrefix),
	})
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		err := it.Item().Value(func(val []byte) error {
			e := new(recordIndexEntry)
			err := e.UnmarshalBinary(val)
			if err != nil {
				return errors.EncodingError.WithFormat("decode record index entry: %w", err)
			}
			err = x.Write(snapshot.RecordIndexEntry{
				Key:     e.Key.Hash(),
				Section: int(e.Section),
				Offset:  e.Offset,
			})
			return errors.UnknownError.Wrap(err)
		})
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	err = x.Close()
	return errors.UnknownError.Wrap(err)
}

func collectOptions(index *badger.DB, opts *CollectOptions) (func() error, snapshot.CollectOptions) {
	copts := snapshot.CollectOptions{
		Walk: database.WalkOptions{
			IgnoreIndices: true,
		},
		Predicate: opts.Predicate,
	}

	if !opts.BuildIndex {
		return func() error { return nil }, copts
	}

	wb := index.NewWriteBatch()
	copts.DidCollect = func(value database.Value, section, offset uint64) error {
		entry := &recordIndexEntry{
			Key:     value.Key(),
			Section: section,
			Offset:  offset,
		}
		b, err := entry.MarshalBinary()
		if err != nil {
			return errors.EncodingError.WithFormat("encode record index entry: %w", err)
		}
		h := value.Key().Hash()
		err = wb.Set(append([]byte(collectIndexRecordPrefix), h[:]...), b)
		if err != nil {
			return errors.InternalError.WithFormat("write record index entry: %w", err)
		}
		return nil
	}

	return wb.Flush, copts
}
