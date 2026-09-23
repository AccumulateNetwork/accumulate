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
// see every partition's value for a key at once. The key space is split into
// shards walked in parallel: each value is a random value-log read, so one
// walker is bound by read latency, not bandwidth.
//
// A key two archive partitions hold with different values is not written to
// the sidecar, which serves one value per key. Each variant goes to the
// conflicts database under the key followed by the partition's index.
//
// Every value is verified: a value-log value is taken only from an intact log
// entry for exactly its key and version (vlog.go). One that fails is listed in
// unreadable.log, and after the walk each archive's log is scanned for it by
// key and version and the key decided again (recover.go). What no archive can
// produce is listed in lost.log.
//
// The archives and the current databases are opened read-only. A Badger v1
// database that was not closed cleanly refuses a read-only open; pass a copy of
// it with -writable.
//
//	sidecar-extract -out <dir> -current <leveldb> [-current ...] [-shards n] \
//	    [-writable <name>] <name>=<badger> [<name>=<badger> ...]
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
	"sync"
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
var flagShards = flag.Int("shards", 32, "key ranges walked in parallel (at most 65536); fixed for the life of an output directory")
var flagCurrent listFlag
var flagWritable listFlag

func main() {
	flag.Var(&flagCurrent, "current", "a current LevelDB database (repeatable); a key any of them holds is not extracted")
	flag.Var(&flagWritable, "writable", "an archive name to open writable, for a copy that will not open read-only (repeatable)")
	flag.Parse()
	if *flagOut == "" || len(flagCurrent) == 0 || flag.NArg() == 0 || *flagShards < 1 || *flagShards > 1<<16 {
		flag.Usage()
		os.Exit(2)
	}

	err := run(*flagOut, flagCurrent, flagWritable, flag.Args(), *flagShards)
	if err != nil {
		log.Fatal(err)
	}
}

// Counts are what a walk found.
type Counts struct {
	Seen      map[string]int64 `json:"seen"`      // keys read, per archive
	Keys      int64            `json:"keys"`      // distinct archive keys
	Present   int64            `json:"present"`   // held by the current database
	Extracted int64            `json:"extracted"` // written to the sidecar
	Bytes     int64            `json:"bytes"`     // value bytes written to the sidecar
	Conflicts int64            `json:"conflicts"` // keys whose archives disagree
	Odd       int64            `json:"odd"`       // keys that are not 32-byte hashes

	// Values an archive holds a key for but cannot produce, per archive. Each
	// is listed in unreadable.log.
	Unreadable map[string]int64 `json:"unreadable"`
	Lost       int64            `json:"lost"` // keys no archive could produce a value for
}

func (c *Counts) init() {
	if c.Seen == nil {
		c.Seen = map[string]int64{}
	}
	if c.Unreadable == nil {
		c.Unreadable = map[string]int64{}
	}
}

func (c *Counts) add(d *Counts) {
	c.init()
	for k, v := range d.Seen {
		c.Seen[k] += v
	}
	for k, v := range d.Unreadable {
		c.Unreadable[k] += v
	}
	c.Keys += d.Keys
	c.Present += d.Present
	c.Extracted += d.Extracted
	c.Bytes += d.Bytes
	c.Conflicts += d.Conflicts
	c.Odd += d.Odd
	c.Lost += d.Lost
}

// Shard is one key range [Lo, Hi); an empty Hi is the end of the key space.
type Shard struct {
	Lo      string `json:"lo"`
	Hi      string `json:"hi"`
	LastKey string `json:"lastKey"` // the last key flushed; the walk resumes after it
	Done    bool   `json:"done"`
	Counts
}

// Progress is persisted after every flushed batch, so a stopped run resumes
// each shard after the last key it wrote.
type Progress struct {
	Done     bool      `json:"done"`
	Archives []string  `json:"archives"`
	Total    Counts    `json:"total"`
	Shards   []*Shard  `json:"shards"`
	Recovery *Recovery `json:"recovery,omitempty"`
}

type archive struct {
	name string
	db   *badger.DB
	log  *vlog
}

type extractor struct {
	archives  []*archive
	current   map[[32]byte]struct{}
	sidecar   *leveldb.DB
	conflicts *leveldb.DB

	mu         sync.Mutex // guards the fields below
	prog       *Progress
	progPath   string
	unreadable *os.File
}

func run(out string, currentPaths, writable, archiveArgs []string, shards int) error {
	x := &extractor{progPath: filepath.Join(out, "progress.json")}

	// Open the archives
	for _, arg := range archiveArgs {
		name, path, ok := strings.Cut(arg, "=")
		if !ok {
			return fmt.Errorf("archive %q: want name=path", arg)
		}
		db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(!contains(writable, name)).WithLogger(nil))
		if err != nil {
			return fmt.Errorf("archive %s: %w", name, err)
		}
		// The archives are not closed: they are read-only or copies, and a
		// run that fails part way must not wait on them.
		x.archives = append(x.archives, &archive{name: name, db: db, log: &vlog{dir: path}})
	}
	names := make([]string, len(x.archives))
	for i, a := range x.archives {
		names[i] = a.name
	}

	// Unmarshal decodes into the slices it finds, so the defaults must not
	// share names' backing array or the comparison below compares names to
	// itself
	x.prog = &Progress{Archives: append([]string(nil), names...), Shards: split(shards)}
	if b, err := os.ReadFile(x.progPath); err == nil {
		if err := json.Unmarshal(b, x.prog); err != nil {
			return fmt.Errorf("read progress: %w", err)
		}
		if x.prog.Done {
			log.Printf("already done: %s", b)
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if strings.Join(x.prog.Archives, ",") != strings.Join(names, ",") {
		return fmt.Errorf("progress was written for archives %v, not %v", x.prog.Archives, names)
	}
	if len(x.prog.Shards) != shards {
		return fmt.Errorf("progress was written for %d shards, not %d", len(x.prog.Shards), shards)
	}

	// Load the current key set
	x.current = map[[32]byte]struct{}{}
	for _, path := range currentPaths {
		n, err := loadKeys(path, x.current)
		if err != nil {
			return fmt.Errorf("current %s: %w", path, err)
		}
		log.Printf("current %s: %d keys", path, n)
	}
	log.Printf("current: %d distinct keys", len(x.current))

	// Open the outputs
	var err error
	x.sidecar, err = openOutput(filepath.Join(out, "sidecar.db"))
	if err != nil {
		return err
	}
	defer x.sidecar.Close()
	x.conflicts, err = openOutput(filepath.Join(out, "conflicts.db"))
	if err != nil {
		return err
	}
	defer x.conflicts.Close()
	x.unreadable, err = os.OpenFile(filepath.Join(out, "unreadable.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer x.unreadable.Close()

	// Walk the shards
	start := time.Now()
	stop := make(chan struct{})
	go x.report(start, stop)
	errs := make(chan error, len(x.prog.Shards))
	for _, s := range x.prog.Shards {
		go func(s *Shard) { errs <- x.walk(s) }(s)
	}
	var first error
	for range x.prog.Shards {
		if err := <-errs; err != nil && first == nil {
			first = err
		}
	}
	close(stop)
	if first != nil {
		return first
	}

	if err := x.recoverUnreadable(out, 8); err != nil {
		return fmt.Errorf("recovery: %w", err)
	}

	x.mu.Lock()
	defer x.mu.Unlock()
	x.prog.Done = true
	if err := x.save(); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(struct {
		Walk     Counts
		Recovery *Recovery
	}{x.prog.Total, x.prog.Recovery}, "", "  ")
	log.Printf("done in %v:\n%s", time.Since(start).Round(time.Second), b)
	return nil
}

// split divides the key space into n ranges on its first two bytes.
func split(n int) []*Shard {
	bound := func(i int) string {
		if i == n {
			return ""
		}
		v := i * (1 << 16) / n
		return hex.EncodeToString([]byte{byte(v >> 8), byte(v)})
	}
	shards := make([]*Shard, n)
	for i := range shards {
		shards[i] = &Shard{Lo: bound(i), Hi: bound(i + 1)}
	}
	return shards
}

// walk merges every archive over one shard.
func (x *extractor) walk(s *Shard) error {
	x.mu.Lock()
	done, lo, hi, last := s.Done, s.Lo, s.Hi, s.LastKey
	x.mu.Unlock()
	if done {
		return nil
	}
	loKey, _ := hex.DecodeString(lo)
	hiKey, _ := hex.DecodeString(hi)
	resume, err := hex.DecodeString(last)
	if err != nil {
		return fmt.Errorf("shard %s: last key: %w", lo, err)
	}

	type cursor struct {
		*archive
		index int
		it    *badger.Iterator
		moves *badger.Iterator
	}
	var cursors []*cursor
	for i, a := range x.archives {
		txn := a.db.NewTransaction(false)
		defer txn.Discard()
		io := badger.DefaultIteratorOptions
		io.PrefetchValues = false
		it := txn.NewIterator(io)
		defer it.Close()
		if len(resume) > 0 {
			it.Seek(resume)
			if it.Valid() && bytes.Equal(it.Item().Key(), resume) {
				it.Next()
			}
		} else {
			it.Seek(loKey)
		}
		moves := a.newMoves(txn)
		defer moves.Close()
		cursors = append(cursors, &cursor{a, i, it, moves})
	}
	valid := func(c *cursor) bool {
		return c.it.Valid() && (len(hiKey) == 0 || bytes.Compare(c.it.Item().Key(), hiKey) < 0)
	}

	var counts Counts
	counts.init()
	sideBatch, confBatch := new(leveldb.Batch), new(leveldb.Batch)
	var unreadable bytes.Buffer
	var batchBytes int
	var lastKey []byte
	flush := func(done bool) error {
		if err := x.conflicts.Write(confBatch, &opt.WriteOptions{Sync: true}); err != nil {
			return err
		}
		if err := x.sidecar.Write(sideBatch, &opt.WriteOptions{Sync: true}); err != nil {
			return err
		}

		x.mu.Lock()
		defer x.mu.Unlock()
		if unreadable.Len() > 0 {
			if _, err := x.unreadable.Write(unreadable.Bytes()); err != nil {
				return err
			}
			if err := x.unreadable.Sync(); err != nil {
				return err
			}
		}
		s.Counts.add(&counts)
		x.prog.Total.add(&counts)
		if lastKey != nil {
			s.LastKey = hex.EncodeToString(lastKey)
		}
		s.Done = done

		sideBatch.Reset()
		confBatch.Reset()
		unreadable.Reset()
		batchBytes = 0
		counts = Counts{}
		counts.init()
		return x.save()
	}

	at := make([]*cursor, 0, len(cursors))
	from := make([]*cursor, 0, len(cursors))
	vals := make([][]byte, 0, len(cursors))
	for {
		// The smallest key any archive is at
		var min []byte
		for _, c := range cursors {
			if !valid(c) {
				continue
			}
			k := c.it.Item().Key()
			if min == nil || bytes.Compare(k, min) < 0 {
				min = k
			}
		}
		if min == nil {
			break
		}
		key := append([]byte(nil), min...)

		// Every archive's value for it
		at, from, vals = at[:0], from[:0], vals[:0]
		for _, c := range cursors {
			if valid(c) && bytes.Equal(c.it.Item().Key(), key) {
				counts.Seen[c.name]++
				at = append(at, c)
			}
		}
		counts.Keys++

		var present bool
		if len(key) == 32 {
			_, present = x.current[[32]byte(key)]
		} else {
			log.Printf("odd key %x in %s", key, at[0].name)
			counts.Odd++
		}

		if present {
			counts.Present++
		} else {
			// An archive whose value cannot be read is left out of the
			// comparison; the others still decide
			for _, c := range at {
				v, err := c.resolve(c.it.Item(), c.moves)
				if err != nil {
					counts.Unreadable[c.name]++
					fmt.Fprintf(&unreadable, "%s %x %d %v\n", c.name, key, c.it.Item().Version(), err)
					continue
				}
				from = append(from, c)
				vals = append(vals, v)
			}

			agree := true
			for _, v := range vals {
				agree = agree && bytes.Equal(v, vals[0])
			}
			switch {
			case len(vals) == 0:
				counts.Lost++
			case agree:
				sideBatch.Put(key, vals[0])
				counts.Extracted++
				counts.Bytes += int64(len(vals[0]))
				batchBytes += len(key) + len(vals[0])
			default:
				for i, c := range from {
					confBatch.Put(append(key[:len(key):len(key)], byte(c.index)), vals[i])
					batchBytes += len(key) + 1 + len(vals[i])
				}
				counts.Conflicts++
			}
		}

		for _, c := range at {
			c.it.Next()
		}
		lastKey = key

		if batchBytes >= 16<<20 || sideBatch.Len()+confBatch.Len() >= 50_000 {
			if err := flush(false); err != nil {
				return err
			}
		}
	}
	return flush(true)
}

// save writes the progress file. The caller holds x.mu.
func (x *extractor) save() error {
	b, err := json.MarshalIndent(x.prog, "", "  ")
	if err != nil {
		return err
	}
	tmp := x.progPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, x.progPath)
}

func (x *extractor) report(start time.Time, stop <-chan struct{}) {
	x.mu.Lock()
	startKeys := x.prog.Total.Keys
	x.mu.Unlock()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		x.mu.Lock()
		c := x.prog.Total
		var done int
		for _, s := range x.prog.Shards {
			if s.Done {
				done++
			}
		}
		rate := float64(c.Keys-startKeys) / time.Since(start).Seconds()
		log.Printf("shards done %d/%d: keys=%d present=%d extracted=%d (%.1f GiB) conflicts=%d unreadable=%v lost=%d odd=%d seen=%v %.0f keys/s",
			done, len(x.prog.Shards), c.Keys, c.Present, c.Extracted, float64(c.Bytes)/(1<<30), c.Conflicts, c.Unreadable, c.Lost, c.Odd, c.Seen, rate)
		x.mu.Unlock()
	}
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

func contains(l []string, s string) bool {
	for _, t := range l {
		if t == s {
			return true
		}
	}
	return false
}
