// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"bytes"
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// An account's sync state, as ChangedAccountStates and AccountStates serve it,
// is written here (executor.md, "Sync", §3; #4453): one account's main state,
// directory, pending list, the rest of its leaf, and each chain's head with its
// open mark set, all read from one view of the peer's store. Nothing below a
// chain's open mark set is taken, and no message: the account is taken by its
// state, and backfilled after the match.

// CheckState rebuilds the leaf of a served account from its parts, as
// hashState does (internal/database/observer_prod.go), and refuses the
// account unless the leaf is the one served beside them. It is not a proof --
// the peer states both -- but a torn or inconsistent answer is refused on
// arrival instead of being left for the root match to find (executor.md,
// "Sync", §3).
func CheckState(st *api.AccountState) error {
	if st == nil || st.Url == nil {
		return errors.Conflict.With("the peer served an account state that names no account")
	}
	if st.NoLeaf {
		return errors.Conflict.WithFormat("%v: the peer holds no leaf for it", st.Url)
	}
	if st.Main == nil {
		// A leaf with no body does not exist (invariant 13; #4437).
		return errors.Conflict.WithFormat("%v: the peer served a leaf with no body", st.Url)
	}
	if got := st.Main.GetUrl(); got == nil || !got.Equal(st.Url) {
		return errors.Conflict.WithFormat("%v: the peer served the body of %v", st.Url, got)
	}
	rebuilt, err := LeafHashOf(st)
	if err != nil {
		return err
	}
	if !bytes.Equal(rebuilt, st.LeafHash[:]) {
		return errors.Conflict.WithFormat("%v: the parts served hash to leaf %x, not to the leaf %x served beside them",
			st.Url, rebuilt[:8], st.LeafHash[:8])
	}

	// The open mark sets are what an append rebuilds the tail from; each
	// must be as long as its chain's head says (RestoreHead checks that it
	// reproduces the head).
	open := openEntries(st)
	for _, c := range leafOf(st).Chains {
		if _, ok := open[c.Name]; !ok && c.Count > 0 {
			return errors.Conflict.WithFormat("%v: the peer served no open mark set for chain %s", st.Url, c.Name)
		}
	}
	return nil
}

// LeafHashOf is the leaf a served account's parts hash to, by hashState's
// rules: its main state's hash, then [database.RetainedLeaf.EntryHash] over
// the rest.
func LeafHashOf(st *api.AccountState) ([]byte, error) {
	leaf, err := retainedLeaf(st)
	if err != nil {
		return nil, errors.Conflict.WithFormat("%v: %w", st.Url, err)
	}
	main, err := database.MainStateHash(st.Main)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("%v: hash the main state: %w", st.Url, err)
	}
	rebuilt, err := leaf.EntryHash(main)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("%v: rebuild the leaf: %w", st.Url, err)
	}
	return rebuilt, nil
}

// retainedLeaf is the served leaf in the store's form, from which
// [database.RetainedLeaf.EntryHash] rebuilds the leaf by hashState's rules.
func retainedLeaf(st *api.AccountState) (*database.RetainedLeaf, error) {
	l := leafOf(st)
	out := &database.RetainedLeaf{
		Directory:            st.Directory,
		EventsRoot:           l.EventsRoot,
		LocalDeliveryQueue:   l.LocalDeliveryQueue,
		CascadeDeliveryQueue: l.CascadeDeliveryQueue,
	}
	for _, c := range l.Chains {
		if c == nil || c.Name == "" {
			return nil, fmt.Errorf("a chain head with no name")
		}
		out.Chains = append(out.Chains, &database.RetainedChainHead{Name: c.Name, Count: c.Count, Pending: c.State})
	}
	for _, p := range l.Pending {
		v, err := retainedPending(p)
		if err != nil {
			return nil, err
		}
		out.Pending = append(out.Pending, v)
	}
	for _, p := range l.BookPending {
		v, err := retainedPending(p)
		if err != nil {
			return nil, err
		}
		out.BookPending = append(out.BookPending, v)
	}
	return out, nil
}

func retainedPending(p *api.PendingTransactionSets) (*database.RetainedPendingTransaction, error) {
	if p == nil || p.TxID == nil {
		return nil, fmt.Errorf("a pending transaction with no ID")
	}
	v := &database.RetainedPendingTransaction{
		TxID:                p.TxID,
		V1Hashes:            p.V1Hashes,
		ValidatorSignatures: p.ValidatorSignatures,
		Payments:            p.Payments,
	}
	for _, e := range p.Votes {
		x := new(database.VoteEntry)
		if err := reencode(e, x); err != nil {
			return nil, fmt.Errorf("vote: %w", err)
		}
		v.Votes = append(v.Votes, x)
	}
	for _, e := range p.Signatures {
		x := new(database.SignatureSetEntry)
		if err := reencode(e, x); err != nil {
			return nil, fmt.Errorf("signature: %w", err)
		}
		v.Signatures = append(v.Signatures, x)
	}
	return v, nil
}

// reencode moves a value between the API's form and the store's, which
// marshal alike: the leaf hashes the encoding.
func reencode(from interface{ MarshalBinary() ([]byte, error) }, to interface{ UnmarshalBinary([]byte) error }) error {
	if from == nil {
		return fmt.Errorf("empty")
	}
	b, err := from.MarshalBinary()
	if err != nil {
		return err
	}
	return to.UnmarshalBinary(b)
}

func leafOf(st *api.AccountState) *api.AccountLeaf {
	if st.Leaf == nil {
		return new(api.AccountLeaf)
	}
	return st.Leaf
}

func openEntries(st *api.AccountState) map[string][][]byte {
	out := map[string][][]byte{}
	for _, o := range st.OpenEntries {
		if o != nil {
			out[o.Name] = o.Entries
		}
	}
	return out
}

// KeepState writes a served account state, checked by CheckState, into the
// store in a batch of its own, with the BPT, and commits it. What is written
// replaces what the node held for the account (executor.md, "Sync", §3): its
// body, directory, pending list and what it holds for each pending
// transaction, its chain index and each chain's head with its open mark set.
func KeepState(store Store, st *api.AccountState) error {
	if err := CheckState(st); err != nil {
		return err
	}
	leaf, err := retainedLeaf(st)
	if err != nil {
		return errors.Conflict.WithFormat("%v: %w", st.Url, err)
	}

	batch := store.Begin(true)
	defer batch.Discard()
	u := st.Url
	account := batch.Account(u)
	if err := account.Main().Put(st.Main); err != nil {
		return errors.UnknownError.WithFormat("%v: store main: %w", u, err)
	}
	if err := account.Directory().Put(st.Directory); err != nil {
		return errors.UnknownError.WithFormat("%v: store directory: %w", u, err)
	}
	var pending []*url.TxID
	for _, p := range leaf.Pending {
		pending = append(pending, p.TxID)
	}
	if err := account.Pending().Put(pending); err != nil {
		return errors.UnknownError.WithFormat("%v: store pending: %w", u, err)
	}
	// What the leaf hashes for each pending transaction, the account's own
	// and, for a key page, what it holds for its book's.
	for _, p := range append(append([]*database.RetainedPendingTransaction(nil), leaf.Pending...), leaf.BookPending...) {
		if err := putPendingSets(account, p); err != nil {
			return errors.UnknownError.WithFormat("%v: store the sets of pending %v: %w", u, p.TxID, err)
		}
	}

	// The rest of a system account's leaf.
	if _, ok := protocol.ParsePartitionUrl(u); ok {
		switch {
		case u.PathEqual(protocol.Synthetic):
			if err := account.LocalDeliveryQueue().Put(leaf.LocalDeliveryQueue); err != nil {
				return errors.UnknownError.WithFormat("%v: store local delivery queue: %w", u, err)
			}
			if err := account.CascadeDeliveryQueue().Put(leaf.CascadeDeliveryQueue); err != nil {
				return errors.UnknownError.WithFormat("%v: store cascade delivery queue: %w", u, err)
			}
		case u.PathEqual(protocol.Ledger):
			if err := replaceEvents(account.Events(), leafOf(st).Events); err != nil {
				return errors.UnknownError.WithFormat("%v: store scheduled events: %w", u, err)
			}
		}
	}

	// Each chain's head and open mark set, and the chain index as the peer
	// holds it: a chain the node holds that the peer's account does not is
	// hashed into the leaf by the index, so the index is replaced.
	open := openEntries(st)
	var index []*protocol.ChainMetadata
	for _, c := range leafOf(st).Chains {
		chain, err := account.ChainByName(c.Name)
		if err != nil {
			return errors.UnknownError.WithFormat("%v: chain %s: %w", u, c.Name, err)
		}
		head := &merkle.State{Count: int64(c.Count), Pending: c.State}
		if err := chain.Inner().RestoreHead(head, open[c.Name]); err != nil {
			return errors.Conflict.WithFormat("%v: chain %s: %w", u, c.Name, err)
		}
		index = append(index, &protocol.ChainMetadata{Name: c.Name, Type: c.Type})
	}
	if err := account.Chains().Put(index); err != nil {
		return errors.UnknownError.WithFormat("%v: store the chain index: %w", u, err)
	}

	// Commit does not update the BPT (#4305).
	if err := batch.UpdateBPT(); err != nil {
		return errors.UnknownError.Wrap(err)
	}
	return errors.UnknownError.Wrap(batch.Commit())
}

func putPendingSets(account *database.Account, p *database.RetainedPendingTransaction) error {
	txn := account.Transaction(p.TxID.Hash())
	if err := txn.ValidatorSignatures().Put(p.ValidatorSignatures); err != nil {
		return err
	}
	if err := txn.Payments().Put(p.Payments); err != nil {
		return err
	}
	if err := txn.Votes().Put(p.Votes); err != nil {
		return err
	}
	return txn.Signatures().Put(p.Signatures)
}

// KeepQueuedMessages fetches, from src, what the store lacks of each queued
// local delivery of a served synthetic ledger: the message, which the next
// block executes from (block.drainDeliveryQueues), and its transaction's
// status where the peer had delivered it by block, the block the state was
// served at.
//
// The status matters because a local delivery is deduplicated by hash: a burn
// the Directory queues for its own credit purchases is a SyntheticBurnTokens
// to acc://ACME carrying only its amount, so two burns of one amount are one
// transaction, and a node holding the first one's status skips the second.
// The status is not in the BPT. A joined node held none and burned again,
// leaving ACME's issued amount below its peers' at the first block after its
// proof (#4442, run 20260928T051402Z). A status the peer first received after
// block is the drain the next block does, which this node does too: it is not
// taken.
func KeepQueuedMessages(ctx context.Context, src Source, store Store, st *api.AccountState, block uint64) error {
	if st == nil || st.Url == nil || st.Leaf == nil || len(st.Leaf.LocalDeliveryQueue) == 0 {
		return nil
	}
	if _, ok := protocol.ParsePartitionUrl(st.Url); !ok || !st.Url.PathEqual(protocol.Synthetic) {
		return nil
	}
	batch := store.Begin(true)
	defer batch.Discard()
	for _, id := range st.Leaf.LocalDeliveryQueue {
		rec, err := src.QueryMessage(ctx, id, nil)
		if err != nil {
			return errors.UnknownError.WithFormat("queued local delivery %v: %w", id, err)
		}
		if rec == nil || rec.Message == nil {
			return errors.Conflict.WithFormat("queued local delivery %v: the peer served no message", id)
		}
		if h := rec.Message.Hash(); h != id.Hash() {
			return errors.Conflict.WithFormat("queued local delivery %v: the peer served a message that hashes to %x", id, h[:8])
		}
		if err := batch.Message(id.Hash()).Main().Put(rec.Message); err != nil {
			return errors.UnknownError.WithFormat("queued local delivery %v: %w", id, err)
		}

		status := &protocol.TransactionStatus{TxID: id, Code: rec.Status, Error: rec.Error, Result: rec.Result, Received: rec.Received}
		if rec.Received == 0 || rec.Received > block || !status.Delivered() {
			continue
		}
		h := id.Hash()
		held, err := batch.Transaction(h[:]).Status().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("queued local delivery %v: load status: %w", id, err)
		}
		if held.Delivered() {
			continue
		}
		if err := batch.Transaction(h[:]).Status().Put(status); err != nil {
			return errors.UnknownError.WithFormat("queued local delivery %v: store status: %w", id, err)
		}
	}
	return errors.UnknownError.Wrap(batch.Commit())
}
