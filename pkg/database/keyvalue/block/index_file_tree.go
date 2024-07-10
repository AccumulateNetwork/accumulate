// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

const indexFileEntrySize = 64
const indexFileEntryCount = 1 << 10
const indexFileSize = indexFileEntrySize * indexFileEntryCount
const indexFilePrefix = "index-"

type indexFileTree struct {
	mu     *sync.RWMutex
	config *config
	root   indexFileNode
}

type indexFileNode interface {
	Close() error
	get(key [32]byte) (*recordLocation, bool)
	forEach(fn func([32]byte, *recordLocation) error) error
	commit(centries []*locationAndHash) (indexFileNode, error)
}

type emptyIndexFileTree string

type locationAndHash struct {
	Hash     [32]byte
	Location *recordLocation
}

var reHex = regexp.MustCompile(`^-?[0-9a-f]*`)

func openIndexFileTree(cfg *config) (_ *indexFileTree, err error) {
	s := new(indexFileTree)
	s.mu = new(sync.RWMutex)
	s.config = cfg
	defer closeIfError(&err, s)

	entries, err := os.ReadDir(cfg.path)
	switch {
	case err == nil,
		errors.Is(err, fs.ErrNotExist):
		// Directory exists, or doesn't

	default:
		// Some other error
		return nil, err
	}

	emap := map[string]bool{}
	for _, e := range entries {
		s := strings.TrimPrefix(e.Name(), indexFilePrefix)
		if s == e.Name() || !reHex.MatchString(s) {
			continue
		}
		emap[s] = true
	}

	s.root, err = openIndexFileNode(cfg.path, 0, emap)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func openIndexFileNode(dir string, level int, files map[string]bool) (indexFileNode, error) {
	if level == 0 && len(files) == 0 {
		return emptyIndexFileTree(dir), nil
	}

	if len(files) == 1 {
		var name string
		for name = range files {
		}
		return openIndexFile(filepath.Join(dir, indexFilePrefix+name), level)
	}

	if len(files) < 16 {
		return nil, errors.InternalError.With("corrupted index")
	}

	// Partition the file list
	parts := [16]map[string]bool{}
	for i := range parts {
		parts[i] = map[string]bool{}
	}
	for file := range files {
		// If there's an old index file hanging around that didn't get deleted
		// somehow, just ignore it
		if len(file) <= level {
			continue
		}

		i, err := strconv.ParseUint(file[level:level+1], 16, 4)
		if err != nil {
			panic("internal error: invalid file name")
		}
		parts[i][file] = true
	}

	s := &indexFileSet{level: level}
	for i, files := range parts {
		var err error
		s.children[i], err = openIndexFileNode(dir, level+1, files)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *indexFileTree) Close() error {
	if s == nil || s.root == nil {
		return nil
	}
	return s.root.Close()
}

func (s *indexFileTree) Get(key [32]byte) (*recordLocation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.root.get(key)
}

func (s *indexFileTree) ForEach(fn func([32]byte, *recordLocation) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.root.forEach(fn)
}

func (s *indexFileTree) Commit(entries map[[32]byte]*recordLocation) error {
	if len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]*locationAndHash, 0, len(entries))
	for hash, loc := range entries {
		if loc.Block == nil {
			panic("invalid location: missing block")
		}
		list = append(list, &locationAndHash{Hash: hash, Location: loc})
	}

	slices.SortFunc(list, func(a, b *locationAndHash) int {
		return bytes.Compare(a.Hash[:], b.Hash[:])
	})

	n, err := s.root.commit(list)
	if err != nil {
		return err
	}

	s.root = n
	return nil
}

func (emptyIndexFileTree) Close() error                                           { return nil }
func (emptyIndexFileTree) get(key [32]byte) (*recordLocation, bool)               { return nil, false }
func (emptyIndexFileTree) forEach(fn func([32]byte, *recordLocation) error) error { return nil }

func (e emptyIndexFileTree) commit(entries []*locationAndHash) (indexFileNode, error) {
	f, err := newIndexFile(filepath.Join(string(e), indexFilePrefix), 0)
	if err != nil {
		return nil, err
	}
	return f.commit(entries)
}
