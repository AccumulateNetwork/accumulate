// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	badger "github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// Recovery is the pass after the walk: every value the walk could not read is
// looked for in its archive's value log by key and version, and each such key
// is decided again with what was found.
type Recovery struct {
	Done      bool                  `json:"done"`
	Keys      int                   `json:"keys"` // distinct keys with an unreadable value
	Scans     map[string]*scanStats `json:"scans"`
	Extracted int                   `json:"extracted"` // now in the sidecar
	Conflicts int                   `json:"conflicts"` // now in conflicts
	Lost      int                   `json:"lost"`      // no archive has a value; listed in lost.log
}

// recoverUnreadable runs the recovery pass. It rewrites every key it decides,
// so running it again is harmless.
func (x *extractor) recoverUnreadable(out string, workers int) error {
	// What the walk could not read: archive, key, version
	misses := map[string]map[string]uint64{}
	f, err := os.Open(filepath.Join(out, "unreadable.log"))
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.SplitN(sc.Text(), " ", 4)
		if len(fields) < 3 {
			return fmt.Errorf("unreadable.log: %q", sc.Text())
		}
		key, err := hex.DecodeString(fields[1])
		if err != nil {
			return fmt.Errorf("unreadable.log: %q: %w", sc.Text(), err)
		}
		version, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return fmt.Errorf("unreadable.log: %q: %w", sc.Text(), err)
		}
		if misses[string(key)] == nil {
			misses[string(key)] = map[string]uint64{}
		}
		misses[string(key)][fields[0]] = version
	}
	f.Close()
	if err := sc.Err(); err != nil {
		return err
	}

	rec := &Recovery{Keys: len(misses), Scans: map[string]*scanStats{}}
	log.Printf("recovery: %d keys with an unreadable value", len(misses))

	// Look for each in its archive's log
	found := map[string]map[string][]byte{}
	for _, a := range x.archives {
		want := map[string]bool{}
		for key, by := range misses {
			if v, ok := by[a.name]; ok {
				want[string(keyWithTs([]byte(key), v))] = true
			}
		}
		if len(want) == 0 {
			continue
		}
		vals, stats, err := a.scan(want, workers)
		if err != nil {
			return fmt.Errorf("scan %s: %w", a.name, err)
		}
		found[a.name] = vals
		rec.Scans[a.name] = stats
		log.Printf("recovery: %s: %d of %d found in %d entries (%d bytes skipped, %d ambiguous)",
			a.name, stats.Found, len(want), stats.Entries, stats.Skipped, stats.Disagree)
	}

	// Decide each key again with every archive's value
	lost, err := os.Create(filepath.Join(out, "lost.log"))
	if err != nil {
		return err
	}
	defer lost.Close()
	txns := make([]*badger.Txn, len(x.archives))
	for i, a := range x.archives {
		txns[i] = a.db.NewTransaction(false)
		defer txns[i].Discard()
	}

	keys := make([]string, 0, len(misses))
	for k := range misses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	side, conf := new(leveldb.Batch), new(leveldb.Batch)
	for _, key := range keys {
		var vals [][]byte
		var from []int
		var held []string
		for i, a := range x.archives {
			if version, ok := misses[key][a.name]; ok {
				held = append(held, a.name)
				if v, ok := found[a.name][string(keyWithTs([]byte(key), version))]; ok {
					vals, from = append(vals, v), append(from, i)
				}
				continue
			}
			item, err := txns[i].Get([]byte(key))
			if errors.Is(err, badger.ErrKeyNotFound) {
				continue
			}
			if err != nil {
				return fmt.Errorf("%s: get %x: %w", a.name, key, err)
			}
			held = append(held, a.name)
			v, err := a.value(item)
			if err != nil {
				// Read fine during the walk, so this is not expected
				return fmt.Errorf("%s: value of %x: %w", a.name, key, err)
			}
			vals, from = append(vals, v), append(from, i)
		}

		side.Delete([]byte(key))
		for i := range x.archives {
			conf.Delete(append([]byte(key), byte(i)))
		}
		agree := true
		for _, v := range vals {
			agree = agree && bytes.Equal(v, vals[0])
		}
		switch {
		case len(vals) == 0:
			rec.Lost++
			if _, err := fmt.Fprintf(lost, "%x %s\n", key, strings.Join(held, ",")); err != nil {
				return err
			}
		case agree:
			side.Put([]byte(key), vals[0])
			rec.Extracted++
		default:
			for j, i := range from {
				conf.Put(append([]byte(key), byte(i)), vals[j])
			}
			rec.Conflicts++
		}

		if side.Len()+conf.Len() >= 50_000 {
			if err := x.writeBatches(side, conf); err != nil {
				return err
			}
		}
	}
	if err := x.writeBatches(side, conf); err != nil {
		return err
	}
	if err := lost.Sync(); err != nil {
		return err
	}

	rec.Done = true
	x.mu.Lock()
	defer x.mu.Unlock()
	x.prog.Recovery = rec
	return x.save()
}

func (x *extractor) writeBatches(side, conf *leveldb.Batch) error {
	if err := x.conflicts.Write(conf, &opt.WriteOptions{Sync: true}); err != nil {
		return err
	}
	if err := x.sidecar.Write(side, &opt.WriteOptions{Sync: true}); err != nil {
		return err
	}
	side.Reset()
	conf.Reset()
	return nil
}
