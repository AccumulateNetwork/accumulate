package protocol

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
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
