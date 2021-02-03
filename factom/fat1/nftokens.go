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
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const MaxCapacity = 4e5

var maxCapacity int = MaxCapacity

var ErrorCapacity = fmt.Errorf("NFTokenID max capacity (%v) exceeded", maxCapacity)

// NFTokens are a set of unique NFTokenIDs. A map[NFTokenID]struct{} is used to
// guarantee uniqueness of NFTokenIDs.
type NFTokens map[NFTokenID]struct{}

type nfTokensSetter interface {
	setInto(tkns NFTokens) error
}

func (tkns NFTokens) setInto(to NFTokens) error {
	if len(tkns)+len(to) > maxCapacity {
		return ErrorCapacity
	}
	for tknID := range tkns {
		if _, ok := to[tknID]; ok {
			return errorNFTokenIDIntersection(tknID)
		}
		to[tknID] = struct{}{}
	}
	return nil
}

func (tkns NFTokens) set(ids ...nfTokensSetter) error {
	for _, id := range ids {
		if err := id.setInto(tkns); err != nil {
			return err
		}
	}
	return nil
}

type errorNFTokenIDIntersection NFTokenID

func (id errorNFTokenIDIntersection) Error() string {
	return fmt.Sprintf("duplicate NFTokenID: %v", NFTokenID(id))
}

type errorMissingNFTokenID NFTokenID

func (id errorMissingNFTokenID) Error() string {
	return fmt.Sprintf("missing NFTokenID: %v", NFTokenID(id))
}

func (tkns NFTokens) ContainsAll(tknsSub NFTokens) error {
	if len(tknsSub) > len(tkns) {
		return fmt.Errorf("cannot contain a bigger NFTokens set")
	}
	for tknID := range tknsSub {
		if _, ok := tkns[tknID]; !ok {
			return errorMissingNFTokenID(tknID)
		}
	}
	return nil
}

// Slice returns a sorted slice of tkns' NFTokenIDs.
func (tkns NFTokens) Slice() []NFTokenID {
	tknsAry := make([]NFTokenID, len(tkns))
	i := 0
	for tknID := range tkns {
		tknsAry[i] = tknID
		i++
	}
	sort.Slice(tknsAry, func(i, j int) bool {
		return tknsAry[i] < tknsAry[j]
	})
	return tknsAry
}

func (tkns NFTokens) MarshalJSON() ([]byte, error) {
	if len(tkns) == 0 {
		return []byte(`[]`), nil
	}
	tknsAry := tkns.compress(true)

	return json.Marshal(tknsAry)
}

func (tkns NFTokens) String() string {
	if len(tkns) == 0 {
		return "[]"
	}

	tknsAry := tkns.compress(false)

	str := "["
	for _, tkn := range tknsAry {
		str += fmt.Sprintf("%v,", tkn)
	}
	return str[:len(str)-1] + "]"
}

func (tkns NFTokens) compress(forJSON bool) []interface{} {
	tknsFullAry := tkns.Slice()
	// Compress the tknsAry by replacing contiguous id ranges with an
	// nfTokenIDRange.
	tknsAry := make([]interface{}, len(tkns))
	firstID := tknsFullAry[0]
	idRange := nfTokenIDRange{Min: firstID, Max: firstID}
	i := 0 // index into tknsAry
	// The first id will be placed when the idRange is inserted either as a
	// single NFTokenID or as a range. The last id does not get included,
	// so append 0.
	for _, id := range append(tknsFullAry[1:], 0) {
		// If this id is contiguous with idRange, expand the range to
		// include this id and check the next id.
		if id == idRange.Max+1 {
			idRange.Max = id
			continue
		}
		// Otherwise, the id is not contiguous with the range, so
		// append the idRange and set up a new idRange to start at id.

		// Use the most efficient JSON or String representation for the
		// idRange.
		if (forJSON && idRange.IsJSONEfficient()) ||
			(!forJSON && idRange.IsStringEfficient()) {
			tknsAry[i] = idRange
			i++
		} else {
			for id := idRange.Min; id <= idRange.Max; id++ {
				tknsAry[i] = id
				i++
			}
		}
		idRange = nfTokenIDRange{Min: id, Max: id}
	}
	return tknsAry[:i]
}

func (tkns *NFTokens) UnmarshalJSON(data []byte) error {
	var tknsJSONAry []json.RawMessage
	if err := json.Unmarshal(data, &tknsJSONAry); err != nil {
		return fmt.Errorf("%T: %w", tkns, err)
	}
	if *tkns == nil {
		*tkns = make(NFTokens, len(tknsJSONAry))
	}
	for _, data := range tknsJSONAry {
		var ids nfTokensSetter
		if data[0] == '{' {
			var idRange nfTokenIDRange
			if err := idRange.UnmarshalJSON(data); err != nil {
				return fmt.Errorf("%T: %w", tkns, err)
			}
			ids = idRange
		} else {
			var id NFTokenID
			if err := json.Unmarshal(data, &id); err != nil {
				return fmt.Errorf("%T: %w", tkns, err)
			}
			ids = id
		}
		if err := tkns.set(ids); err != nil {
			return fmt.Errorf("%T: %w", tkns, err)
		}
	}
	return nil

}

func (tkns *NFTokens) UnmarshalText(text []byte) error {
	if len(text) < 2 || text[0] != '[' || text[len(text)-1] != ']' {
		return fmt.Errorf("invalid format")
	}
	text = []byte(strings.Trim(string(text), "[]"))

	texts := strings.Split(string(text), ",")

	if *tkns == nil {
		*tkns = make(NFTokens, len(texts))
	}

	for _, text := range texts {
		var ids nfTokensSetter
		var idRange nfTokenIDRange
		err := idRange.UnmarshalText([]byte(text))
		if err == nil {
			if err := idRange.Valid(); err != nil {
				return err
			}
			ids = idRange
		} else {

			tknID, err := strconv.ParseInt(texts[0], 10, 64)
			if err != nil {
				return err
			}
			ids = NFTokenID(tknID)
		}
		if err := tkns.set(ids); err != nil {
			return err
		}
	}
	return nil
}
