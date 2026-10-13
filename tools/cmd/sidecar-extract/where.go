// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// stores is where a record may be: the live databases, the sidecar, and the
// conflicts store, whose keys are the record key followed by the index of the
// archive that held the value.
type stores struct {
	live      []*leveldb.DB
	sidecar   *leveldb.DB
	conflicts *leveldb.DB
	archives  []string
}

// holding is every store that holds one record. A reader sees the first, in
// the order live, sidecar; the rest say what the sidecar and the conflicts
// store carry beneath it.
type holding struct {
	in     string            // live, sidecar, conflicts, none — the first holder
	all    []string          // every holder
	value  []byte            // the first holder's value
	values map[string][]byte // from conflicts, by archive
}

func (s *stores) locate(key *database.Key) holding {
	h := key.Hash()
	var out holding
	for _, db := range s.live {
		if v, err := db.Get(h[:], nil); err == nil {
			out.all = append(out.all, "live")
			out.value = v
			break
		}
	}
	if v, err := s.sidecar.Get(h[:], nil); err == nil {
		out.all = append(out.all, "sidecar")
		if out.value == nil {
			out.value = v
		}
	}
	if s.conflicts != nil {
		out.values = s.conflictValues(h)
		if len(out.values) > 0 {
			out.all = append(out.all, "conflicts")
		}
	}
	if len(out.all) == 0 {
		return holding{in: "none"}
	}
	out.in = out.all[0]
	return out
}

func (s *stores) conflictValues(h [32]byte) map[string][]byte {
	it := s.conflicts.NewIterator(util.BytesPrefix(h[:]), nil)
	defer it.Release()
	values := map[string][]byte{}
	for it.Next() {
		k := it.Key()
		if len(k) != len(h)+1 {
			continue
		}
		name := fmt.Sprint(k[len(h)])
		if i := int(k[len(h)]); i < len(s.archives) {
			name = s.archives[i]
		}
		values[name] = append([]byte(nil), it.Value()...)
	}
	return values
}

// String names the holder and, for a conflict, the archives that hold the
// record, grouped by value.
func (h holding) String() string {
	if h.in == "none" {
		return h.in
	}
	var parts []string
	for _, in := range h.all {
		if in == "conflicts" {
			parts = append(parts, h.conflictString())
		} else {
			parts = append(parts, in)
		}
	}
	return strings.Join(parts, "+")
}

func (h holding) conflictString() string {
	byValue := map[string][]string{}
	for name, v := range h.values {
		k := hex.EncodeToString(v)
		byValue[k] = append(byValue[k], name)
	}
	var groups []string
	for _, names := range byValue {
		sort.Strings(names)
		groups = append(groups, strings.Join(names, "+"))
	}
	sort.Strings(groups)
	return "conflicts[" + strings.Join(groups, " | ") + "]"
}

func uintOf(v []byte) string {
	n, k := binary.Uvarint(v)
	if k <= 0 {
		return "0x" + hex.EncodeToString(v)
	}
	return fmt.Sprint(n)
}

// where reports, for each account, which store holds its main state, its main
// chain head, and each record of its data-entry index — the count, the entries
// and the entry-to-transaction map (#4268): the live databases, the sidecar,
// the conflicts store (and which archives' values), or none.
func where(currentPaths []string, sidecarPath, conflictsPath string, accounts []string) error {
	var s stores
	for _, p := range currentPaths {
		db, err := leveldb.OpenFile(p, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
		if err != nil {
			return fmt.Errorf("current %s: %w", p, err)
		}
		defer db.Close()
		s.live = append(s.live, db)
	}
	var err error
	s.sidecar, err = leveldb.OpenFile(sidecarPath, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return fmt.Errorf("sidecar: %w", err)
	}
	defer s.sidecar.Close()
	if conflictsPath != "" {
		s.conflicts, err = leveldb.OpenFile(conflictsPath, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
		if err != nil {
			return fmt.Errorf("conflicts: %w", err)
		}
		defer s.conflicts.Close()
		b, err := os.ReadFile(filepath.Join(filepath.Dir(conflictsPath), "progress.json"))
		if err == nil {
			var p Progress
			if json.Unmarshal(b, &p) == nil {
				s.archives = p.Archives
			}
		}
	}

	// The batch only derives keys
	batch := coredb.New(memory.New(nil), nil).Begin(false)
	defer batch.Discard()

	for _, a := range accounts {
		u, err := url.Parse(a)
		if err != nil {
			return err
		}
		acct := batch.Account(u)
		fmt.Printf("%s\n", u)
		fmt.Printf("\tmain state %s: %s\n", hashOf(acct.Main().Key()), s.locate(acct.Main().Key()))
		fmt.Printf("\tmain chain head %s: %s\n", hashOf(acct.MainChain().Head().Key()), s.locate(acct.MainChain().Head().Key()))

		countKey := acct.Data().Entry().Key()
		count := s.locate(countKey)
		fmt.Printf("\tdata count %s: %s", hashOf(countKey), count)
		var n uint64
		switch count.in {
		case "live", "sidecar":
			fmt.Printf(" = %s", uintOf(count.value))
			n, _ = binary.Uvarint(count.value)
		case "conflicts":
			var names []string
			for name := range count.values {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				fmt.Printf(" %s=%s", name, uintOf(count.values[name]))
				if m, _ := binary.Uvarint(count.values[name]); m > n {
					n = m
				}
			}
		}
		fmt.Println()

		entries := map[string]int{}
		txns := map[string]int{}
		var missing []uint64
		for i := uint64(0); i < n; i++ {
			// the model appends the index as an int (values.Counted)
			e := s.locate(countKey.Append(int(i)))
			entries[e.String()]++
			if e.in == "none" {
				missing = append(missing, i)
				continue
			}
			// the transaction of the entry, for whichever value the entry has
			var hashes [][]byte
			if e.value != nil {
				hashes = append(hashes, e.value)
			}
			for _, v := range e.values {
				hashes = append(hashes, v)
			}
			seen := map[string]bool{}
			for _, h := range hashes {
				if len(h) != 32 || seen[string(h)] {
					continue
				}
				seen[string(h)] = true
				txns[s.locate(acct.Data().Transaction([32]byte(h)).Key()).String()]++
			}
		}
		fmt.Printf("\t%d entries: %s\n", n, tally(entries))
		if len(missing) > 0 {
			fmt.Printf("\tentries in no store: %d (first %d)\n", len(missing), missing[0])
		}
		fmt.Printf("\tentry transactions: %s\n", tally(txns))
	}
	return nil
}

func tally(m map[string]int) string {
	if len(m) == 0 {
		return "-"
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] })
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

func hashOf(key *database.Key) string {
	h := key.Hash()
	return hex.EncodeToString(h[:])
}
