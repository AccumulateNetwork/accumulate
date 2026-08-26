// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// A record key is a path -- ("Account", url, "MainChain", "Element", 5)
// -- and what decides whether its value can ever change is the path's
// shape, not the particular account or index it names.  Shape strips
// the variable parts and keeps the names, so every chain element in the
// database collapses to one bucket:
//
//	Account.(url).MainChain.Element.(uint)
//
// This is what lets the question "is legitimate blockchain data being
// rewritten?" be answered by looking rather than by assuming: a rewrite
// is counted against its shape, and the shapes that get rewritten are
// then plainly visible.
func keyShape(k *record.Key) string {
	if k == nil || k.Len() == 0 {
		return "(empty)"
	}
	parts := make([]string, k.Len())
	for i := 0; i < k.Len(); i++ {
		parts[i] = shapeOf(k.Get(i))
	}
	return strings.Join(parts, ".")
}

// shapeOf keeps a literal string -- those are the record names the
// model defines -- and reduces everything else to its kind
func shapeOf(v any) string {
	switch v := v.(type) {
	case nil:
		return "(nil)"
	case string:
		return v
	case []byte:
		return "(bytes)"
	case [32]byte, *[32]byte:
		return "(hash)"
	case uint, uint8, uint16, uint32, uint64,
		int, int8, int16, int32, int64:
		return "(int)"
	case interface{ AccountID() []byte }:
		return "(url)"
	case interface{ Bytes() []byte }:
		return "(id)"
	default:
		_ = v
		return "(other)"
	}
}

// ShapeCount is what happened to the writes of one shape
type ShapeCount struct {
	// Layer is where isWriteOnce sends this shape: "perm" or "dyna".
	// Reading it beside Rewritten is the check -- a perm shape with a
	// non-zero Rewritten is a classification that is wrong.
	Layer string `json:"layer"`

	New       uint64 `json:"new"`       // The key had not been written before
	Duplicate uint64 `json:"duplicate"` // Written again with the same bytes
	Rewritten uint64 `json:"rewritten"` // Written again with different bytes

	// Misrouted counts the writes the permanent layer refused, which
	// is the same event as Rewritten seen from the store's side.  They
	// can differ: the tally sees a rewrite the first time a key is
	// written twice in one process, the store sees it across restarts
	// too, having the previous value on disk.
	Misrouted uint64 `json:"misrouted"`
}
