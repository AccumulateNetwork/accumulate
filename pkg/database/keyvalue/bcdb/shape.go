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
// rewritten?" be answered by looking rather than by assuming: the store
// refuses a rewrite of a permanent record, the refusal is counted
// against its shape, and the shapes that get rewritten are then plainly
// visible.
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

// ShapeCount is what happened to the writes of one shape.
//
// Two counters, and nothing else.  An earlier version answered "was this
// key written again with different bytes" by remembering a digest of the
// last value written for every key -- which on a 500 tx/s soak was 192 MB,
// 38% of the live heap and the largest single consumer on the node, to
// produce a statistic (#4165).
//
// It was not needed.  The permanent layer REFUSES a rewrite, so the store
// already reports the thing that matters, exactly, for free, and across
// restarts -- which the in-memory digests never did.  Whatever the answer
// costs, it should not be a second index of the database.
type ShapeCount struct {
	// Layer is where isWriteOnce sends this shape: "perm" or "dyna".
	Layer string `json:"layer"`

	// Writes is every write of this shape.
	Writes uint64 `json:"writes"`

	// Misrouted counts the writes the permanent layer refused: a
	// permanent record written again with different bytes, which means
	// isWriteOnce classified this shape wrongly.  Non-zero on a perm
	// shape is the defect, and the first one is logged.
	Misrouted uint64 `json:"misrouted"`
}
