// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	badger "github.com/dgraph-io/badger"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// recoverElements rebuilds lost chain element records. An element duplicates
// an entry of its mark point's hash list (or of the head's, for the open
// mark set), so its value is that entry, encoded as the store encodes an
// element. The encoding is not assumed: for each chain it must reproduce the
// stored bytes of every present element sampled around the lost one before
// any lost element of that chain is written.
func recoverElements(archiveArgs []string, identified, out string) error {
	var copies []*archive
	for _, arg := range archiveArgs {
		n, path, _ := strings.Cut(arg, "=")
		db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
		if err != nil {
			return fmt.Errorf("archive %s: %w", n, err)
		}
		copies = append(copies, &archive{name: n, db: db, log: &vlog{dir: path}})
	}
	store := &verifiedStore{copies}
	batch := coredb.New(store, nil).Begin(false)
	defer batch.Discard()

	// The store's raw bytes for a key, verified, from the first copy with them
	raw := func(key [32]byte) ([]byte, bool) {
		for _, a := range copies {
			txn := a.db.NewTransaction(false)
			moves := a.newMoves(txn)
			item, err := txn.Get(key[:])
			if err == nil {
				if v, err := a.resolve(item, moves); err == nil {
					moves.Close()
					txn.Discard()
					return v, true
				}
			}
			moves.Close()
			txn.Discard()
		}
		return nil, false
	}
	// Measured: the length, then the entry
	encode := func(entry []byte) []byte {
		return append(binary.AppendUvarint(nil, uint64(len(entry))), entry...)
	}

	dst, err := openOutput(out)
	if err != nil {
		return err
	}
	defer dst.Close()

	f, err := os.Open(identified)
	if err != nil {
		return err
	}
	defer f.Close()
	type result struct{ recovered, noSource, unproven int }
	results := map[string]*result{}
	proven := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) != 3 || fields[1] != "chain.element" {
			continue
		}
		// "<account> <chain> element <i> of <count>"
		d := strings.Fields(fields[2])
		acct, err := url.Parse(d[0])
		if err != nil {
			return err
		}
		name := d[1]
		idx, _ := strconv.ParseUint(d[3], 10, 64)
		r := results[name]
		if r == nil {
			r = new(result)
			results[name] = r
		}

		ch, err := batch.Account(acct).ChainByName(name)
		if err != nil {
			r.noSource++
			continue
		}
		m := ch.Inner()
		freq := uint64(m.MarkFreq())

		// The entry, from the mark point's hash list or the head's
		var entry []byte
		mark := idx/freq*freq + freq - 1
		if s, err := m.States(mark).Get(); err == nil && int(idx%freq) < len(s.HashList) {
			entry = s.HashList[idx%freq]
		} else if head, err := m.Head().Get(); err == nil && uint64(head.Count) <= mark && int(idx%freq) < len(head.HashList) {
			entry = head.HashList[idx%freq]
		}
		if entry == nil {
			r.noSource++
			continue
		}

		// Prove the encoding on this chain's present elements
		key := d[0] + " " + name
		if !proven[key] {
			var ok, bad int
			for j := idx - 64; j < idx+64 && ok < 16; j++ {
				if j == idx || j > idx+1<<62 {
					continue
				}
				stored, has := raw(m.Key().Append("Element", j).Hash())
				if !has {
					continue
				}
				e, err := ch.Entry(int64(j))
				if err != nil {
					continue
				}
				if bytes.Equal(stored, encode(e)) {
					ok++
				} else {
					bad++
				}
			}
			if ok < 4 || bad > 0 {
				r.unproven++
				continue
			}
			proven[key] = true
		}

		k, _ := hex.DecodeString(fields[0])
		if err := dst.Put(k, encode(entry), nil); err != nil {
			return err
		}
		r.recovered++
	}
	for name, r := range results {
		fmt.Printf("%-40s recovered %6d  no source %6d  encoding unproven %6d\n", name, r.recovered, r.noSource, r.unproven)
	}
	return sc.Err()
}
