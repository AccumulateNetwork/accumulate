// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"encoding/binary"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// accountStatesBudget is how many bytes of marshalled account states one page
// of AccountStates carries. An account larger than it is answered in parts. A
// variable so a test can page a small store.
var accountStatesBudget = 1 << 20

// maxRecordSpan is how many blocks of records one ChangedAccountStates answer
// covers: T is at most After plus this. It is a bound, not the expected case:
// a joining node's catch-up after its index is a few blocks, and an answer the
// cap cuts short means the join fell behind (Paul, 2026-09-28; #4461).
const maxRecordSpan = 1024

// maxAccountStatesAsked is how many accounts one AccountStates query may name:
// a walk page's worth and more (executor.md, "Sync", §3).
const maxAccountStatesAsked = 4096

// queryChangedAccountStates answers the block-ledger records of the blocks
// after After through T, from one view of the store (executor.md, "Sync",
// §3; #4453). T is this node's block B, capped at maxRecordSpan blocks after
// After. One answer is the whole catch-up from After up to the cap (#4461):
// the records name every account those blocks changed (invariant 14), and
// the asking node takes their states by AccountStates, as of T, at once
// (#4552). The answer carries no states: served inside the answer they came
// a page at a time, each page a view of its own, so a round of the pull was
// complete only when they fit one page, and a node joining a loaded
// partition never caught up (run 20261007T211137Z).
func (s *Querier) queryChangedAccountStates(ctx context.Context, batch *database.Batch, scope *url.URL, query *api.ChangedAccountStatesQuery) (*api.AccountStatesRecord, error) {
	if scope == nil || !s.partition.URL.Equal(scope) {
		return nil, errors.BadRequest.WithFormat(
			"the accounts a partition's records name are the partition's: this node serves %v, and the query is scoped to %v",
			s.partition.URL, scope)
	}
	block, err := s.viewBlock(batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if block < query.After {
		// Behind the asker: its records are records this node has not
		// written yet. Another peer is asked.
		return nil, errors.NotReady.WithFormat("this node stands at block %d, before block %d", block, query.After)
	}
	through := min(block, query.After+maxRecordSpan)
	records, err := s.recordsBetween(batch, query.After, through)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return &api.AccountStatesRecord{Records: records, Through: through, Block: block}, nil
}

// queryAccountStates answers the sync state of the named accounts, from this
// one view of the store, stamped with its block (executor.md, "Sync", §3).
func (s *Querier) queryAccountStates(ctx context.Context, batch *database.Batch, scope *url.URL, query *api.AccountStatesQuery) (*api.AccountStatesRecord, error) {
	if scope == nil || !s.partition.URL.Equal(scope) {
		return nil, errors.BadRequest.WithFormat(
			"account states are asked of the partition: this node serves %v, and the query is scoped to %v",
			s.partition.URL, scope)
	}
	if len(query.Accounts) > maxAccountStatesAsked {
		return nil, errors.BadRequest.WithFormat("%d accounts asked for, at most %d", len(query.Accounts), maxAccountStatesAsked)
	}
	for _, u := range query.Accounts {
		if u == nil {
			return nil, errors.BadRequest.With("an account asked for is empty")
		}
	}
	block, err := s.viewBlock(batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	cur, err := decodeStatesCursor(query.Cursor)
	if err != nil {
		return nil, errors.BadRequest.Wrap(err)
	}
	if query.ForHeight != 0 && query.ForHeight != block {
		// As of a block: each state is the one the node retained for the
		// block (executor.md, "Sync", §3a; #4485). The block is the one the
		// height resolves to, as a BPT page's is.
		_, at, err := s.retainedRootAt(batch, query.ForHeight)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		peers := s.redirectOnce(ctx)
		r, err := s.statesPageWith(query.Accounts, cur, func(u *url.URL) *api.AccountState {
			return s.accountStateAt(batch, u, at, peers)
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		r.Block = at
		return r, nil
	}
	// As of this view's own block, the current state is the block's.
	r, err := s.statesPage(ctx, batch, query.Accounts, cur)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	r.Block = block
	return r, nil
}

// viewBlock is the block this view of the store is at: the partition ledger's
// index, written in the same commit as everything the block changed. A store
// with no ledger has executed nothing, and is at block 0.
func (s *Querier) viewBlock(batch *database.Batch) (uint64, error) {
	var ledger *protocol.SystemLedger
	err := batch.Account(s.partition.Ledger()).Main().GetAs(&ledger)
	switch {
	case err == nil:
		return ledger.Index, nil
	case errors.Is(err, errors.NotFound):
		return 0, nil
	default:
		return 0, errors.UnknownError.WithFormat("load the ledger: %w", err)
	}
}

// recordsBetween reads the block-ledger records of the blocks after after
// through through. A block with no record changed nothing.
func (s *Querier) recordsBetween(batch *database.Batch, after, through uint64) ([]*api.BlockLedgerRecord, error) {
	ledger := batch.Account(s.partition.Ledger())
	var records []*api.BlockLedgerRecord
	for n := after + 1; n <= through; n++ {
		_, entries, err := indexing.LoadBlockLedger(ledger, n)
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			continue
		default:
			return nil, errors.UnknownError.WithFormat("load the record of block %d: %w", n, err)
		}
		records = append(records, &api.BlockLedgerRecord{Index: n, Entries: entries})
	}
	return records, nil
}

// statesPage answers the accounts from cur on, as many whole as fit the
// budget. An account that does not fit a page of its own is answered in
// parts, one part a page, and a page that carries a part carries nothing else.
func (s *Querier) statesPage(ctx context.Context, batch *database.Batch, accounts []*url.URL, cur statesCursor) (*api.AccountStatesRecord, error) {
	peers := s.redirectOnce(ctx)
	return s.statesPageWith(accounts, cur, func(u *url.URL) *api.AccountState { return s.accountState(batch, u, peers) })
}

// redirectOnce is the peers a page names for every account it cannot answer
// because its section is not loaded, asked of the network at most once a
// page and only when such an account is met.
func (s *Querier) redirectOnce(ctx context.Context) func() []string {
	var peers []string
	asked := false
	return func() []string {
		if !asked && s.redirect != nil {
			asked = true
			peers = s.redirect(ctx)
		}
		return peers
	}
}

// notLoaded is the answer for an account in a section this view has not
// loaded: neither a state nor "no leaf" -- only execution decides that an
// account does not exist -- but the section, and the peers to ask instead
// (executor.md, "Sync", §3a, "What is not loaded is not served; the node
// redirects"; #4496). It is the per-account form of the refusal a query of
// the account gets (database.NotLoaded).
func notLoaded(u *url.URL, sec bpt.Section, peers func() []string) *api.AccountState {
	return &api.AccountState{Url: u, NotLoaded: true, Section: uint64(sec), Redirect: peers()}
}

// sectionNotLoaded reports whether the section of the key hash kh is not
// loaded: the tree get reads holds the section's marker. A leaf the tree does
// not hold is "no leaf" only in a section the tree holds whole.
func sectionNotLoaded(kh [32]byte, get func(key *record.Key) error) (bpt.Section, bool) {
	sec := bpt.SectionOf(kh)
	return sec, get(sec.MarkerKey()) == nil
}

// statesPageWith is statesPage with the state of each account read by read.
func (s *Querier) statesPageWith(accounts []*url.URL, cur statesCursor, read func(*url.URL) *api.AccountState) (*api.AccountStatesRecord, error) {
	r := new(api.AccountStatesRecord)
	budget := accountStatesBudget
	used := 0
	for i := cur.index; i < uint64(len(accounts)); i++ {
		st := read(accounts[i])
		data, err := st.MarshalBinary()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("marshal the state of %v: %w", accounts[i], err)
		}

		if i == cur.index && (cur.offset > 0 || len(data) > budget) {
			// In parts. The part is of this view's state; the asker checks
			// the whole against its leaf.
			if cur.offset >= uint64(len(data)) {
				return nil, errors.BadRequest.WithFormat("the cursor names byte %d of a state of %d bytes", cur.offset, len(data))
			}
			end := min(cur.offset+uint64(budget), uint64(len(data)))
			r.Part = &api.AccountStatePart{Url: accounts[i], Offset: cur.offset, Total: uint64(len(data)), Data: data[cur.offset:end]}
			next := statesCursor{index: i, offset: end}
			if end == uint64(len(data)) {
				next = statesCursor{index: i + 1}
			}
			if next.index < uint64(len(accounts)) {
				r.Cursor = next.encode()
			}
			return r, nil
		}
		if used+len(data) > budget {
			r.Cursor = statesCursor{index: i}.encode()
			return r, nil
		}
		used += len(data)
		r.States = append(r.States, st)
	}
	return r, nil
}

// accountState is one account's sync state as this view holds it: what the
// account's leaf hashes (database.Account.LeafState, which reads the store as
// hashState does), the entries of each chain's open mark set, and the leaf
// itself (executor.md, "Sync", §3).
//
// What cannot be read of one account is that account's answer, not the
// page's: it is served with no body beside the leaf it has (or none), which
// the asker refuses as it refuses any account whose parts do not rebuild its
// leaf, and asks again. Failing the page for it would fail it on every peer,
// which all hold the same state, and pin the asker's records where they stand
// -- an account with a leaf and no main state (#4437) is such a case.
func (s *Querier) accountState(batch *database.Batch, u *url.URL, peers func() []string) *api.AccountState {
	st, err := s.readAccountState(batch, u)
	if err == nil && st.NoLeaf {
		sec, ok := sectionNotLoaded(batch.Account(u).Key().Hash(), func(k *record.Key) error {
			_, err := batch.BPT().Get(k)
			return err
		})
		if ok {
			return notLoaded(u, sec, peers)
		}
	}
	if err == nil {
		return st
	}
	if d, ok := database.NotLoadedOf(err); ok {
		// The leaf is there, the records behind it are not: the gate
		// refused them.
		return notLoaded(u, bpt.Section(*d.Section), peers)
	}
	s.logger.Info("An account's sync state could not be read; it is served without its body", "account", u, "error", err)
	out := &api.AccountState{Url: u}
	if st != nil {
		out.LeafHash = st.LeafHash
	}
	return out
}

// readAccountState reads one account's sync state. On an error it returns
// what it read so far.
func (s *Querier) readAccountState(batch *database.Batch, u *url.URL) (*api.AccountState, error) {
	st := &api.AccountState{Url: u}
	record := batch.Account(u)
	leafHash, err := batch.BPT().Get(record.Key())
	switch {
	case err == nil:
	case errors.Is(err, errors.NotFound):
		st.NoLeaf = true
		return st, nil
	default:
		return st, errors.UnknownError.WithFormat("load the leaf: %w", err)
	}
	if len(leafHash) != 32 {
		return st, errors.InternalError.WithFormat("the leaf is %d bytes", len(leafHash))
	}
	st.LeafHash = [32]byte(leafHash)

	st.Main, err = record.Main().Get()
	if err != nil {
		return st, errors.UnknownError.WithFormat("load the main state: %w", err)
	}
	leaf, err := record.LeafState()
	if err != nil {
		return st, errors.UnknownError.Wrap(err)
	}
	st.Directory = leaf.Directory

	l := new(api.AccountLeaf)
	l.EventsRoot = leaf.EventsRoot
	l.LocalDeliveryQueue = leaf.LocalDeliveryQueue
	l.CascadeDeliveryQueue = leaf.CascadeDeliveryQueue
	if u.Equal(s.partition.Ledger()) {
		l.Events, err = ledgerEvents(record.Events())
		if err != nil {
			return st, errors.UnknownError.WithFormat("load scheduled events: %w", err)
		}
	}
	for _, c := range leaf.Chains {
		chain, err := record.ChainByName(c.Name)
		if err != nil {
			return st, errors.UnknownError.WithFormat("get chain %s: %w", c.Name, err)
		}
		l.Chains = append(l.Chains, &api.ChainRecord{Name: c.Name, Type: chain.Type(), Count: c.Count, State: c.Pending})

		inner := chain.Inner()
		head, err := inner.Head().Get()
		if err != nil {
			return st, errors.UnknownError.WithFormat("load the head of chain %s: %w", c.Name, err)
		}
		open, err := inner.OpenSet(head)
		if err != nil {
			return st, errors.UnknownError.WithFormat("load the open mark set of chain %s: %w", c.Name, err)
		}
		st.OpenEntries = append(st.OpenEntries, &api.ChainOpenEntries{Name: c.Name, Entries: open})
	}
	for _, p := range leaf.Pending {
		v, err := pendingSets(p)
		if err != nil {
			return st, errors.UnknownError.Wrap(err)
		}
		l.Pending = append(l.Pending, v)
	}
	for _, p := range leaf.BookPending {
		v, err := pendingSets(p)
		if err != nil {
			return st, errors.UnknownError.Wrap(err)
		}
		l.BookPending = append(l.BookPending, v)
	}
	st.Leaf = l
	return st, nil
}

// accountStateAt is one account's sync state as this node retained it for
// block at (executor.md, "Sync", §3a; #4485): the body and the rest of the
// leaf retained when the account last changed at or before the block
// (indexing.HistoricalAccountStateProof), the leaf hash from the tree's
// history, and each chain's open mark set as of the head retained -- the
// entries below a head never change, so the chain as it stands now holds
// them. An account that did not exist at the block is served with NoLeaf;
// one the node cannot answer as of the block -- a state it did not retain,
// or a partition ledger, whose scheduled events are not retained -- with no
// body, which the asker refuses and asks the next peer for.
func (s *Querier) accountStateAt(batch *database.Batch, u *url.URL, at uint64, peers func() []string) *api.AccountState {
	st, err := s.readAccountStateAt(batch, u, at)
	if err == nil && st.NoLeaf {
		root, _, rerr := indexing.BPTRootAt(s.partition, batch, at)
		if rerr == nil {
			sec, ok := sectionNotLoaded(batch.Account(u).Key().Hash(), func(k *record.Key) error {
				_, err := batch.BPT().GetReceiptAt(k, at, root)
				return err
			})
			if ok {
				return notLoaded(u, sec, peers)
			}
		}
	}
	if err == nil {
		return st
	}
	if d, ok := database.NotLoadedOf(err); ok {
		return notLoaded(u, bpt.Section(*d.Section), peers)
	}
	s.logger.Info("An account's sync state as of a block could not be read; it is served without its body", "account", u, "block", at, "error", err)
	return &api.AccountState{Url: u}
}

func (s *Querier) readAccountStateAt(batch *database.Batch, u *url.URL, at uint64) (*api.AccountState, error) {
	st := &api.AccountState{Url: u}
	if u.Equal(s.partition.Ledger()) {
		return nil, errors.IncompleteChain.WithFormat("the scheduled events of %v are not retained by block", u)
	}
	record := batch.Account(u)
	root, _, err := indexing.BPTRootAt(s.partition, batch, at)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	full, err := batch.BPT().GetReceiptAt(record.Key(), at, root)
	switch {
	case err == nil:
	case errors.Is(err, errors.NotFound):
		st.NoLeaf = true
		return st, nil
	default:
		return nil, errors.UnknownError.WithFormat("the leaf as of %d: %w", at, err)
	}
	if len(full.Start) != 32 {
		return nil, errors.InternalError.WithFormat("the leaf is %d bytes", len(full.Start))
	}
	st.LeafHash = [32]byte(full.Start)

	proof, err := indexing.HistoricalAccountStateProof(s.partition, batch, record, at)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if proof.Leaf == nil {
		return nil, errors.IncompleteChain.WithFormat("%v: the rest of the leaf as of %d is not retained", u, at)
	}
	st.Main = proof.State
	st.Directory = proof.Leaf.Directory

	l := new(api.AccountLeaf)
	l.EventsRoot = proof.Leaf.EventsRoot
	l.LocalDeliveryQueue = proof.Leaf.LocalDeliveryQueue
	l.CascadeDeliveryQueue = proof.Leaf.CascadeDeliveryQueue
	for _, c := range proof.Leaf.Chains {
		chain, err := record.ChainByName(c.Name)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get chain %s: %w", c.Name, err)
		}
		l.Chains = append(l.Chains, &api.ChainRecord{Name: c.Name, Type: chain.Type(), Count: c.Count, State: c.Pending})
		open, err := chain.Inner().OpenSet(&merkle.State{Count: int64(c.Count), Pending: c.Pending})
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load the open mark set of chain %s as of %d: %w", c.Name, at, err)
		}
		st.OpenEntries = append(st.OpenEntries, &api.ChainOpenEntries{Name: c.Name, Entries: open})
	}
	for _, p := range proof.Leaf.Pending {
		v, err := pendingSets(p)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		l.Pending = append(l.Pending, v)
	}
	for _, p := range proof.Leaf.BookPending {
		v, err := pendingSets(p)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		l.BookPending = append(l.BookPending, v)
	}
	st.Leaf = l
	return st, nil
}

// statesCursor is where the next page of an answer starts: the records' end
// the answer is fixed to, the account, and the byte of its state a part
// continues from.
type statesCursor struct {
	index, offset uint64
}

func (c statesCursor) encode() []byte {
	b := make([]byte, 0, 2*binary.MaxVarintLen64)
	b = binary.AppendUvarint(b, c.index)
	b = binary.AppendUvarint(b, c.offset)
	return b
}

func decodeStatesCursor(b []byte) (statesCursor, error) {
	var c statesCursor
	if len(b) == 0 {
		return c, nil
	}
	for _, v := range []*uint64{&c.index, &c.offset} {
		x, n := binary.Uvarint(b)
		if n <= 0 {
			return c, errors.BadRequest.With("malformed cursor")
		}
		*v, b = x, b[n:]
	}
	if len(b) != 0 {
		return c, errors.BadRequest.With("malformed cursor")
	}
	return c, nil
}
