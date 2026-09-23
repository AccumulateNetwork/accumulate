// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// sidecar-extract builds the sidecar database of #4273: every record the
// pre-13-July-2025 archives hold whose key the current database does not.
//
// Both backends key a record by record.Key.Hash() and store the value as is,
// so the difference is taken on raw keys and no record is decoded. The archive
// partitions are Badger v1; they are merged in key order, which lets one pass
// see every partition's value for a key at once and hands the sidecar sorted
// writes.
//
// A key two archive partitions hold with different values is not written to
// the sidecar, which serves one value per key. Each variant goes to the
// conflicts database under the key followed by the partition's index.
//
// The archives and the current databases are opened read-only. A Badger v1
// database that was not closed cleanly refuses a read-only open; pass a copy of
// it with -writable.
//
//	sidecar-extract -out <dir> -current <leveldb> [-current ...] \
//	    [-writable] <name>=<badger> [<name>=<badger> ...]
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(s string) error { *l = append(*l, s); return nil }

var flagOut = flag.String("out", "", "directory for the sidecar, the conflicts database and the progress file")
var flagCurrent listFlag
var flagWritable listFlag

func main() {
	flag.Var(&flagCurrent, "current", "a current LevelDB database (repeatable); a key any of them holds is not extracted")
	flag.Var(&flagWritable, "writable", "an archive name to open writable, for a copy that will not open read-only (repeatable)")
	flag.Parse()
	if *flagOut == "" || len(flagCurrent) == 0 || flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	err := run(*flagOut, flagCurrent, flagWritable, flag.Args())
	if err != nil {
		log.Fatal(err)
	}
}

// Progress is persisted after every flushed batch, so a stopped run resumes
// after the last key it wrote.
type Progress struct {
	LastKey  string           `json:"lastKey"`
	Done     bool             `json:"done"`
	Archives []string         `json:"archives"`
	Seen     map[string]int64 `json:"seen"` // keys read, per archive

	Keys      int64 `json:"keys"`      // distinct archive keys
	Present   int64 `json:"present"`   // held by the current database
	Extracted int64 `json:"extracted"` // written to the sidecar
	Bytes     int64 `json:"bytes"`     // value bytes written to the sidecar
	Conflicts int64 `json:"conflicts"` // keys whose archives disagree
	Odd       int64 `json:"odd"`       // keys that are not 32-byte hashes

	// Values an archive holds a key for but cannot produce, per archive. Each
	// is listed in unreadable.log.
	Unreadable map[string]int64 `json:"unreadable"`
	Lost       int64            `json:"lost"` // keys no archive could produce a value for
}

type archive struct {
	name string
	db   *badger.DB
	txn  *badger.Txn
	it   *badger.Iterator
}

func run(out string, currentPaths, writable, archiveArgs []string) error {
	progressPath := filepath.Join(out, "progress.json")
	prog := &Progress{Seen: map[string]int64{}, Unreadable: map[string]int64{}}
	if b, err := os.ReadFile(progressPath); err == nil {
		if err := json.Unmarshal(b, prog); err != nil {
			return fmt.Errorf("read progress: %w", err)
		}
		if prog.Unreadable == nil {
			prog.Unreadable = map[string]int64{}
		}
		if prog.Done {
			log.Printf("already done: %s", b)
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Load the current key set
	current := map[[32]byte]struct{}{}
	for _, path := range currentPaths {
		n, err := loadKeys(path, current)
		if err != nil {
			return fmt.Errorf("current %s: %w", path, err)
		}
		log.Printf("current %s: %d keys", path, n)
	}
	log.Printf("current: %d distinct keys", len(current))

	// Open the archives
	var archives []*archive
	for _, arg := range archiveArgs {
		name, path, ok := strings.Cut(arg, "=")
		if !ok {
			return fmt.Errorf("archive %q: want name=path", arg)
		}
		a, err := openArchive(name, path, contains(writable, name))
		if err != nil {
			return fmt.Errorf("archive %s: %w", name, err)
		}
		// The archives are not closed. A corrupt value log panics inside
		// Badger with its lock held (see value), and Close then deadlocks;
		// they are read-only or copies, so exiting without closing is safe.
		archives = append(archives, a)
	}
	names := make([]string, len(archives))
	for i, a := range archives {
		names[i] = a.name
	}
	if prog.Archives == nil {
		prog.Archives = names
	} else if strings.Join(prog.Archives, ",") != strings.Join(names, ",") {
		return fmt.Errorf("progress was written for archives %v, not %v", prog.Archives, names)
	}

	// Open the outputs
	sidecar, err := openOutput(filepath.Join(out, "sidecar.db"))
	if err != nil {
		return err
	}
	defer sidecar.Close()
	conflicts, err := openOutput(filepath.Join(out, "conflicts.db"))
	if err != nil {
		return err
	}
	defer conflicts.Close()
	unreadable, err := os.OpenFile(filepath.Join(out, "unreadable.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer unreadable.Close()

	// Resume after the last written key
	var resume []byte
	if prog.LastKey != "" {
		resume, err = hex.DecodeString(prog.LastKey)
		if err != nil {
			return fmt.Errorf("progress last key: %w", err)
		}
		log.Printf("resuming after %x", resume)
	}
	for _, a := range archives {
		a.it.Seek(resume)
		if resume != nil && a.it.Valid() && bytes.Equal(a.it.Item().Key(), resume) {
			a.it.Next()
		}
	}

	sideBatch, confBatch := new(leveldb.Batch), new(leveldb.Batch)
	var batchBytes int
	var lastKey []byte
	flush := func() error {
		if sideBatch.Len() == 0 && confBatch.Len() == 0 {
			return nil
		}
		if err := unreadable.Sync(); err != nil {
			return err
		}
		if err := conflicts.Write(confBatch, &opt.WriteOptions{Sync: true}); err != nil {
			return err
		}
		if err := sidecar.Write(sideBatch, &opt.WriteOptions{Sync: true}); err != nil {
			return err
		}
		sideBatch.Reset()
		confBatch.Reset()
		batchBytes = 0
		prog.LastKey = hex.EncodeToString(lastKey)
		return writeProgress(progressPath, prog)
	}

	start, lastLog := time.Now(), time.Now()
	startKeys := prog.Keys
	at := make([]*archive, 0, len(archives))
	vals := make([][]byte, 0, len(archives))
	for {
		// The smallest key any archive is at
		var min []byte
		for _, a := range archives {
			if !a.it.Valid() {
				continue
			}
			k := a.it.Item().Key()
			if min == nil || bytes.Compare(k, min) < 0 {
				min = k
			}
		}
		if min == nil {
			break
		}
		key := append([]byte(nil), min...)

		// Every archive's value for it
		at, vals = at[:0], vals[:0]
		for _, a := range archives {
			if !a.it.Valid() || !bytes.Equal(a.it.Item().Key(), key) {
				continue
			}
			prog.Seen[a.name]++
			at = append(at, a)
		}
		prog.Keys++

		var present bool
		if len(key) == 32 {
			_, present = current[[32]byte(key)]
		} else {
			if prog.Odd < 20 {
				log.Printf("odd key %x in %s", key, at[0].name)
			}
			prog.Odd++
		}

		if present {
			prog.Present++
		} else {
			// An archive whose value cannot be read is left out of the
			// comparison; the others still decide
			from := at[:0:0]
			for _, a := range at {
				v, err := value(a.it.Item())
				if err != nil {
					if prog.Unreadable[a.name] < 20 {
						log.Printf("%s: value of %x: %v", a.name, key, err)
					}
					prog.Unreadable[a.name]++
					if _, err := fmt.Fprintf(unreadable, "%s %x %v\n", a.name, key, err); err != nil {
						return err
					}
					continue
				}
				from = append(from, a)
				vals = append(vals, v)
			}

			agree := true
			for _, v := range vals {
				agree = agree && bytes.Equal(v, vals[0])
			}
			switch {
			case len(vals) == 0:
				prog.Lost++
			case agree:
				sideBatch.Put(key, vals[0])
				prog.Extracted++
				prog.Bytes += int64(len(vals[0]))
				batchBytes += len(key) + len(vals[0])
			default:
				for i, a := range from {
					confBatch.Put(append(key[:len(key):len(key)], byte(index(archives, a))), vals[i])
					batchBytes += len(key) + 1 + len(vals[i])
				}
				prog.Conflicts++
			}
		}

		for _, a := range at {
			a.it.Next()
		}
		lastKey = key

		if batchBytes >= 64<<20 || sideBatch.Len()+confBatch.Len() >= 200_000 {
			if err := flush(); err != nil {
				return err
			}
		}

		if time.Since(lastLog) >= time.Minute {
			lastLog = time.Now()
			rate := float64(prog.Keys-startKeys) / time.Since(start).Seconds()
			log.Printf("at %x: keys=%d present=%d extracted=%d (%.1f GiB) conflicts=%d odd=%d seen=%v %.0f keys/s",
				key[:4], prog.Keys, prog.Present, prog.Extracted, float64(prog.Bytes)/(1<<30), prog.Conflicts, prog.Odd, prog.Seen, rate)
		}
	}

	if err := flush(); err != nil {
		return err
	}
	prog.Done = true
	if err := writeProgress(progressPath, prog); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(prog, "", "  ")
	log.Printf("done in %v:\n%s", time.Since(start).Round(time.Second), b)
	return nil
}

func loadKeys(path string, into map[[32]byte]struct{}) (int, error) {
	db, err := leveldb.OpenFile(path, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return 0, err
	}
	defer db.Close()

	it := db.NewIterator(nil, nil)
	defer it.Release()
	var n int
	for it.Next() {
		if len(it.Key()) != 32 {
			return n, fmt.Errorf("key %x is not a 32-byte hash", it.Key())
		}
		into[[32]byte(it.Key())] = struct{}{}
		n++
	}
	return n, it.Error()
}

func openArchive(name, path string, writable bool) (*archive, error) {
	opts := badger.DefaultOptions(path).WithReadOnly(!writable).WithLogger(nil)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	txn := db.NewTransaction(false)
	io := badger.DefaultIteratorOptions
	io.PrefetchValues = false
	return &archive{name: name, db: db, txn: txn, it: txn.NewIterator(io)}, nil
}

// value reads an item's value, turning Badger's panic on a value pointer that
// does not decode — a corrupt or stale value log region, found in the
// pre-reorg dn archive — into an error.
func value(item *badger.Item) (v []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unreadable value: %v", r)
		}
	}()
	return item.ValueCopy(nil)
}

func openOutput(path string) (*leveldb.DB, error) {
	// The bloom filter matches the node's, so a sidecar read through an
	// overlay answers a miss without walking the levels
	return leveldb.OpenFile(path, &opt.Options{
		Filter:              filter.NewBloomFilter(10),
		WriteBuffer:         128 * opt.MiB,
		CompactionTableSize: 64 * opt.MiB,
		CompactionTotalSize: 640 * opt.MiB,
	})
}

func writeProgress(path string, prog *Progress) error {
	b, err := json.MarshalIndent(prog, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func contains(l []string, s string) bool {
	for _, t := range l {
		if t == s {
			return true
		}
	}
	return false
}

func index(l []*archive, a *archive) int {
	for i, b := range l {
		if b == a {
			return i
		}
	}
	panic("not an archive")
}
