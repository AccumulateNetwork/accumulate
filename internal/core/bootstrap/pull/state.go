// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"log/slog"
	"strings"

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
	if st.NotLoaded {
		// Neither a state nor "no leaf": the peer has not loaded the
		// section, and another peer is asked (executor.md, "Sync", §3a;
		// #4496).
		return errors.NotReady.WithFormat("%v: the peer has not loaded section %d of its state tree", st.Url, st.Section)
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
// store in a batch of its own, with the BPT, and commits it (KeepStates).
// asOf is the block the state is of.
func KeepState(store Store, st *api.AccountState, asOf uint64) error {
	return KeepStates(store, []*api.AccountState{st}, []uint64{asOf})[0]
}

// ErrStaleState is a served state dropped at the write: the node holds the
// account as of a later block (executor spec, "Sync", §3a, "Every account
// state the node holds is stamped"; #4518). A writer that meets it has
// nothing to do for that account.
var ErrStaleState = stderrors.New("the state held is of a later block")

// stampedWrite checks a served state against the block the node holds the
// account as of: older is dropped (ErrStaleState); the same block with a
// different leaf is the account that failed to have the right state, said,
// and the served state -- the block ledger's -- stands. It returns the stamp
// to write with the state.
func stampedWrite(batch *database.Batch, account *database.Account, st *api.AccountState, asOf uint64) error {
	if asOf == 0 {
		return nil
	}
	held, err := account.StateBlock().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("%v: load the state block: %w", st.Url, err)
	}
	switch {
	case held > asOf:
		mStaleStateWrites.Inc()
		return ErrStaleState
	case held == asOf:
		local, err := batch.BPT().Get(account.Key())
		if err == nil && len(local) == 32 && [32]byte(local) != st.LeafHash {
			mStateDiffers.Inc()
			slog.Warn("An account failed to have the right state; the block ledger's state stands",
				"account", st.Url, "block", asOf, "local", fmt.Sprintf("%x", local[:8]), "record", fmt.Sprintf("%x", st.LeafHash[:8]))
		}
	}
	return nil
}

var (
	// mStaleStateWrites counts served states dropped because the node held
	// the account as of a later block.
	mStaleStateWrites = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "pull", Name: "stale_state_writes_total",
		Help: "Served account states dropped at the write: the node held the account as of a later block",
	})
	// mStateDiffers counts accounts whose served state, of the block the node
	// held them as of, differed from what the node held.
	mStateDiffers = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "pull", Name: "state_differs_total",
		Help: "Accounts whose state as of the block the node held them at differed from the block ledger's",
	})
)

// KeepStates writes served account states, each checked by CheckState, into
// the store in one batch, with the BPT, and commits it once: one commit per
// answer, not per account (#4461: on BlockchainDB a commit cost the join about
// 85 ms, and a walk page of 256 accounts 22 s). What is written replaces what
// the node held for each account (executor.md, "Sync", §3): its body,
// directory, pending list and what it holds for each pending transaction, its
// chain index and each chain's head with its open mark set.
//
// The result holds, for each state, why it was refused, or nil. An account
// refused is written not at all -- each is written in a batch nested in the
// one committed -- and the others are written. If the commit fails, every
// account is refused with its error.
func KeepStates(store Store, states []*api.AccountState, blocks []uint64) []error {
	errs := make([]error, len(states))
	batch := store.Begin(true)
	defer batch.Discard()
	kept := 0
	for i, st := range states {
		if err := CheckState(st); err != nil {
			errs[i] = err
			continue
		}
		var asOf uint64
		if i < len(blocks) {
			asOf = blocks[i]
		}
		one := batch.Begin(true)
		err := writeState(one, st, asOf)
		if err == nil {
			err = one.Commit()
		}
		one.Discard()
		if stderrors.Is(err, ErrStaleState) {
			continue // Held as of a later block: nothing to write
		}
		if err != nil {
			errs[i] = err
			continue
		}
		kept++
	}
	if kept == 0 {
		return errs
	}

	// Commit does not update the BPT (#4305).
	err := batch.UpdateBPT()
	if err == nil {
		err = batch.Commit()
	}
	if err != nil {
		for i := range errs {
			if errs[i] == nil {
				errs[i] = errors.UnknownError.Wrap(err)
			}
		}
	}
	return errs
}

// WriteStates writes checked account states into the batch, each in a batch
// nested in it, and updates no tree: a section's load writes its accounts
// beneath the batch a block executes in, and swaps the section's marker for
// the leaves itself (executor.md, "Sync", §3a; #4485). The first state that
// cannot be written is the error, and nothing of it is written.
func WriteStates(batch *database.Batch, states []*api.AccountState, asOf uint64) error {
	for _, st := range states {
		if err := CheckState(st); err != nil {
			return err
		}
		one := batch.Begin(true)
		err := writeState(one, st, asOf)
		if err == nil {
			err = one.Commit()
		}
		one.Discard()
		if stderrors.Is(err, ErrStaleState) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// The parts of a served state a block's settling writes (SettleStates).
type SettleParts int

const (
	// SettleAll is every part, as WriteStates writes them.
	SettleAll SettleParts = iota

	// SettleNotIndex is every part but the index chains, which a block
	// writes when it closes, after its transactions.
	SettleNotIndex

	// SettleIndex is the index chains alone.
	SettleIndex
)

// SettleStates writes checked account states into the batch a block executes
// in, as WriteStates does, except that a chain whose head the batch already
// holds as served is left as it is -- a chain written is a chain the block
// changed, and the block's record names it -- and only the parts named
// (executor.md, "Sync", §3a, "The handoff is an overlap"; #4495).
func SettleStates(batch *database.Batch, states []*api.AccountState, parts SettleParts, asOf uint64) error {
	for _, st := range states {
		if err := CheckState(st); err != nil {
			return err
		}
		one := batch.Begin(true)
		err := writeStateWith(one, st, true, parts, asOf)
		if err == nil {
			err = one.Commit()
		}
		one.Discard()
		if stderrors.Is(err, ErrStaleState) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// writeState writes one checked account state into the batch.
func writeState(batch *database.Batch, st *api.AccountState, asOf uint64) error {
	return writeStateWith(batch, st, false, SettleAll, asOf)
}

// sameHead is whether the chain's head is count and pending already.
func sameHead(chain *database.Chain2, count uint64, pending [][]byte) bool {
	head, err := chain.Inner().Head().Get()
	if err != nil || head.Count != int64(count) || len(head.Pending) != len(pending) {
		return false
	}
	for i := range pending {
		if !bytes.Equal(head.Pending[i], pending[i]) {
			return false
		}
	}
	return true
}

// writeStateWith writes the parts of one checked account state into the
// batch; changed leaves alone each chain whose head is already the one served.
func writeStateWith(batch *database.Batch, st *api.AccountState, changed bool, parts SettleParts, asOf uint64) error {
	leaf, err := retainedLeaf(st)
	if err != nil {
		return errors.Conflict.WithFormat("%v: %w", st.Url, err)
	}

	u := st.Url
	account := batch.Account(u)
	if parts == SettleIndex {
		return writeChains(account, st, changed, parts)
	}
	if err := stampedWrite(batch, account, st, asOf); err != nil {
		return err
	}
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

	if asOf != 0 {
		if err := account.StateBlock().Put(asOf); err != nil {
			return errors.UnknownError.WithFormat("%v: stamp the state block: %w", u, err)
		}
	}
	return writeChains(account, st, changed, parts)
}

// writeChains writes each chain's head and open mark set, and the chain index
// as the peer holds it: a chain the node holds that the peer's account does
// not is hashed into the leaf by the index, so the index is replaced.
func writeChains(account *database.Account, st *api.AccountState, changed bool, parts SettleParts) error {
	u := st.Url
	open := openEntries(st)
	var index []*protocol.ChainMetadata
	for _, c := range leafOf(st).Chains {
		chain, err := account.ChainByName(c.Name)
		if err != nil {
			return errors.UnknownError.WithFormat("%v: chain %s: %w", u, c.Name, err)
		}
		isIndex := strings.HasSuffix(strings.ToLower(c.Name), "-index")
		if (parts == SettleNotIndex && isIndex) || (parts == SettleIndex && !isIndex) ||
			(changed && sameHead(chain, c.Count, c.State)) {
			index = append(index, &protocol.ChainMetadata{Name: c.Name, Type: c.Type})
			continue
		}
		head := &merkle.State{Count: int64(c.Count), Pending: c.State}
		prev, err := chain.Inner().Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("%v: chain %s: load the head: %w", u, c.Name, err)
		}
		// Taken by its head alone: the open mark set came without messages
		// (executor spec, "Past the match").
		held := c.Count - uint64(len(open[c.Name]))
		if err := recordTaken(account, c.Name, c.Type, uint64(prev.Count), head, held, c.Count); err != nil {
			return errors.UnknownError.WithFormat("%v: chain %s: %w", u, c.Name, err)
		}
		if err := chain.Inner().RestoreHead(head, open[c.Name]); err != nil {
			return errors.Conflict.WithFormat("%v: chain %s: %w", u, c.Name, err)
		}
		index = append(index, &protocol.ChainMetadata{Name: c.Name, Type: c.Type})
	}
	if err := account.Chains().Put(index); err != nil {
		return errors.UnknownError.WithFormat("%v: store the chain index: %w", u, err)
	}
	return nil
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

// KeepChainMessages fetches, from src, the message behind each entry of the
// open mark sets of a served state's chains that carry messages, where the
// store does not hold it, and keeps each only if it hashes to its entry. A
// block that settles an account against its record writes the record's chain
// heads and open mark sets (SettleStates); the entries this node did not
// execute have no message behind them until this takes them (executor.md,
// "Sync", §3a, "The handoff is an overlap"; #4495).
func KeepChainMessages(ctx context.Context, src Source, store Store, st *api.AccountState) error {
	if st == nil || st.Url == nil {
		return nil
	}
	open := openEntries(st)
	batch := store.Begin(true)
	defer batch.Discard()
	for _, c := range leafOf(st).Chains {
		if c == nil || !carriesMessages(c) {
			continue
		}
		for _, e := range open[c.Name] {
			if len(e) != 32 {
				continue
			}
			_, err := batch.Message([32]byte(e)).Main().Get()
			switch {
			case err == nil:
				continue
			case !errors.Is(err, errors.NotFound):
				return errors.UnknownError.WithFormat("%v chain %s: %w", st.Url, c.Name, err)
			}
			if err := pullMessage(ctx, src, batch, st.Url.WithTxID([32]byte(e))); err != nil {
				return errors.UnknownError.WithFormat("%v chain %s entry %x: %w", st.Url, c.Name, e[:8], err)
			}
		}
	}
	return errors.UnknownError.Wrap(batch.Commit())
}

// KeepQueuedMessages fetches, from src, the message behind each queued local
// delivery of a served synthetic ledger that the store does not hold, and
// keeps each only if it hashes to the ID the queue names. The next block
// executes each from its stored message (block.drainDeliveryQueues), so a
// queue written by KeepState without them cannot be executed from (#4442).
func KeepQueuedMessages(ctx context.Context, src Source, store Store, st *api.AccountState) error {
	if st == nil || st.Url == nil || st.Leaf == nil || len(st.Leaf.LocalDeliveryQueue) == 0 {
		return nil
	}
	if _, ok := protocol.ParsePartitionUrl(st.Url); !ok || !st.Url.PathEqual(protocol.Synthetic) {
		return nil
	}
	batch := store.Begin(true)
	defer batch.Discard()
	for _, id := range st.Leaf.LocalDeliveryQueue {
		_, err := batch.Message(id.Hash()).Main().Get()
		switch {
		case err == nil:
			continue
		case !errors.Is(err, errors.NotFound):
			return errors.UnknownError.WithFormat("queued local delivery %v: %w", id, err)
		}
		if err := pullMessage(ctx, src, batch, id); err != nil {
			return errors.UnknownError.WithFormat("queued local delivery %v: %w", id, err)
		}
	}
	return errors.UnknownError.Wrap(batch.Commit())
}
