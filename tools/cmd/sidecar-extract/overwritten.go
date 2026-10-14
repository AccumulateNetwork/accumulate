// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// Overwritten is the progress file beside overwritten.db: the archives in
// index order, the last live key handled, and the counts.
type Overwritten struct {
	Archives   []string         `json:"archives"`
	LastKey    string           `json:"lastKey"`
	Done       bool             `json:"done"`
	Seen       uint64           `json:"seen"`
	Present    map[string]int64 `json:"present"`
	Differ     map[string]int64 `json:"differ"`
	Unreadable map[string]int64 `json:"unreadable"`
}

// keepOverwritten builds overwritten.db in out: for every key the live
// databases hold, the value each archive holds for it when that value
// differs, under key || byte(archive index). The extractor leaves such keys
// to the live database, so a chain the live database restarted under the
// same keys -- the Directory's root chain at the reorg -- is in no store it
// builds; this one holds the archive's side of every such key.
func keepOverwritten(out string, currentPaths, archiveArgs []string) error {
	progPath := filepath.Join(out, "overwritten.json")
	prog := &Overwritten{Present: map[string]int64{}, Differ: map[string]int64{}, Unreadable: map[string]int64{}}
	// Values are read through the archive's verified value-log reader, not
	// Badger's, which panics on an index entry whose log bytes are gone
	// (vlog.go). The archives are not closed, as in run.
	type arch struct {
		*archive
		txn   *badger.Txn
		moves *badger.Iterator
	}
	var archives []arch
	for _, arg := range archiveArgs {
		name, path, ok := strings.Cut(arg, "=")
		if !ok {
			return fmt.Errorf("archive %q: want name=path", arg)
		}
		group, _, _ := strings.Cut(name, "/")
		db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
		if err != nil {
			return fmt.Errorf("archive %s: %w", name, err)
		}
		a := &archive{group: group, name: name, db: db, log: &vlog{dir: path}}
		txn := db.NewTransaction(false)
		defer txn.Discard()
		moves := a.newMoves(txn)
		defer moves.Close()
		archives = append(archives, arch{a, txn, moves})
		prog.Archives = append(prog.Archives, name)
	}
	if b, err := os.ReadFile(progPath); err == nil {
		saved := new(Overwritten)
		if err := json.Unmarshal(b, saved); err != nil {
			return fmt.Errorf("read %s: %w", progPath, err)
		}
		if strings.Join(saved.Archives, ",") != strings.Join(prog.Archives, ",") {
			return fmt.Errorf("%s was written for archives %v, not %v", progPath, saved.Archives, prog.Archives)
		}
		if saved.Done {
			log.Printf("already done: %s", b)
			return nil
		}
		prog = saved
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	save := func() error {
		b, err := json.MarshalIndent(prog, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(progPath, b, 0644)
	}

	db, err := openOutput(filepath.Join(out, "overwritten.db"))
	if err != nil {
		return err
	}
	defer db.Close()

	resume, _ := hex.DecodeString(prog.LastKey)
	batch := new(leveldb.Batch)
	var lastKey []byte
	flush := func() error {
		if err := db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
			return err
		}
		batch.Reset()
		if lastKey != nil {
			prog.LastKey = hex.EncodeToString(lastKey)
		}
		return save()
	}
	start, report := time.Now(), time.Now()
	for _, path := range currentPaths {
		live, err := leveldb.OpenFile(path, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
		if err != nil {
			return fmt.Errorf("current %s: %w", path, err)
		}
		it := live.NewIterator(nil, nil)
		if len(resume) > 0 {
			if it.Seek(resume) && bytes.Equal(it.Key(), resume) {
				it.Next()
			}
		} else {
			it.First()
		}
		for ; it.Valid(); it.Next() {
			k, v := it.Key(), it.Value()
			prog.Seen++
			for i, a := range archives {
				item, err := a.txn.Get(k)
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				if err != nil {
					return fmt.Errorf("archive %s: %w", a.name, err)
				}
				prog.Present[a.name]++
				av, err := a.resolve(item, a.moves)
				if err != nil {
					prog.Unreadable[a.name]++
					continue
				}
				if bytes.Equal(av, v) {
					continue
				}
				prog.Differ[a.name]++
				batch.Put(append(append([]byte(nil), k...), byte(i)), av)
			}
			lastKey = append(lastKey[:0], k...)
			if batch.Len() >= 10000 {
				if err := flush(); err != nil {
					return err
				}
			}
			if time.Since(report) > 30*time.Second {
				report = time.Now()
				log.Printf("%v: %d live keys; present %v, differ %v, unreadable %v", time.Since(start).Round(time.Second), prog.Seen, prog.Present, prog.Differ, prog.Unreadable)
			}
		}
		it.Release()
		live.Close()
		if err := it.Error(); err != nil {
			return err
		}
	}
	prog.Done = true
	if err := flush(); err != nil {
		return err
	}
	log.Printf("done in %v: %d live keys; present %v, differ %v, unreadable %v", time.Since(start).Round(time.Second), prog.Seen, prog.Present, prog.Differ, prog.Unreadable)
	return nil
}
