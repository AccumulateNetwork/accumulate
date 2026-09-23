// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	badger "github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
)

func key(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }

func writeBadger(t *testing.T, path string, kv map[string]string) {
	db, err := badger.Open(badger.DefaultOptions(path).WithLogger(nil))
	require.NoError(t, err)
	wb := db.NewWriteBatch()
	for k, v := range kv {
		require.NoError(t, wb.Set(key(k), []byte(v)))
	}
	require.NoError(t, wb.Flush())
	require.NoError(t, db.Close())
}

func writeLevel(t *testing.T, path string, keys ...string) {
	db, err := leveldb.OpenFile(path, nil)
	require.NoError(t, err)
	for _, k := range keys {
		require.NoError(t, db.Put(key(k), []byte("current"), nil))
	}
	require.NoError(t, db.Close())
}

func readLevel(t *testing.T, path string) map[string]string {
	db, err := leveldb.OpenFile(path, nil)
	require.NoError(t, err)
	defer db.Close()
	m := map[string]string{}
	it := db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		m[hex.EncodeToString(it.Key())] = string(it.Value())
	}
	return m
}

func hx(k string, suffix ...byte) string { return hex.EncodeToString(append(key(k), suffix...)) }

func setup(t *testing.T) (dir string, current []string, archives []string) {
	dir = t.TempDir()
	writeBadger(t, filepath.Join(dir, "a"), map[string]string{
		"kept":     "kept-a", // current has it
		"only-a":   "a",
		"same":     "same",
		"disagree": "from-a",
	})
	writeBadger(t, filepath.Join(dir, "b"), map[string]string{
		"only-b":   "b",
		"same":     "same",
		"disagree": "from-b",
		"in-dn":    "b",
	})
	writeLevel(t, filepath.Join(dir, "bvnn"), "kept")
	writeLevel(t, filepath.Join(dir, "dnn"), "in-dn")
	current = []string{filepath.Join(dir, "bvnn"), filepath.Join(dir, "dnn")}
	archives = []string{"a=" + filepath.Join(dir, "a"), "b=" + filepath.Join(dir, "b")}
	return
}

func readProgress(t *testing.T, out string) *Progress {
	var prog Progress
	b, err := os.ReadFile(filepath.Join(out, "progress.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &prog))
	return &prog
}

func TestExtract(t *testing.T) {
	for _, shards := range []int{1, 3, 256} {
		t.Run(fmt.Sprint(shards), func(t *testing.T) {
			dir, current, archives := setup(t)
			out := filepath.Join(dir, "out")
			require.NoError(t, run(out, current, nil, archives, shards))

			// A key any current database holds is left out; a key the
			// archives agree on is written once; a key they disagree on goes
			// to conflicts, per archive
			require.Equal(t, map[string]string{
				hx("only-a"): "a",
				hx("only-b"): "b",
				hx("same"):   "same",
			}, readLevel(t, filepath.Join(out, "sidecar.db")))
			require.Equal(t, map[string]string{
				hx("disagree", 0): "from-a",
				hx("disagree", 1): "from-b",
			}, readLevel(t, filepath.Join(out, "conflicts.db")))

			prog := readProgress(t, out)
			require.True(t, prog.Done)
			require.EqualValues(t, 6, prog.Total.Keys)
			require.EqualValues(t, 2, prog.Total.Present)
			require.EqualValues(t, 3, prog.Total.Extracted)
			require.EqualValues(t, 1, prog.Total.Conflicts)
			require.Equal(t, map[string]int64{"a": 4, "b": 4}, prog.Total.Seen)
			require.Len(t, prog.Shards, shards)
			for _, s := range prog.Shards {
				require.True(t, s.Done)
			}

			// A finished run is not repeated
			require.NoError(t, run(out, current, nil, archives, shards))
		})
	}
}

func TestSplit(t *testing.T) {
	// The ranges tile the key space with no gap and no overlap
	for _, n := range []int{1, 2, 3, 32, 65536} {
		s := split(n)
		require.Len(t, s, n)
		require.Equal(t, "0000", s[0].Lo)
		require.Equal(t, "", s[n-1].Hi)
		for i := 1; i < n; i++ {
			require.Equal(t, s[i-1].Hi, s[i].Lo)
			require.Less(t, s[i-1].Lo, s[i].Lo)
		}
	}
}

func TestResume(t *testing.T) {
	dir, current, archives := setup(t)
	const shards = 3

	full := filepath.Join(dir, "full")
	require.NoError(t, run(full, current, nil, archives, shards))
	all := readLevel(t, filepath.Join(full, "sidecar.db"))

	// Every shard stopped after the smallest key it extracted
	prog := readProgress(t, full)
	prog.Done = false
	prog.Total = Counts{}
	want := map[string]string{}
	for k, v := range all {
		want[k] = v
	}
	for _, s := range prog.Shards {
		s.Done, s.LastKey, s.Counts = false, "", Counts{}
		for k := range all {
			if k >= s.Lo && (s.Hi == "" || k < s.Hi) && (s.LastKey == "" || k < s.LastKey) {
				s.LastKey = k
			}
		}
		delete(want, s.LastKey)
	}
	out := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(out, 0700))
	b, err := json.Marshal(prog)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(out, "progress.json"), b, 0644))

	// Resuming writes everything after those keys and nothing at or before them
	require.NoError(t, run(out, current, nil, archives, shards))
	require.Equal(t, want, readLevel(t, filepath.Join(out, "sidecar.db")))

	// Neither the archives nor the shards can change under a resume
	prog.Archives = []string{"b", "a"}
	b, _ = json.Marshal(prog)
	require.NoError(t, os.WriteFile(filepath.Join(out, "progress.json"), b, 0644))
	require.ErrorContains(t, run(out, current, nil, archives, shards), "progress was written for archives")
	prog.Archives = []string{"a", "b"}
	b, _ = json.Marshal(prog)
	require.NoError(t, os.WriteFile(filepath.Join(out, "progress.json"), b, 0644))
	require.ErrorContains(t, run(out, current, nil, archives, 4), "progress was written for 3 shards")
}

// corrupt overwrites the value log entry header of a key, as found in the
// pre-reorg dn archive: the pointer is in range, the bytes there do not decode.
func corrupt(t *testing.T, dir, k string) {
	files, err := filepath.Glob(filepath.Join(dir, "*.vlog"))
	require.NoError(t, err)
	for _, f := range files {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		i := bytes.Index(b, key(k))
		if i < 0 {
			continue
		}
		const header = 18 // klen, vlen, meta, user meta, expiresAt
		for j := i - header; j < i; j++ {
			b[j] = 0xff
		}
		require.NoError(t, os.WriteFile(f, b, 0600))
		return
	}
	t.Fatalf("%s is not in a value log", k)
}

func TestUnreadable(t *testing.T) {
	dir := t.TempDir()
	big := string(bytes.Repeat([]byte("v"), 100)) // above the value threshold, so in the log
	writeBadger(t, filepath.Join(dir, "a"), map[string]string{
		"ok":        "a",
		"both":      big,
		"only-a":    big + "a",
		"after-bad": "a",
	})
	writeBadger(t, filepath.Join(dir, "b"), map[string]string{
		"both": big,
	})
	corrupt(t, filepath.Join(dir, "a"), "both")
	corrupt(t, filepath.Join(dir, "a"), "only-a")
	writeLevel(t, filepath.Join(dir, "bvnn"))

	out := filepath.Join(dir, "out")
	require.NoError(t, run(out, []string{filepath.Join(dir, "bvnn")}, nil,
		[]string{"a=" + filepath.Join(dir, "a"), "b=" + filepath.Join(dir, "b")}, 4))

	// The readable archive decides a key another cannot read; a key no archive
	// can read is lost, not fatal, and both are listed
	require.Equal(t, map[string]string{
		hx("ok"):        "a",
		hx("both"):      big,
		hx("after-bad"): "a",
	}, readLevel(t, filepath.Join(out, "sidecar.db")))

	prog := readProgress(t, out)
	require.True(t, prog.Done)
	require.Equal(t, map[string]int64{"a": 2}, prog.Total.Unreadable)
	require.EqualValues(t, 1, prog.Total.Lost)

	list, err := os.ReadFile(filepath.Join(out, "unreadable.log"))
	require.NoError(t, err)
	require.Contains(t, string(list), "a "+hx("both"))
	require.Contains(t, string(list), "a "+hx("only-a"))
}
