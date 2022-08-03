package protocol

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
)

func (a *AddressBookEntries) Set(e *AddressBookEntry) {
	ptr, _ := sortutil.BinaryInsert(&a.Entries, func(f *AddressBookEntry) int { return bytes.Compare(f.PublicKeyHash[:], e.PublicKeyHash[:]) })
	*ptr = e
}

func (a *AddressBookEntries) Remove(keyHash []byte) {
	i, found := sortutil.Search(a.Entries, func(f *AddressBookEntry) int { return bytes.Compare(f.PublicKeyHash[:], keyHash) })
	if found {
		a.Entries = append(a.Entries[:i], a.Entries[i+1:]...)
	}
}
