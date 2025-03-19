// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/util/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/remote"
)

var cmdDbSync = &cobra.Command{
	Use:   "sync [source] [destination]",
	Short: "Sync databases",
	Example: `` +
		`  debug db sync badger:///path/to/source.db leveldb:///path/to/dest.db` + "\n" +
		`  debug db sync badger:///path/to/source.db unix:///path/to/sync.sock` + "\n" +
		`  debug db sync unix:///path/to/sync.sock   leveldb:///path/to/dest.db` + "\n",
	Args: cobra.ExactArgs(2),
	Run:  syncDatabases,
}

func init() {
	cmdDb.AddCommand(cmdDbSync)
}

func syncDatabases(_ *cobra.Command, args []string) {
	srcDb, srcAddr := openDbUrl(args[0], false)
	dstDb, dstAddr := openDbUrl(args[1], true)

	var didProgress bool
	progress := func(s string) {
		if !didProgress {
			fmt.Println(s)
			didProgress = true
			return
		}
		fmt.Printf("\033[A\r\033[K%s\n", s)
	}

	switch {
	case srcDb == nil && dstDb == nil:
		fatalf("syncing from remote to remote is not supported")

	case dstDb == nil:
		// Serve for syncing
		serveDatabases(srcDb, dstAddr)

	case srcDb == nil:
		// Sync from remote
		defer func() { check(dstDb.Close()) }()
		dir, err := os.MkdirTemp("", "accumulate-db-sync-*")
		check(err)
		check(syncRemote(dstDb, srcAddr, dir, progress))

	default:
		// Sync local
		defer func() { check(srcDb.Close()) }()
		defer func() { check(dstDb.Close()) }()
		src := srcDb.Begin(nil, false)
		defer src.Discard()
		dir, err := os.MkdirTemp("", "accumulate-db-sync-*")
		check(err)
		check(syncDb(dstDb, src, dir, progress))
	}
}

func syncRemote(db keyvalue.Beginner, addr net.Addr, dir string, progress func(string)) error {
	return withRemoteKeyValueStore(addr, func(src *remote.Store) error {
		return syncDb(db, src, dir, progress)
	})
}

func syncDb(dst keyvalue.Beginner, src keyvalue.Store, dir string, progress func(string)) error {
	srcCount, dstCount := new(atomic.Int32), new(atomic.Int32)
	srcErr, dstErr := make(chan error), make(chan error)

	srcIdx, err := newDbIndex(filepath.Join(dir, "src"))
	if err != nil {
		return err
	}
	defer func() { _ = srcIdx.Close() }()

	dstIdx, err := newDbIndex(filepath.Join(dir, "dst"))
	if err != nil {
		return err
	}
	defer func() { _ = dstIdx.Close() }()

	go func() {
		defer close(srcErr)
		err := buildIndex(src, srcIdx, srcCount)
		srcErr <- err
	}()
	go func() {
		dst := dst.Begin(nil, false)
		defer dst.Discard()
		defer close(dstErr)
		err := buildIndex(dst, dstIdx, dstCount)
		dstErr <- err
	}()

	progress("Indexing...")
	errCh := srcErr
	t := time.NewTicker(time.Second / 2)
	defer t.Stop()
	for errCh != nil {
		select {
		case <-t.C:
			progress(fmt.Sprintf("Indexing src=%d dst=%d...", srcCount.Load(), dstCount.Load()))
		case err, ok := <-errCh:
			if ok {
				check(err)
			}
			if errCh == srcErr {
				errCh = dstErr
			} else {
				errCh = nil
			}
		}
	}

	progress("Comparing...")
	updates := map[[32]byte]*record.Key{}
	deletes := map[[32]byte]*record.Key{}
	for bucket := 0; bucket < indexing.BucketCount; bucket++ {
		src, err := srcIdx.Read(byte(bucket))
		if err != nil {
			return err
		}

		dst, err := dstIdx.Read(byte(bucket))
		if err != nil {
			return err
		}

		var i, j int
		for i < len(src) || j < len(dst) {
			select {
			case <-t.C:
				progress(fmt.Sprintf("Comparing (%d, %d)...", i, j))
			default:
			}

			switch {
			case i >= len(src):
				// No source
				deletes[dst[j].Key.Hash()] = dst[j].Key
				j++
				continue

			case j >= len(dst):
				// No destination
				updates[src[i].Key.Hash()] = src[i].Key
				i++
				continue
			}

			c := bytes.Compare(src[i].KeyHash[:], dst[j].KeyHash[:])
			switch {
			case c < 0:
				// No destination
				updates[src[i].Key.Hash()] = src[i].Key
				i++
				continue

			case c > 0:
				// No source
				deletes[dst[j].Key.Hash()] = dst[j].Key
				j++
				continue
			}

			if src[i].ValueHash != dst[j].ValueHash {
				updates[src[i].Key.Hash()] = src[i].Key
			}
			i++
			j++
		}
	}

	progress(fmt.Sprintf("Will add/modify=%d delete=%d\n", len(updates), len(deletes)))

	progress("Deleting...")
	t.Reset(time.Second / 2)
	batch := dst.Begin(nil, true)
	defer batch.Discard()
	for i, k := range sorted(deletes) {
		select {
		case <-t.C:
			progress(fmt.Sprintf("Deleting %d/%d...", i, len(deletes)))
		default:
		}

		err := batch.Delete(deletes[k])
		if err != nil {
			return err
		}
	}
	err = batch.Commit()
	if err != nil {
		return err
	}

	progress("Updating...")
	t.Reset(time.Second / 2)
	batch = dst.Begin(nil, true)
	defer func() { batch.Discard() }()
	for i, k := range sorted(updates) {
		select {
		case <-t.C:
			progress(fmt.Sprintf("Updating %d/%d...", i, len(updates)))
			err = batch.Commit()
			if err != nil {
				return err
			}
			batch = dst.Begin(nil, true)
		default:
		}

		key := updates[k]
		data, err := src.Get(key)
		if err != nil {
			return err
		}
		err = batch.Put(key, data)
		if err != nil {
			return err
		}
	}

	return batch.Commit()
}

func sorted[T any](m map[[32]byte]T) [][32]byte {
	// Sort for repeatability
	keys := make([][32]byte, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})

	return keys
}

type KeyAndHash struct {
	Key  *record.Key
	Hash [32]byte
}

func buildIndex(db keyvalue.Store, index *dbIndex, count *atomic.Int32) error {
	switch db := db.(type) {
	case *remote.Store:
		return db.ForEachHash(func(key *record.Key, hash [32]byte) error {
			count.Add(1)
			return index.Write(key, hash)
		})

	default:
		return db.ForEach(func(key *record.Key, value []byte) error {
			count.Add(1)
			hash := sha256.Sum256(value)
			return index.Write(key, hash)
		})
	}
}

type dbIndex struct {
	offset int64
	keys   *indexing.File
	hashes *indexing.Bucket
}

func newDbIndex(prefix string) (*dbIndex, error) {
	keys, err := indexing.OpenFile(prefix+"keys", 0, true)
	if err != nil {
		return nil, err
	}

	hashes, err := indexing.OpenBucket(prefix+"hash", dbIndexEntrySize, true)
	if err != nil {
		return nil, err
	}

	return &dbIndex{keys: keys, hashes: hashes}, nil
}

func (d *dbIndex) Close() error {
	e1 := d.keys.Close()
	e2 := d.hashes.Close()
	if e1 != nil {
		return e1
	}
	return e2
}

const dbIndexEntrySize = 48

type dbIndexEntry struct {
	Key       *record.Key
	KeyHash   [32]byte
	ValueHash [32]byte
}

func (d *dbIndex) Write(key *record.Key, hash [32]byte) error {
	b, err := key.MarshalBinary()
	if err != nil {
		return err
	}

	n, err := d.keys.Write(b)
	if err != nil {
		return err
	}

	var data [dbIndexEntrySize]byte
	*(*[32]byte)(data[:]) = hash
	binary.BigEndian.PutUint64(data[32:], uint64(d.offset))
	binary.BigEndian.PutUint64(data[40:], uint64(n))
	d.offset += int64(n)
	return d.hashes.Write(key.Hash(), data[:])
}

func (d *dbIndex) Read(i byte) ([]dbIndexEntry, error) {
	x, err := d.hashes.Read(i)
	if err != nil {
		return nil, err
	}
	sort.Slice(x, func(i, j int) bool {
		return bytes.Compare(x[i].Hash[:], x[j].Hash[:]) < 0
	})

	y := make([]dbIndexEntry, 0, len(x))
	for _, e := range x {
		hash := *(*[32]byte)(e.Value)
		offset := binary.BigEndian.Uint64(e.Value[32:])
		size := binary.BigEndian.Uint64(e.Value[40:])

		b := make([]byte, size)
		n, err := d.keys.ReadAt(b, int64(offset))
		if err != nil {
			return nil, err
		}
		if n < int(size) {
			return nil, fmt.Errorf("short read")
		}

		key := new(record.Key)
		if err := key.UnmarshalBinary(b); err != nil {
			return nil, err
		}

		y = append(y, dbIndexEntry{Key: key, KeyHash: e.Hash, ValueHash: hash})
	}
	return y, nil
}
