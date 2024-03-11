// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
)

func (c *IndexDBPartitionAnchors) Produced() *IndexedList[*AnchorMetadata] {
	return &IndexedList[*AnchorMetadata]{c.getProduced()}
}

type IndexedList[T any] struct {
	values.List[T]
}

// Find returns the index and value of the first entry for which the predicate
// is true, assuming it is false for some (possibly empty) prefix and true for
// the remainder of the entries.
func (l *IndexedList[T]) Find(fn func(T) bool) (int, T, error) {
	entries, err := l.Get()
	if err != nil {
		var z T
		return 0, z, err
	}

	i, e, ok := FindEntry(entries, fn)
	if !ok {
		var z T
		return 0, z, (*database.NotFoundError)(l.Key().Append("(Find)"))
	}
	return i, e, nil
}

// Scan does a linear scan for the first element for which the predicate is
// true.
func (l *IndexedList[T]) Scan(fn func(T) bool) (int, T, error) {
	entries, err := l.Get()
	var z T
	if err != nil {
		return 0, z, err
	}

	for i, e := range entries {
		if fn(e) {
			return i, e, nil
		}
	}

	return 0, z, (*database.NotFoundError)(l.Key().Append("(Scan)"))
}
