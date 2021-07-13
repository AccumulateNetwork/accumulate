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
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var searchID = NewBytes32(
	"b0bb9c3e14514d7109b18b8edf3875618bac0881935e20a14b69d81fdc39e624")

// TestPendingEntries is not a stable test. It depends on the current pending
// entries. It could be improved with using static data, but it works for now.
// Some tests are only performed if the pending entries for the test are
// available. But all of these test cases have been executed and manually
// verified.
func TestPendingEntries(t *testing.T) {
	var pe PendingEntries
	c := NewClient()
	//c.Factomd.DebugRequest = true
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(pe.Get(nil, c))

	if len(pe) == 0 {
		return
	}

	//fmt.Printf("pe: %+v\n", pe)

	require.True(sort.SliceIsSorted(pe, func(i, j int) bool {
		var ci, cj []byte
		ei, ej := pe[i], pe[j]
		if ei.ChainID != nil {
			ci = ei.ChainID[:]
		}
		if ej.ChainID != nil {
			cj = ej.ChainID[:]
		}
		return bytes.Compare(ci, cj) > 0
	}), "not sorted")

	es := pe.Entries(&Bytes32{})
	if len(es) > 0 {
		assert.Nil(es[0].ChainID)
	}
	chainID := pe[len(pe)-1].ChainID
	if chainID != nil {
		es := pe.Entries(chainID)
		require.NotEmpty(es)
		for _, e := range es {
			assert.Equal(*e.ChainID, *chainID)
		}
	}

	es = pe.Entries(&searchID)
	if len(es) == 0 {
		return
	}
	//fmt.Printf("es: %+v\n", es)
	for _, e := range es {
		assert.Equal(*e.ChainID, searchID)
	}
}
