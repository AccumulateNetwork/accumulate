// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"

	badger "github.com/dgraph-io/badger"
)

// Badger v1 does not verify a value it reads from its value log: it slices the
// bytes its index points at and returns them. In an archive whose index points
// at log offsets that were later overwritten — the pre-reorg dn archive does —
// that returns another record's bytes, or panics. So values are read here
// instead, and accepted only when the log entry's checksum holds and the entry
// is for exactly the key and version the index names.

const (
	headerSize      = 18 // klen, vlen, expiresAt, meta, user meta
	bitValuePointer = 1 << 1
)

var (
	badgerMove = []byte("!badger!move")
	castagnoli = crc32.MakeTable(crc32.Castagnoli)
)

// keyWithTs is Badger's internal key: the key and its inverted version.
func keyWithTs(key []byte, version uint64) []byte {
	out := make([]byte, len(key)+8)
	copy(out, key)
	binary.BigEndian.PutUint64(out[len(key):], math.MaxUint64-version)
	return out
}

type vlog struct {
	dir   string
	mu    sync.Mutex
	files map[uint32]*os.File
}

func (l *vlog) file(fid uint32) (*os.File, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if f, ok := l.files[fid]; ok {
		return f, nil
	}
	f, err := os.Open(filepath.Join(l.dir, fmt.Sprintf("%06d.vlog", fid)))
	if err != nil {
		return nil, err
	}
	if l.files == nil {
		l.files = map[uint32]*os.File{}
	}
	l.files[fid] = f
	return f, nil
}

// value returns an item's value, verified. The item's meta and value pointer
// are unexported; reflection reads them without modifying anything.
func (a *archive) value(item *badger.Item) ([]byte, error) {
	rv := reflect.ValueOf(item).Elem()
	meta := byte(rv.FieldByName("meta").Uint())
	vptr := rv.FieldByName("vptr").Bytes()
	if meta&bitValuePointer == 0 {
		// Stored in the index, not the log
		return append([]byte{}, vptr...), nil
	}
	if len(vptr) < 12 {
		return nil, fmt.Errorf("value pointer is %d bytes", len(vptr))
	}
	fid := binary.BigEndian.Uint32(vptr[0:4])
	size := binary.BigEndian.Uint32(vptr[4:8])
	offset := binary.BigEndian.Uint32(vptr[8:12])

	f, err := a.log.file(fid)
	if errors.Is(err, fs.ErrNotExist) {
		// Garbage collection moved it; recovery finds the move entry
		return nil, fmt.Errorf("vlog %d was collected", fid)
	}
	if err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	if _, err := f.ReadAt(buf, int64(offset)); err != nil {
		return nil, fmt.Errorf("vlog %d offset %d: %w", fid, offset, err)
	}
	k, v, ok := decodeEntry(buf)
	if !ok || len(k) != int(size)-headerSize-len(v)-crc32.Size {
		return nil, fmt.Errorf("vlog %d offset %d: not an intact entry", fid, offset)
	}
	want := keyWithTs(item.Key(), item.Version())
	if !bytes.Equal(k, want) && !bytes.Equal(k, append(badgerMove[:len(badgerMove):len(badgerMove)], want...)) {
		return nil, fmt.Errorf("vlog %d offset %d: entry is for another key", fid, offset)
	}
	return append([]byte{}, v...), nil
}

// decodeEntry decodes the log entry at the start of b, if b starts with an
// intact one.
func decodeEntry(b []byte) (key, value []byte, ok bool) {
	if len(b) < headerSize+crc32.Size {
		return nil, nil, false
	}
	klen := binary.BigEndian.Uint32(b[0:4])
	vlen := binary.BigEndian.Uint32(b[4:8])
	if klen > 1<<16 || uint64(len(b)) < uint64(headerSize)+uint64(klen)+uint64(vlen)+crc32.Size {
		return nil, nil, false
	}
	end := headerSize + int(klen) + int(vlen)
	if crc32.Checksum(b[:end], castagnoli) != binary.BigEndian.Uint32(b[end:]) {
		return nil, nil, false
	}
	return b[headerSize : headerSize+klen], b[headerSize+klen : end], true
}

// scanStats is what a scan of one archive's log found.
type scanStats struct {
	Files    int   `json:"files"`
	Entries  int64 `json:"entries"`
	Skipped  int64 `json:"skipped"` // bytes that are not an intact entry
	Found    int   `json:"found"`
	Disagree int   `json:"disagree"` // wanted keys found twice with different values; not used
}

// scan reads every intact entry of an archive's value log and returns the
// values of the wanted internal keys (key and version), found under the key
// itself or under Badger's move prefix. A region that is not an intact entry
// is stepped over a byte at a time, so entries after damage are still found. A
// key found with two different values is dropped: neither can be trusted.
func (a *archive) scan(want map[string]bool, workers int) (map[string][]byte, *scanStats, error) {
	paths, err := filepath.Glob(filepath.Join(a.log.dir, "*.vlog"))
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(paths)

	var mu sync.Mutex
	found := map[string][]byte{}
	disagree := map[string]bool{}
	stats := &scanStats{Files: len(paths)}
	jobs := make(chan string)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			var err error
			for path := range jobs {
				if err != nil {
					continue
				}
				var b []byte
				b, err = os.ReadFile(path)
				if err != nil {
					continue
				}
				var entries, skipped int64
				for off := 0; off < len(b); {
					k, v, ok := decodeEntry(b[off:])
					if !ok {
						off++
						skipped++
						continue
					}
					entries++
					off += headerSize + len(k) + len(v) + crc32.Size
					k = bytes.TrimPrefix(k, badgerMove)
					if !want[string(k)] {
						continue
					}
					mu.Lock()
					if prev, ok := found[string(k)]; !ok {
						found[string(k)] = append([]byte{}, v...)
						stats.Found++
					} else if !bytes.Equal(prev, v) {
						disagree[string(k)] = true
					}
					mu.Unlock()
				}
				mu.Lock()
				stats.Entries += entries
				stats.Skipped += skipped
				mu.Unlock()
			}
			errs <- err
		}()
	}
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)
	for i := 0; i < workers; i++ {
		if e := <-errs; e != nil && err == nil {
			err = e
		}
	}
	for k := range disagree {
		delete(found, k)
	}
	stats.Found -= len(disagree)
	stats.Disagree = len(disagree)
	return found, stats, err
}
