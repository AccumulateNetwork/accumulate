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

func FindEntry[T any](entries []T, fn func(T) bool) (int, T, bool) {
	i := sort.Search(len(entries), func(i int) bool { return fn(entries[i]) })
	if i >= len(entries) {
		var z T
		return 0, z, false
	}
	return i, entries[i], true
}

func ByIndexSource(source uint64) func(*protocol.IndexEntry) bool {
	return func(e *protocol.IndexEntry) bool { return e.Source >= source }
}

func ByIndexBlock(block uint64) func(*protocol.IndexEntry) bool {
	return func(e *protocol.IndexEntry) bool { return e.BlockIndex >= block }
}

func ByIndexRootIndexIndex(rootIndex uint64) func(*protocol.IndexEntry) bool {
	return func(e *protocol.IndexEntry) bool { return e.RootIndexIndex >= rootIndex }
}

func ByAnchorBlock(block uint64) func(*AnchorMetadata) bool {
	return func(a *AnchorMetadata) bool { return a.Anchor.MinorBlockIndex >= block }
}
