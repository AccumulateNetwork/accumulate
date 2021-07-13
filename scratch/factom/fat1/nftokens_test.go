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

package fat1

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newNFTokens(ids ...NFTokensSetter) NFTokens {
	maxCapacity = math.MaxInt64
	defer func() { maxCapacity = MaxCapacity }()
	nfTkns, err := NewNFTokens(ids...)
	if err != nil {
		panic(err)
	}
	return nfTkns
}

var NFTokensMarshalTests = []struct {
	Name   string
	NFTkns NFTokens
	Error  string
	JSON   string
}{{
	Name:   "valid, contiguous, expanded (0-7)",
	NFTkns: newNFTokens(NewNFTokenIDRange(0, 7)),
	JSON:   `[0,1,2,3,4,5,6,7]`,
}, {
	Name:   "valid, contiguous, condensed (0-12)",
	NFTkns: newNFTokens(NewNFTokenIDRange(0, 12)),
	JSON:   `[{"min":0,"max":12}]`,
}, {
	Name: "valid, disjoint (0-7, 9-20, 22, 100-10000, 10002)",
	NFTkns: newNFTokens(
		NewNFTokenIDRange(0, 7),
		NewNFTokenIDRange(9, 20),
		NFTokenID(22),
		NewNFTokenIDRange(1e2, 1e4),
		NFTokenID(1e4+2),
	),
	JSON: `[0,1,2,3,4,5,6,7,{"min":9,"max":20},22,{"min":100,"max":10000},10002]`,
}, {
	Name:   "invalid, empty",
	NFTkns: newNFTokens(),
	Error:  "fat1.NFTokens: empty",
}}

func TestNFTokensMarshal(t *testing.T) {
	for _, test := range NFTokensMarshalTests {
		t.Run(test.Name, func(t *testing.T) {
			assert := assert.New(t)
			data, err := test.NFTkns.MarshalJSON()
			if len(test.Error) > 0 {
				assert.EqualError(err, test.Error)
			} else {
				assert.Equal(test.JSON, string(data))
			}
		})
	}
}

var NFTokensUnmarshalTests = []struct {
	Name              string
	NFTkns            NFTokens
	Error             string
	JSON              string
	UnlimitedCapacity bool
}{{
	Name:   "valid, contiguous, expanded (0-7)",
	JSON:   `[0,1,2,3,4,5,6,7]`,
	NFTkns: newNFTokens(NewNFTokenIDRange(0, 7)),
	//}, {
	//	Name:   "valid, large, len 1000001",
	//	JSON:   `[{"min":0,"max":1000000}]`,
	//	NFTkns: newNFTokens(NewNFTokenIDRange(0, 1000000)),
	//}, {
	//	Name:   "valid, large, len 10000001",
	//	JSON:   `[{"min":0,"max":10000000}]`,
	//	NFTkns: newNFTokens(NewNFTokenIDRange(0, 10000000)),
	//}, {
	//	Name:   "valid, large, len 2000001",
	//	JSON:   `[{"min":0,"max":1000000}, {"min":1000001,"max":2000000}]`,
	//	NFTkns: newNFTokens(NewNFTokenIDRange(0, 1000000), NewNFTokenIDRange(1000001, 2000000)),
}, {
	Name:   "valid, large, len 100001",
	JSON:   `[{"min":0,"max":100000}]`,
	NFTkns: newNFTokens(NewNFTokenIDRange(0, 100000)),
}, {
	Name:   "valid, single",
	JSON:   `[0]`,
	NFTkns: newNFTokens(NewNFTokenIDRange(0)),
}, {
	Name:   "valid, single min/max",
	JSON:   `[{"min":0,"max":0}]`,
	NFTkns: newNFTokens(NewNFTokenIDRange(0)),
}, {
	Name:   "valid, single min/max",
	JSON:   `[{"min":9,"max":9}]`,
	NFTkns: newNFTokens(NewNFTokenIDRange(9)),
}, {
	Name: "valid, large, len 200001",
	JSON: `[{"min":0,"max":100000},{"min":100001,"max":200000}]`,
	NFTkns: newNFTokens(NewNFTokenIDRange(0, 100000),
		NewNFTokenIDRange(100001, 200000)),
}, {
	Name: "valid, large, len 300002",
	JSON: `[{"min":0,"max":100000},1000000000,{"min":100001,"max":200000},{"min":200001,"max":300000}]`,
	NFTkns: newNFTokens(NewNFTokenIDRange(0, 100000),
		NewNFTokenIDRange(100001, 200000),
		NewNFTokenIDRange(200001, 300000),
		NFTokenID(1000000000)),
}, {
	Name:   "valid, completely disjoint, len 500000",
	JSON:   disjoint1000TknsJSON,
	NFTkns: disjoint1000Tkns,
}, {
	Name:              "valid, contiguous, len 500000",
	JSON:              `[{"min":1,"max":500000}]`,
	NFTkns:            newNFTokens(NewNFTokenIDRange(1, 500000)),
	UnlimitedCapacity: true,
}, {
	Name:   "valid, contiguous, expanded, out of order (0-7)",
	JSON:   `[7,0,1,4,3,2,5,6]`,
	NFTkns: newNFTokens(NewNFTokenIDRange(0, 7)),
}, {
	Name:   "valid, contiguous, expanded, out of order ranges (0-7)",
	JSON:   `[7,0,1,4,3,2,5,6]`,
	NFTkns: newNFTokens(NewNFTokenIDRange(0, 7)),
}, {
	Name:   "valid, contiguous, condensed (0-12)",
	JSON:   `[{"min":0,"max":12}]`,
	NFTkns: newNFTokens(NewNFTokenIDRange(0, 12)),
}, {
	Name: "valid, disjoint (0-7, 9-20, 22, 100-10000, 10002)",
	JSON: `[0,1,2,3,4,5,6,7,{"min":9,"max":20},22,{"min":100,"max":10000},10002]`,
	NFTkns: newNFTokens(
		NewNFTokenIDRange(0, 7),
		NewNFTokenIDRange(9, 20),
		NFTokenID(22),
		NewNFTokenIDRange(1e2, 1e4),
		NFTokenID(1e4+2),
	),
}, {
	Name: "valid, disjoint, out of order (0-7, 9-20, 22, 100-10000, 10002)",
	JSON: `[0,{"min":9,"max":20},22,{"min":100,"max":10000},10002,1,2,3,4,5,6,7]`,
	NFTkns: newNFTokens(
		NewNFTokenIDRange(),
		NewNFTokenIDRange(1, 7),
		NewNFTokenIDRange(9, 20),
		NFTokenID(22),
		NewNFTokenIDRange(1e2, 1e4),
		NFTokenID(1e4+2),
	),
}, {
	Name: "valid, disjoint, out of order, inefficient (0-7, 9-20, 22, 100-10000, 10002)",
	JSON: `[0,{"min":9,"max":20},22,{"min":100,"max":10000},{"min":10002,"max":10002},1,2,3,4,5,6,7]`,
	NFTkns: newNFTokens(
		NewNFTokenIDRange(0, 7),
		NewNFTokenIDRange(9, 20),
		NFTokenID(22),
		NewNFTokenIDRange(1e2, 1e4),
		NFTokenID(1e4+2),
	),
}, {
	Name:  "invalid, empty",
	JSON:  `[]`,
	Error: "*fat1.NFTokens: empty",
}, {
	Name:  "invalid, min greater than max",
	JSON:  `[{"min":900,"max":0}]`,
	Error: "*fat1.NFTokens: *fat1.NFTokenIDRange: Min is greater than Max",
}, {
	Name:  "invalid, duplicates",
	JSON:  `[0,0]`,
	Error: "*fat1.NFTokens: duplicate NFTokenID: 0",
}, {
	Name:  "invalid, duplicates, overlapping ranges",
	JSON:  `[{"min":0,"max":7},{"min":6,"max":10}]`,
	Error: "*fat1.NFTokens: duplicate NFTokenID: 6",
}, {
	Name:  "invalid, duplicates, overlapping ranges",
	JSON:  `[{"min":0,"max":7},{"min":6,"max":10}]`,
	Error: "*fat1.NFTokens: duplicate NFTokenID: 6",
}, {
	Name:  "invalid, invalid range",
	JSON:  `[{"min":5,"max":0},{"min":6,"max":10}]`,
	Error: "*fat1.NFTokens: *fat1.NFTokenIDRange: Min is greater than Max",
}, {
	Name:  "invalid, malformed JSON",
	JSON:  `[{"min":5,"max":10},{"min":6,"max":10]]`,
	Error: "*fat1.NFTokens: invalid character ']' after object key:value pair",
}, {
	Name:  "invalid, NFTokenID JSON type",
	JSON:  `["hello",{"min":6,"max":10}]`,
	Error: "*fat1.NFTokens: json: cannot unmarshal string into Go value of type fat1.NFTokenID",
}, {
	Name:  "invalid, NFTokenIDRange.Min JSON type",
	JSON:  `[{"min":{},"max":10}]`,
	Error: "*fat1.NFTokens: *fat1.NFTokenIDRange: json: cannot unmarshal object into Go struct field nfTokenIDRange.min of type fat1.NFTokenID",
}, {
	Name:  "invalid, duplicate JSON keys",
	JSON:  `[{"min":6,"max":10,"max":11}]`,
	Error: "*fat1.NFTokens: *fat1.NFTokenIDRange: unexpected JSON length",
}, {
	Name:  "invalid, contiguous, len 500000",
	JSON:  `[{"min":1,"max":500000}]`,
	Error: "*fat1.NFTokens: *fat1.NFTokenIDRange: NFTokenID max capacity (400000) exceeded",
}}

func TestNFTokensUnmarshal(t *testing.T) {
	for _, test := range NFTokensUnmarshalTests {
		t.Run(test.Name, func(t *testing.T) {
			assert := assert.New(t)
			nfTkns, _ := NewNFTokens()
			if test.UnlimitedCapacity {
				maxCapacity = math.MaxInt64
				defer func() { maxCapacity = MaxCapacity }()
			}
			err := nfTkns.UnmarshalJSON([]byte(test.JSON))
			if len(test.Error) > 0 {
				assert.EqualError(err, test.Error)
			} else {
				assert.Equal(test.NFTkns, nfTkns)
			}
		})
	}
}

var Tkns NFTokens

func BenchmarkNFTokensUnmarshal(b *testing.B) {
	maxCapacity = math.MaxInt64
	defer func() { maxCapacity = MaxCapacity }()
	for _, test := range NFTokensUnmarshalTests {
		if test.Name[:len("invalid")] == "invalid" {
			break
		}
		b.Run(test.Name, func(b *testing.B) {
			var nfTkns NFTokens
			for i := 0; i < b.N; i++ {
				nfTkns, _ = NewNFTokens()
				nfTkns.UnmarshalJSON([]byte(test.JSON))
			}
			Tkns = nfTkns
		})
	}
}

func TestNewNFTokens(t *testing.T) {
	var nfTknID NFTokenID
	_, err := NewNFTokens(nfTknID, nfTknID)
	assert.EqualError(t, err, "duplicate NFTokenID: 0")
}

func TestNFTokenIDRangeMarshal(t *testing.T) {
	idRange := NFTokenIDRange{Min: 5}
	_, err := json.Marshal(idRange)
	assert.EqualError(t, err, "json: error calling MarshalJSON for type fat1.NFTokenIDRange: Min is greater than Max")
}

func TestNFTokensIntersect(t *testing.T) {
	nfTkns1 := newNFTokens(NewNFTokenIDRange(0, 5))
	nfTkns2 := newNFTokens(NFTokenID(5))
	nfTkns3 := newNFTokens(NewNFTokenIDRange(6, 8))
	assert := assert.New(t)
	assert.EqualError(nfTkns1.NoIntersection(nfTkns2), "duplicate NFTokenID: 5")
	assert.EqualError(nfTkns2.NoIntersection(nfTkns1), "duplicate NFTokenID: 5")
	assert.NoError(nfTkns1.NoIntersection(nfTkns3))
}
