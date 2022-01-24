package protocol

import "github.com/AccumulateNetwork/accumulate/smt/managed"

func (r *Receipt) Convert() *managed.Receipt {
	m := new(managed.Receipt)
	m.Element = r.Start
	m.Nodes = make([]*managed.Node, len(r.Entries))

	m.MDRoot = r.Start
	for i, e := range r.Entries {
		m.Nodes[i] = &managed.Node{Hash: e.Hash, Right: e.Right}
		if e.Right {
			m.MDRoot = m.MDRoot.Combine(managed.Sha256, e.Hash)
		} else {
			m.MDRoot = managed.Hash(e.Hash).Combine(managed.Sha256, m.MDRoot)
		}
	}

	return m
}
