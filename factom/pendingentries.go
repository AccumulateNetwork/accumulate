// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package factom

import (
	"bytes"
	"context"
	"sort"
)

// PendingEntries is a list of pending entries which may or may not be
// revealed. If the entry's ChainID is not nil, then its data has been revealed
// and can be queried from factomd.
type PendingEntries []Entry

// Get returns all pending entries sorted by descending ChainID, and then order
// they were originally returned. Pending Entries that are committed but not
// revealed have a nil ChainID and are at the end of the pe slice.
func (pe *PendingEntries) Get(ctx context.Context, c *Client) error {
	if err := c.FactomdRequest(ctx, "pending-entries", nil, pe); err != nil {
		return err
	}
	sort.SliceStable(*pe, func(i, j int) bool {
		pe := *pe
		var ci, cj []byte
		ei, ej := pe[i], pe[j]
		if ei.ChainID != nil {
			ci = ei.ChainID[:]
		}
		if ej.ChainID != nil {
			cj = ej.ChainID[:]
		}
		return bytes.Compare(ci, cj) > 0
	})
	return nil
}

// Entries efficiently finds and returns all entries in pe for the given
// chainID, if any exist. Otherwise, Entries returns nil.
func (pe PendingEntries) Entries(chainID *Bytes32) []Entry {
	var cID []byte
	if chainID != nil {
		cID = chainID[:]
	}
	// Find the first index of the entry with this chainID.
	ei := sort.Search(len(pe), func(i int) bool {
		var c []byte
		e := pe[i]
		if e.ChainID != nil {
			c = e.ChainID[:]
		}
		return bytes.Compare(c, cID) <= 0
	})
	if chainID == nil {
		// Unrevealed entries with no ChainID are all at the end, if any.
		return pe[ei:]
	}
	if ei == len(pe) || // (None are true) OR
		// (We found nil OR we did not find an exact match)
		(pe[ei].ChainID == nil || *pe[ei].ChainID != *chainID) {
		// There are no entries for this ChainID.
		return nil
	}
	// Find all remaining entries with the chainID.
	for i, e := range pe[ei:] {
		if e.ChainID == nil || *e.ChainID != *chainID {
			return pe[ei : ei+i]
		}
	}
	return pe[ei:]
}
