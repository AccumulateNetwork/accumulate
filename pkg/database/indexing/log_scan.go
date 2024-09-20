// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"iter"
	"slices"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
)

type logScanner[V any] Log[V]

type logScanFrom[V any] struct {
	log  *Log[V]
	from *record.Key
}

type logScanFromTo[V any] struct {
	log  *Log[V]
	from *record.Key
	to   *record.Key
}

func (x *Log[V]) Scan() *logScanner[V] {
	return (*logScanner[V])(x)
}

func (x *logScanner[V]) All() iter.Seq[QueryResult[V]] {
	return x.FromFirst().ToLast()
}

func (x *logScanner[V]) From(from *record.Key) *logScanFrom[V] {
	return &logScanFrom[V]{log: (*Log[V])(x), from: from}
}

func (x *logScanner[V]) FromFirst() *logScanFrom[V] {
	return x.From(nil)
}

func (x *logScanFrom[V]) To(to *record.Key) iter.Seq[QueryResult[V]] {
	s := &logScanFromTo[V]{log: x.log, from: x.from, to: to}
	return func(yield func(QueryResult[V]) bool) { s.scan(x.log.getHead(), true, yield) }
}

func (x *logScanFrom[V]) ToLast() iter.Seq[QueryResult[V]] {
	return x.To(nil)
}

func (s *logScanFromTo[V]) scan(block values.Value[*Block[V]], checkFrom bool, yield func(QueryResult[V]) bool) bool {
	b, err := block.Get()
	if err != nil {
		yield(errResult[V](err))
		return false
	}

	var i int
	if checkFrom && s.from != nil {
		var exact bool
		i, exact = slices.BinarySearchFunc(b.Entries, s.from, func(e *Entry[V], t *record.Key) int {
			return e.Key.Compare(t)
		})
		if i > 0 && !exact {
			i--
		}
	}

	if i >= len(b.Entries) {
		return true
	}

	for i, e := range b.Entries[i:] {
		if s.to != nil && e.Key.Compare(s.to) >= 0 {
			return true
		}
		if b.Level == 0 {
			if !yield(entry(e)) {
				return false
			}
		} else {
			if !s.scan(s.log.getBlock(b.Level-1, e.Index), i == 0, yield) {
				return false
			}
		}
	}

	return true
}
