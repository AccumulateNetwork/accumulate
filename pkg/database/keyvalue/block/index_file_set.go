// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type indexFileSet struct {
	level    int
	children [16]indexFileNode
}

func newIndexFileSet(f *indexFile) (_ *indexFileSet, err error) {
	dir := filepath.Dir(f.file.Name())
	prefix := strings.TrimPrefix(filepath.Base(f.file.Name()), indexFilePrefix)
	set := new(indexFileSet)
	set.level = f.level
	defer closeIfError(&err, set)

	src := f.file.Acquire()
	defer src.Release()

	var j int
	var entry [64]byte
	var entryOk bool
	count := f.count.Load()

	for i := range set.children {
		name := fmt.Sprintf("%s%s%x", indexFilePrefix, prefix, i)
		g, err := newIndexFile(filepath.Join(dir, name), f.level+1)
		if err != nil {
			return nil, err
		}
		set.children[i] = g

		dst := g.file.Acquire()
		defer dst.Release()

		for j < int(count) {
			if !entryOk {
				entryOk = true
				j++
				_, err = src.Read(entry[:])
				if err != nil {
					return nil, err
				}
			}

			if indexFor(entry[:], f.level) == byte(i) {
				entryOk = false
				g.count.Add(1)
				_, err = dst.Write(entry[:])
				if err != nil {
					return nil, err
				}
			} else {
				break
			}
		}
	}

	return set, nil
}

func (s *indexFileSet) Close() error {
	var errs []error
	for _, n := range s.children {
		if n != nil {
			errs = append(errs, n.Close())
		}
	}
	return errors.Join(errs...)
}

func (s *indexFileSet) get(key [32]byte) (*recordLocation, bool) {
	i := indexFor(key[:], s.level)
	return s.children[i].get(key)
}

func (s *indexFileSet) forEach(fn func([32]byte, *recordLocation) error) error {
	for _, n := range s.children {
		err := n.forEach(fn)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *indexFileSet) commit(entries []*locationAndHash) (indexFileNode, error) {
	type Range struct {
		Index byte
		Start int
		End   int
	}

	var ranges []Range
	for i, e := range entries {
		x := indexFor(e.Hash[:], s.level)
		if len(ranges) == 0 || ranges[len(ranges)-1].Index != x {
			ranges = append(ranges, Range{Index: x, Start: i, End: i + 1})
		} else {
			ranges[len(ranges)-1].End = i + 1
		}
	}

	for _, r := range ranges {
		n, err := s.children[r.Index].commit(entries[r.Start:r.End])
		if err != nil {
			return nil, err
		}
		s.children[r.Index] = n
	}
	return s, nil
}

func indexFor(hash []byte, level int) byte {
	b := hash[level/2]
	if level%2 == 0 {
		return b >> 4
	}
	return b & 0xF
}
