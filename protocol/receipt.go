package protocol

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

func (r *Receipt) Convert() *managed.Receipt {
	m := new(managed.Receipt)
	m.Element = r.Start
	m.Nodes = make([]*managed.ReceiptNode, len(r.Entries))

	m.MDRoot = r.Start
	for i, e := range r.Entries {
		m.Nodes[i] = &managed.ReceiptNode{Hash: e.Hash, Right: e.Right}
		if e.Right {
			m.MDRoot = m.MDRoot.Combine(managed.Sha256, e.Hash)
		} else {
			m.MDRoot = managed.Hash(e.Hash).Combine(managed.Sha256, m.MDRoot)
		}
	}

	return m
}

// Combine
// Take a 2nd receipt, attach it to a root receipt, and return the resulting
// receipt.  The idea is that if this receipt is anchored into another chain,
// Then we can create a receipt that proves the element in this receipt all
// the way down to an anchor in the root receipt.
// Note that both this receipt and the root receipt are expected to be good.
// Returns nil if the receipts cannot be combined.
func (r *Receipt) Combine(s *Receipt) *Receipt {
	if !bytes.Equal(r.Result, s.Start) {
		return nil
	}
	nr := r.Copy()                // Make a copy of the first Receipt
	nr.Result = s.Result          // The MDRoot will be the one from the appended receipt
	for _, n := range s.Entries { // Make a copy and append the Entries of the appended receipt
		nr.Entries = append(nr.Entries, *n.Copy())
	}
	return nr
}

func ReceiptFromManaged(src *managed.Receipt) *Receipt {
	r := new(Receipt)
	r.Start = src.Element
	r.Result = src.MDRoot
	r.Entries = make([]ReceiptEntry, len(src.Nodes))
	for i, n := range src.Nodes {
		r.Entries[i] = ReceiptEntry{Right: n.Right, Hash: n.Hash}
	}
	return r
}
