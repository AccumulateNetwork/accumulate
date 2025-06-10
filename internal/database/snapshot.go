// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

//AI: This file implements the logic for collecting and restoring database
//AI: snapshots in Accumulate. It defines options and metrics for snapshot
//AI: operations, and provides functions for collecting all account and message
//AI: data into a portable, restorable file. Special attention is given to
//AI: batching, memory usage, and the creation of fast-lookup indices.

import (
	"bytes"
	"encoding/binary"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/util/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// AI: CollectOptions defines configuration for snapshot collection, including
// AI: which records to include, whether to build a fast-lookup index, and hooks
// AI: for filtering or metrics.
type CollectOptions struct {
	BuildIndex     bool
	Predicate      func(database.Record) (bool, error)
	DidWriteHeader func(*snapshot.Writer) error
	KeepMessage    func(messaging.Message) (bool, error)

	// When processing a message chain, don't try to follow a message's links to
	// other messages (e.g. a signature's link to a transaction).
	SkipMessageRefs bool

	Metrics *CollectMetrics
}

// AI: CollectMetrics tracks metrics about the snapshot collection process, such
// AI: as the number of messages processed.
type CollectMetrics struct {
	Messages struct {
		Count      int
		Collecting int
	}
}

// Collect collects a snapshot of the database.
//
// Collect is a wrapper around [Batch.Collect].
// AI: Database.Collect is a high-level entry point for snapshot collection. It
// AI: creates a read-only batch and delegates to Batch.Collect, ensuring the
// AI: batch is always discarded to avoid memory leaks.
func (db *Database) Collect(file io.WriteSeeker, partition *url.URL, opts *CollectOptions) (*snapshot.Writer, error) {
	batch := db.Begin(false)
	defer batch.Discard()
	return batch.Collect(file, partition, opts)
}

// AI: Batch.Collect performs the core snapshot logic. WARNING: If this batch is
// AI: nested (not a root batch), memory usage can spike until the root batch is
// AI: discarded. Always prefer root batches for large snapshots.
// Collect collects a snapshot of the database.
//
// WARNING: If the batch is nested (if it is not a root batch), Collect may
// cause excessive memory consumption until the root batch is discarded.
func (batch *Batch) Collect(file io.WriteSeeker, partition *url.URL, opts *CollectOptions) (*snapshot.Writer, error) {
	//AI: Ensure options are always non-nil for downstream logic.
	if opts == nil {
		opts = new(CollectOptions)
	}

	// Start the snapshot
	//AI: Create the snapshot writer, which manages the output file format.
	w, err := snapshot.Create(file)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open snapshot: %w", err)
	}

	// Write the header
	//AI: Write the snapshot file header, including the BPT root hash and partition
	//AI: ledger.
	err = batch.writeSnapshotHeader(w, partition, opts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	//AI: Optional hook for custom logic after writing the header but before
	//AI: collecting records.
	if opts.DidWriteHeader != nil {
		err = opts.DidWriteHeader(w)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Collect the BPT
	//AI: Collect and write the BPT (Binary Patricia Tree), which is the
	//AI: database's root hash structure.
	err = batch.collectBPT(w, opts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	//AI: Create a temporary directory for storing intermediate index/hash files
	//AI: during snapshot construction. This is cleaned up at the end.
	dir, err := os.MkdirTemp("", "accumulate-snapshot-*")
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	defer func() {
		err := os.RemoveAll(dir)
		if err != nil {
			slog.Error("Failed to remove temp directory", "dir", dir, "error", err)
		}
	}()

	//AI: Open a hash bucket for tracking the hashes of all records/messages
	//AI: collected.
	hashes, err := indexing.OpenBucket(filepath.Join(dir, "hash"), 0, true)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	defer func() { _ = hashes.Close() }()

	var index *indexing.Bucket
	//AI: If BuildIndex is true, create an index bucket to build a fast-lookup
	//AI: index for the snapshot. This enables random access to records in the
	//AI: snapshot file.
	if opts.BuildIndex {
		index, err = indexing.OpenBucket(filepath.Join(dir, "index"), indexDataSize, true)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		defer func() { _ = index.Close() }()
	}

	// Collect accounts
	//AI: Collect and write all account records to the snapshot file, optionally
	//AI: recording their locations in the index.
	err = batch.collectAccounts(w, index, hashes, opts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Collect messages
	//AI: Collect and write all message records (transactions, signatures, etc.)
	//AI: to the snapshot file, optionally recording their locations in the index.
	err = batch.collectMessages(w, index, hashes, opts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Write the record index
	//AI: Write the constructed index (if any) to the snapshot file. This allows
	//AI: fast lookups during restore or analysis.
	err = writeSnapshotIndex(w, index, opts)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("write index: %w", err)
	}

	return w, nil
}

// AI: writeSnapshotHeader writes metadata to the snapshot file, including the
// AI: BPT root hash and partition ledger if available.
func (batch *Batch) writeSnapshotHeader(w *snapshot.Writer, partition *url.URL, _ *CollectOptions) error {
	header := new(snapshot.Header)

	// Load the BPT root hash
	var err error
	header.RootHash, err = batch.GetBptRootHash()
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

// AI: collectAccounts iterates over all accounts in the database, applies any
// AI: filtering, and writes their records to the snapshot, updating the index
// AI: if enabled.
func (batch *Batch) collectAccounts(w *snapshot.Writer, index, hashes *indexing.Bucket, opts *CollectOptions) error {
	// Open a records section
	records, err := w.OpenRecords()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Iterate over the BPT and collect accounts
	it := batch.IterateAccounts()
	copts := collectOptions(index, opts)
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
		err = collectMessageHashes(account, hashes, opts)
		if err != nil {
			return errors.UnknownError.WithFormat("collect %v message hashes: %w", account.Url(), err)
		}
	}
	if it.Err() != nil {
		return errors.UnknownError.Wrap(it.Err())
	}

	err = records.Close()
	return errors.UnknownError.Wrap(err)
}

func (batch *Batch) collectMessages(w *snapshot.Writer, index, hashes *indexing.Bucket, opts *CollectOptions) error {
	// Open a records section
	records, err := w.OpenRecords()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	copts := collectOptions(index, opts)

	for i := 0; i < indexing.BucketCount; i++ {
		hashes, err := hashes.Read(byte(i))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		sort.Slice(hashes, func(i, j int) bool {
			return bytes.Compare(hashes[i].Hash[:], hashes[j].Hash[:]) < 0
		})

		for i, hash := range hashes {
			// Skip duplicates
			if i > 0 && hash.Hash == hashes[i-1].Hash {
				continue
			}

			if opts.Metrics != nil {
				opts.Metrics.Messages.Collecting++
			}

			// Check if the caller wants to skip this message
			message := batch.newMessage(messageKey{Hash: hash.Hash})
			if opts.Predicate != nil {
				ok, err := opts.Predicate(message)
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
				if !ok {
					continue
				}
			}

			// This may be a duplicate check, but better safe than sorry
			if opts.KeepMessage != nil {
				msg, err := message.Main().Get()
				if err != nil {
					slog.Error("Failed to collect message", "hash", logging.AsHex(hash.Hash), "error", err)
				} else {
					ok, err := opts.KeepMessage(msg)
					if err != nil {
						return errors.UnknownError.Wrap(err)
					}
					if !ok {
						continue
					}
				}
			}

			// Collect the message's records
			err = records.Collect(message, copts)
			if err != nil {
				return errors.UnknownError.WithFormat("collect %x: %w", hash, err)
			}

			// Collect the transaction's records. Executor v2 only uses the
			// transaction status, but transactions and signatures from v1 are still
			// stored here, so they should be collected.
			err = records.Collect(batch.newTransaction(transactionKey{Hash: hash.Hash}), copts)
			if err != nil {
				return errors.UnknownError.WithFormat("collect %x status: %w", hash, err)
			}
		}
	}

	err = records.Close()
	return errors.UnknownError.Wrap(err)
}

// AI: collectBPT writes the Binary Patricia Tree (BPT) section of the snapshot.
// AI: It iterates over the BPT and writes all key/value pairs to the snapshot.
// AI: Filtering can be applied via the Predicate option.
func (batch *Batch) collectBPT(w *snapshot.Writer, opts *CollectOptions) error {
	// AI: Optionally skip writing the BPT if the Predicate filter excludes it.
	// AI: The Predicate can be used to filter out the BPT from the snapshot.
	// AI: No new batch is created here.
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

	// AI: Open a raw section in the snapshot for writing BPT data.
	wr, err := w.OpenRaw(snapshot.SectionTypeBPT)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// AI: Iterate over all BPT entries in batches of 1000 and write each key/value
	// AI: to the snapshot.
	// Iterate over the BPT and collect hashes
	it := batch.BPT().Iterate(1000)
	for it.Next() {
		for _, entry := range it.Value() {
			key := batch.resolveAccountKey(entry.Key)
			err = wr.WriteValue(&snapshot.RecordEntry{
				Key:   key,
				Value: entry.Value[:],
			})
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}
	}
	if it.Err() != nil {
		return errors.UnknownError.Wrap(it.Err())
	}

	// AI: Close the BPT section writer and return any errors.
	err = wr.Close()
	return errors.UnknownError.Wrap(err)
}

type RestoreOptions struct {
	BatchRecordLimit int
	SkipHashCheck    bool
	Predicate        func(*snapshot.RecordEntry) (bool, error) //, func() (database.Value, error)

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

func Restore(db Beginner, file ioutil.SectionReader, opts *RestoreOptions) error {
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
			// Update the BPT, commit, and recreate the batch after every N
			// records
			if opts.BatchRecordLimit != 0 && count > 0 && count%opts.BatchRecordLimit == 0 {
				err = batch.UpdateBPT()
				if err != nil {
					return errors.UnknownError.WithFormat("update BPT: %w", err)
				}
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

			if opts.Metrics != nil {
				opts.Metrics.Records.Restoring = count
			}

			if opts.Predicate != nil {
				ok, err := opts.Predicate(entry)
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
				if !ok {
					continue
				}
			}

			v, err := values.Resolve[database.Value](batch, entry.Key)
			if err != nil {
				return errors.UnknownError.WithFormat("resolve %v: %w", entry.Key, err)
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

	// Force the BPT to update
	err = batch.UpdateBPT()
	if err != nil {
		return errors.UnknownError.WithFormat("update BPT: %w", err)
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

	// If hashes is not empty at this point it means the snapshot's BPT has an
	// entry that is not present in the newly created BPT, which will cause the
	// root hash to differ. This likely means some account has not been
	// restored.
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
	rh, err := batch.GetBptRootHash()
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

	bpt, err := snap.OpenBPT(-1)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	hashes := map[[32]byte][32]byte{}
	for {
		r, err := bpt.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, errors.UnknownError.Wrap(err)
		}

		hashes[r.Key.Hash()] = *(*[32]byte)(r.Value)
	}
	return hashes, nil
}

func collectMessageHashes(a *Account, hashes *indexing.Bucket, opts *CollectOptions) error {
	chains, err := a.Chains().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load chains index: %w", err)
	}
	for _, chain := range chains {
		// The data model incorrectly labels synthetic sequence chains as
		// transaction chains when in reality they are index chains
		typ := chain.Type
		if strings.HasPrefix(chain.Name, "synthetic-sequence(") {
			typ = merkle.ChainTypeIndex
		}
		if typ != merkle.ChainTypeTransaction {
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
		for _, h := range head.HashList {
			err = collectMessageHash(a, c, hashes, opts, *(*[32]byte)(h))
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}

		var gotMP bool
		for i := 256; i < int(head.Count); i += 256 {
			s, err := c.State(int64(i) - 1)
			switch {
			case err == nil:
				gotMP = true
			case !errors.Is(err, errors.NotFound):
				return errors.UnknownError.Wrap(err)
			default:
				// The mark point is missing. Assume this is an error if we have
				// a previous mark point. However if *all* mark points are
				// missing, assume that is intentional.
				if gotMP {
					slog.Error("Missing mark point", "account", a.Url(), "height", i-1)
				} else {
					slog.Debug("Skipping mark point", "account", a.Url(), "height", i-1)
				}
				continue
			}
			for _, h := range s.HashList {
				err = collectMessageHash(a, c, hashes, opts, *(*[32]byte)(h))
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
			}
		}
	}

	return errors.UnknownError.Wrap(err)
}

func collectMessageHash(a *Account, c *Chain2, hashes *indexing.Bucket, opts *CollectOptions, h [32]byte) error {
	// If we're not following signatures, just
	if opts.SkipMessageRefs {
		if opts.Metrics != nil {
			opts.Metrics.Messages.Count++
		}
		err := hashes.Write(h, nil)
		if err != nil {
			return errors.UnknownError.WithFormat("record %s chain entry: %w", c.Name(), err)
		}
		return nil
	}

	msg, err := a.parent.newMessage(messageKey{h}).Main().Get()
	if err != nil {
		slog.Error("Failed to collect message", "account", a.Url(), "chain", c.Name(), "hash", logging.AsHex(h), "error", err)
		return nil
	}

	if opts.KeepMessage != nil {
		ok, err := opts.KeepMessage(msg)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if !ok {
			return nil
		}
	}

	if opts.Metrics != nil {
		opts.Metrics.Messages.Count++
	}
	err = hashes.Write(h, nil)
	if err != nil {
		return errors.UnknownError.WithFormat("record %s chain entry: %w", c.Name(), err)
	}

	forTxn, ok := msg.(messaging.MessageForTransaction)
	if !ok {
		return nil
	}

	return collectMessageHash(a, c, hashes, opts, forTxn.GetTxID().Hash())
}

func writeSnapshotIndex(w *snapshot.Writer, index *indexing.Bucket, opts *CollectOptions) error {
	if !opts.BuildIndex {
		return nil
	}

	x, err := w.OpenIndex()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	for i := 0; i < indexing.BucketCount; i++ {
		entries, err := index.Read(byte(i))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		sort.Slice(entries, func(i, j int) bool {
			return bytes.Compare(entries[i].Hash[:], entries[j].Hash[:]) < 0
		})

		for _, e := range entries {
			err = x.Write(snapshot.RecordIndexEntry{
				Key:     e.Hash,
				Section: int(binary.BigEndian.Uint64(e.Value)),
				Offset:  binary.BigEndian.Uint64(e.Value[8:]),
			})
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}
	}

	err = x.Close()
	return errors.UnknownError.Wrap(err)
}

const indexDataSize = 16

func collectOptions(index *indexing.Bucket, opts *CollectOptions) snapshot.CollectOptions {
	copts := snapshot.CollectOptions{
		Walk: database.WalkOptions{
			IgnoreIndices: true,
		},
		Predicate: opts.Predicate,
	}

	if !opts.BuildIndex {
		return copts
	}

	copts.DidCollect = func(value database.Value, section, offset uint64) error {
		var b [indexDataSize]byte
		binary.BigEndian.PutUint64(b[:], section)
		binary.BigEndian.PutUint64(b[8:], offset)
		return index.Write(value.Key().Hash(), b[:])
	}

	return copts
}
