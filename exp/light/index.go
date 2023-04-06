// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// FindEntry returns the index and value of the first entry for which the
// predicate is true, assuming it is false for some (possibly empty) prefix and
// true for the remainder of the entries.
func FindEntry[T any](entries []T, fn func(T) bool) (int, T, bool) {
	i := sort.Search(len(entries), func(i int) bool { return fn(entries[i]) })
	if i >= len(entries) {
		var z T
		return 0, z, false
	}
	return i, entries[i], true
}

// ByIndexSource searches for an index entry with [protocol.IndexEntry.Source]
// equal to or greater than the given value.
func ByIndexSource(source uint64) func(*protocol.IndexEntry) bool {
	return func(e *protocol.IndexEntry) bool { return e.Source >= source }
}

// ByIndexBlock searches for an index entry with
// [protocol.IndexEntry.BlockIndex] equal to or greater than the given value.
func ByIndexBlock(block uint64) func(*protocol.IndexEntry) bool {
	return func(e *protocol.IndexEntry) bool { return e.BlockIndex >= block }
}

// ByIndexRootIndexIndex searches for an index entry with
// [protocol.IndexEntry.RootIndexIndex] equal to or greater than the given
// value.
func ByIndexRootIndexIndex(rootIndex uint64) func(*protocol.IndexEntry) bool {
	return func(e *protocol.IndexEntry) bool { return e.RootIndexIndex >= rootIndex }
}

// ByAnchorBlock searches for an anchor entry with
// [protocol.AnchorMetadata.MinorBlockIndex] equal to or greater than the given
// value.
func ByAnchorBlock(block uint64) func(*AnchorMetadata) bool {
	return func(a *AnchorMetadata) bool { return a.Anchor.MinorBlockIndex >= block }
}
