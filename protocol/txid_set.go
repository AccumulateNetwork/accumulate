// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func (s *TxIdSet) Add(txid *url.TxID) {
	ptr, new := sortutil.BinaryInsert(&s.Entries, func(x *url.TxID) int {
		return x.Compare(txid)
	})
	if new {
		*ptr = txid
	}
}

func (s *TxIdSet) Remove(txid *url.TxID) {
	i, found := sortutil.Search(s.Entries, func(x *url.TxID) int {
		return x.Compare(txid)
	})
	if found {
		copy(s.Entries[i:], s.Entries[i+1:])
		s.Entries = s.Entries[:len(s.Entries)-1]
	}
}

func (s *TxIdSet) ContainsHash(hash []byte) bool {
	_, found := sortutil.Search(s.Entries, func(x *url.TxID) int {
		h := x.Hash()
		return bytes.Compare(h[:], hash)
	})
	return found
}
