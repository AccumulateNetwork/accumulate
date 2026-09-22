// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// LeafState returns what the account's BPT entry commits to besides its main
// state: exactly what hashState (observer_prod.go) reads, as values rather than
// hashes, so it can be retained per block and served as of that block (#4361).
//
// It reads the store as hashState does, so what it returns is what the entry
// written from the same batch hashed. [RetainedLeaf.EntryHash] rebuilds the
// entry from it, and a historical answer checks that rebuild against the entry
// before serving it, so a divergence between the two is refused rather than
// served.
func (a *Account) LeafState() (*RetainedLeaf, error) {
	var err error
	l := new(RetainedLeaf)
	l.Directory = loadState(&err, false, a.Directory().Get)

	u := a.Url()
	if _, ok := protocol.ParsePartitionUrl(u); ok && u.PathEqual(protocol.Ledger) {
		l.EventsRoot = loadState(&err, false, a.Events().BPT().GetRootHash)
	}
	if _, ok := protocol.ParsePartitionUrl(u); ok && u.PathEqual(protocol.Synthetic) {
		l.LocalDeliveryQueue = loadState(&err, false, a.LocalDeliveryQueue().Get)
		l.CascadeDeliveryQueue = loadState(&err, false, a.CascadeDeliveryQueue().Get)
	}

	for _, meta := range loadState(&err, false, a.Chains().Get) {
		chain := loadState1(&err, false, a.GetChainByName, meta.Name)
		if err != nil {
			break
		}
		head := chain.CurrentState()
		h := &RetainedChainHead{Name: meta.Name, Count: uint64(head.Count)}
		for _, v := range head.Pending {
			if v != nil {
				v = append([]byte(nil), v...)
			}
			h.Pending = append(h.Pending, v)
		}
		l.Chains = append(l.Chains, h)
	}

	for _, txid := range loadState(&err, false, a.Pending().Get) {
		p := &RetainedPendingTransaction{TxID: txid}

		// V1 BPT logic for pending transactions
		v1 := a.parent.Transaction2(txid.Hash())
		if main := loadState(&err, true, v1.Main().Get); main != nil {
			p.V1Hashes = append(p.V1Hashes, hashOfBinary(&err, main))
			p.V1Hashes = append(p.V1Hashes, hashOfBinary(&err, loadState(&err, false, v1.Status().Get)))
		}
		a.pendingSets(&err, p)
		l.Pending = append(l.Pending, p)
	}

	// A page hashes the sets it holds for its book's pending transactions
	if page, ok := loadState(&err, true, a.Main().Get).(*protocol.KeyPage); ok {
		for _, txid := range loadState(&err, false, a.parent.Account(page.GetAuthority()).Pending().Get) {
			p := &RetainedPendingTransaction{TxID: txid}
			a.pendingSets(&err, p)
			l.BookPending = append(l.BookPending, p)
		}
	}

	if err != nil {
		return nil, errors.UnknownError.WithFormat("load leaf state of %v: %w", u, err)
	}
	return l, nil
}

func (a *Account) pendingSets(err *error, p *RetainedPendingTransaction) {
	txn := a.Transaction(p.TxID.Hash())
	p.ValidatorSignatures = loadState(err, true, txn.ValidatorSignatures().Get)
	p.Payments = loadState(err, true, txn.Payments().Get)
	p.Votes = loadState(err, true, txn.Votes().Get)
	p.Signatures = loadState(err, true, txn.Signatures().Get)
}

func hashOfBinary(err *error, v interface{ MarshalBinary() ([]byte, error) }) [32]byte {
	if *err != nil {
		return [32]byte{}
	}
	data, e := v.MarshalBinary()
	if e != nil {
		*err = e
		return [32]byte{}
	}
	return sha256.Sum256(data)
}

// EntryHash rebuilds the account's BPT entry from the hash of its main state
// and the rest of the leaf, by hashState's rules.
func (l *RetainedLeaf) EntryHash(mainHash []byte) ([]byte, error) {
	var err error
	var secondary, dir hash.Hasher
	for _, u := range l.Directory {
		dir.AddUrl(u)
	}
	secondary.AddValue(dir)
	if l.EventsRoot != [32]byte{} {
		secondary.AddHash2(l.EventsRoot)
	}
	if len(l.LocalDeliveryQueue)+len(l.CascadeDeliveryQueue) > 0 {
		var q hash.Hasher
		for _, id := range l.LocalDeliveryQueue {
			q.AddUrl(id.AsUrl())
		}
		for _, id := range l.CascadeDeliveryQueue {
			q.AddUrl(id.AsUrl())
		}
		secondary.AddValue(q)
	}

	var chains hash.Hasher
	for _, c := range l.Chains {
		state := c.State()
		if state.Count == 0 {
			chains.AddHash(new([32]byte))
		} else {
			chains.AddHash((*[32]byte)(state.Anchor()))
		}
	}

	var pending hash.Hasher
	for _, p := range l.Pending {
		if len(p.V1Hashes) > 0 {
			for _, h := range p.V1Hashes {
				pending.AddHash2(h)
			}
		} else {
			pending.AddTxID(p.TxID)
		}
		p.addSets(&err, &pending)
	}
	for _, p := range l.BookPending {
		p.addSets(&err, &pending)
	}
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return hash.Hasher{mainHash, secondary.MerkleHash(), chains.MerkleHash(), pending.MerkleHash()}.MerkleHash(), nil
}

func (p *RetainedPendingTransaction) addSets(err *error, hasher *hash.Hasher) {
	for _, sig := range p.ValidatorSignatures {
		hasher.AddHash((*[32]byte)(sig.Hash()))
	}
	for _, h := range p.Payments {
		hasher.AddHash2(h)
	}
	for _, v := range p.Votes {
		hasher.AddHash2(hashOfBinary(err, v))
	}
	for _, v := range p.Signatures {
		hasher.AddHash2(hashOfBinary(err, v))
	}
}

// State returns the chain head as a merkle state, from which its anchor is
// computed.
func (c *RetainedChainHead) State() *merkle.State {
	return &merkle.State{Count: int64(c.Count), Pending: c.Pending}
}
