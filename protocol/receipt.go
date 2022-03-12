package protocol

import "gitlab.com/accumulatenetwork/accumulate/smt/managed"

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
