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

	"github.com/AccumulateNetwork/accumulated/factom/jsonlen"
)

type NFTokenIDMetadataMap map[NFTokenID]json.RawMessage

type nfTokenMetadata struct {
	Tokens   NFTokens        `json:"ids"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

func (m *NFTokenIDMetadataMap) UnmarshalJSON(data []byte) error {
	var tknMs []struct {
		Tokens   json.RawMessage `json:"ids"`
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(data, &tknMs); err != nil {
		return fmt.Errorf("%T: %w", m, err)
	}
	*m = make(NFTokenIDMetadataMap, len(tknMs))
	var expectedJSONLen int
	for _, tknM := range tknMs {
		if len(tknM.Tokens) == 0 {
			return fmt.Errorf(`%T: missing required field "ids"`, m)
		}
		if len(tknM.Metadata) == 0 {
			return fmt.Errorf(`%T: missing required field "metadata"`, m)
		}
		var tkns NFTokens
		if err := tkns.UnmarshalJSON(tknM.Tokens); err != nil {
			return fmt.Errorf("%T: %w", m, err)
		}
		metadata := jsonlen.Compact(tknM.Metadata)
		expectedJSONLen += len(metadata) + len(jsonlen.Compact(tknM.Tokens))
		for tknID := range tkns {
			if _, ok := (*m)[tknID]; ok {
				return fmt.Errorf("%T: Duplicate NFTokenID: %v", m, tknID)
			}
			(*m)[tknID] = metadata
		}
	}
	expectedJSONLen += len(`[]`) - len(`,`) +
		len(tknMs)*len(`{"ids":,"metadata":},`)
	if expectedJSONLen != len(jsonlen.Compact(data)) {
		return fmt.Errorf("%T: unexpected JSON length %v %v ",
			m, expectedJSONLen, len(jsonlen.Compact(data)))

	}
	return nil
}

func (m NFTokenIDMetadataMap) MarshalJSON() ([]byte, error) {
	metadataNFTokens := make(map[string]NFTokens, len(m))
	for tknID, metadata := range m {
		tkns := metadataNFTokens[string(metadata)]
		if tkns == nil {
			tkns = make(NFTokens)
			metadataNFTokens[string(metadata)] = tkns
		}
		if err := tkns.set(tknID); err != nil {
			return nil, err
		}
	}

	var i int
	tknMs := make([]nfTokenMetadata, len(metadataNFTokens))
	for metadata, tkns := range metadataNFTokens {
		tknMs[i].Tokens = tkns
		tknMs[i].Metadata = json.RawMessage(metadata)
		i++
	}

	return json.Marshal(tknMs)
}

func (m NFTokenIDMetadataMap) isSubsetOf(tkns NFTokens) error {
	if len(m) > len(tkns) {
		return fmt.Errorf("too many NFTokenIDs")
	}
	for tknID := range m {
		if _, ok := tkns[tknID]; ok {
			continue
		}
		return fmt.Errorf("NFTokenID(%v) is missing", tknID)
	}
	return nil
}

func (m NFTokenIDMetadataMap) set(md nfTokenMetadata) {
	for tknID := range md.Tokens {
		m[tknID] = md.Metadata
	}
}
