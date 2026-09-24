// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	apierrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// spineWithMessages builds the anchor pool the way execution leaves it: main
// chain entries that are transactions, and a signature chain entry that is an
// anchor stored referring to its transaction by hash (the executor's
// storedForm, #4236), with the transaction stored under its own hash and on
// no chain of the account.
func spineWithMessages(t *testing.T) (*database.Database, *url.URL, [][32]byte) {
	t.Helper()
	u := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	db := newObservedDB(t)
	b := db.Begin(true)
	defer b.Discard()
	require.NoError(t, b.Account(u).Main().Put(&protocol.AnchorLedger{Url: u}))

	var entries [][32]byte
	for i := 0; i < 3; i++ {
		entries = append(entries, addTransactionEntry(t, b, u, i, 0x42))
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = u
	txn.Body = &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.DnUrl(), MinorBlockIndex: 7}}
	full := &messaging.BlockAnchor{
		Signature: &protocol.ED25519Signature{PublicKey: make([]byte, 32), Signature: make([]byte, 64), Signer: protocol.DnUrl().JoinPath(protocol.Network), TransactionHash: txn.ID().Hash()},
		Anchor:    &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: protocol.DnUrl(), Destination: protocol.DnUrl(), Number: 1},
	}
	h := full.Hash()
	ref := new(protocol.Transaction)
	ref.Header.Principal = u
	ref.Body = &protocol.RemoteTransaction{Hash: txn.ID().Hash()}
	stored := &messaging.BlockAnchor{
		Signature: full.Signature,
		Anchor:    &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: ref}, Source: protocol.DnUrl(), Destination: protocol.DnUrl(), Number: 1},
	}
	require.NotEqual(t, h, stored.Hash(), "precondition: the stored form does not hash to its key")
	require.NoError(t, b.Message(h).Main().Put(stored))
	require.NoError(t, b.Message(txn.ID().Hash()).Main().Put(&messaging.TransactionMessage{Transaction: txn}))
	require.NoError(t, b.Account(u).SignatureChain().Inner().AddEntry(h[:], false))
	entries = append(entries, h)
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
	return db, u, entries
}

// TestFullSpine_TakesTheMessageBehindEachEntry — the first block a restarted
// process opens loads the message behind each entry of the anchor pool's
// chains. A pull that takes the entries as hashes leaves the node unable to
// open it (#4400). The messages come with the entries, a stored form's
// transaction with it, and the account still hashes to the peer's leaf.
func TestFullSpine_TakesTheMessageBehindEachEntry(t *testing.T) {
	src, u, entries := spineWithMessages(t)

	dst := newObservedDB(t)
	b := dst.Begin(true)
	require.NoError(t, Account(context.Background(), &dbSource{db: src}, b, u, Options{Mode: ModeFullSpine}))
	require.NoError(t, b.Commit())

	s := src.Begin(false)
	defer s.Discard()
	d := dst.Begin(false)
	defer d.Discard()
	for i, h := range entries {
		want, err := s.Message(h).Main().Get()
		require.NoError(t, err)
		got, err := d.Message(h).Main().Get()
		require.NoError(t, err, "entry %d names a message the node does not hold", i)
		require.True(t, messaging.EqualMessage(want, got), "entry %d: the node holds another message than the peer", i)
	}

	// The anchor's transaction, which the stored form refers to, is held
	// under its own hash, so the reference resolves locally.
	var anchor *messaging.BlockAnchor
	require.NoError(t, d.Message(entries[len(entries)-1]).Main().GetAs(&anchor))
	ref := anchor.Anchor.(*messaging.SequencedMessage).Message.(*messaging.TransactionMessage).Transaction.Body.(*protocol.RemoteTransaction)
	var txn *messaging.TransactionMessage
	require.NoError(t, d.Message(ref.Hash).Main().GetAs(&txn))
	require.IsType(t, (*protocol.DirectoryAnchor)(nil), txn.Transaction.Body)

	want, err := s.Account(u).Hash()
	require.NoError(t, err)
	got, err := d.Account(u).Hash()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// lying serves a spine whose entries are true and whose messages are not.
type lying struct {
	*dbSource
	serve func(e *api.ChainEntryRecord[api.Record])
	txn   func(*api.MessageRecord[messaging.Message])
}

func (l *lying) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	r, err := l.dbSource.QueryChainEntries(ctx, u, q)
	if err != nil || l.serve == nil {
		return r, err
	}
	for _, e := range r.Records {
		if e.Value != nil {
			l.serve(e)
		}
	}
	return r, nil
}

func (l *lying) QueryMessage(ctx context.Context, id *url.TxID, q *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error) {
	r, err := l.dbSource.QueryMessage(ctx, id, q)
	if err != nil || l.txn == nil {
		return r, err
	}
	l.txn(r)
	return r, nil
}

// TestFullSpine_RefusesAMessageThatIsNotItsEntry — the entry is under the
// proven root and the message is not, so the message is believed only if it
// hashes to its entry. A peer that serves another message, a transaction
// without its body, no message at all, or a stored form whose transaction is
// not the one it names, has not served the chain: nothing is kept from it and
// the next peer is asked.
func TestFullSpine_RefusesAMessageThatIsNotItsEntry(t *testing.T) {
	src, u, entries := spineWithMessages(t)

	other := new(protocol.Transaction)
	other.Header.Principal = u
	other.Body = &protocol.WriteData{Entry: &protocol.DoubleHashDataEntry{Data: [][]byte{[]byte("not the entry")}}}
	otherHash := other.ID().Hash()
	func() {
		// The peer holds the other transaction whole, so it can serve it.
		b := src.Begin(true)
		defer b.Discard()
		require.NoError(t, b.Message(otherHash).Main().Put(&messaging.TransactionMessage{Transaction: other}))
		require.NoError(t, b.Commit())
	}()

	// The two cases below get past every check but the last: the stored
	// form's transaction is whole and hashes to what it names, and only the
	// hash of the whole wrapper, put back together, is not the entry
	// (reviewer, #4400 note_3896114642).
	anchorWith := func(e *api.ChainEntryRecord[api.Record], change func(*messaging.BlockAnchor)) {
		r, ok := e.Value.(*api.MessageRecord[messaging.Message])
		if !ok {
			return
		}
		ba, ok := r.Message.(*messaging.BlockAnchor)
		if !ok {
			return
		}
		c := *ba
		change(&c)
		r.Message = &c
	}

	cases := map[string]*lying{
		"the right transaction under another signature": {serve: func(e *api.ChainEntryRecord[api.Record]) {
			anchorWith(e, func(ba *messaging.BlockAnchor) {
				sig := *ba.Signature.(*protocol.ED25519Signature)
				sig.Signature = append([]byte(nil), sig.Signature...)
				sig.Signature[0] ^= 0xff
				ba.Signature = &sig
			})
		}},
		"a stored form naming another transaction the peer serves whole": {serve: func(e *api.ChainEntryRecord[api.Record]) {
			anchorWith(e, func(ba *messaging.BlockAnchor) {
				seq := *ba.Anchor.(*messaging.SequencedMessage)
				ref := new(protocol.Transaction)
				ref.Header.Principal = u
				ref.Body = &protocol.RemoteTransaction{Hash: otherHash}
				seq.Message = &messaging.TransactionMessage{Transaction: ref}
				ba.Anchor = &seq
			})
		}},
		"another message": {serve: func(e *api.ChainEntryRecord[api.Record]) {
			if r, ok := e.Value.(*api.MessageRecord[messaging.Message]); ok {
				if _, ok := r.Message.(*messaging.TransactionMessage); ok {
					r.Message = &messaging.TransactionMessage{Transaction: other}
				}
			}
		}},
		"a transaction without its body": {serve: func(e *api.ChainEntryRecord[api.Record]) {
			if r, ok := e.Value.(*api.MessageRecord[messaging.Message]); ok {
				if _, ok := r.Message.(*messaging.TransactionMessage); ok {
					stub := new(protocol.Transaction)
					stub.Header.Principal = u
					stub.Body = &protocol.RemoteTransaction{Hash: e.Entry}
					r.Message = &messaging.TransactionMessage{Transaction: stub}
				}
			}
		}},
		"no message": {serve: func(e *api.ChainEntryRecord[api.Record]) {
			e.Value = &api.ErrorRecord{Value: apierrors.NotFound.With("message not found")}
		}},
		"a stored form whose transaction is not its own": {txn: func(r *api.MessageRecord[messaging.Message]) {
			r.Message = &messaging.TransactionMessage{Transaction: other}
		}},
	}
	for name, liar := range cases {
		t.Run(name, func(t *testing.T) {
			liar.dbSource = &dbSource{db: src}
			dst := newObservedDB(t)
			b := dst.Begin(true)
			defer b.Discard()

			_, _, err := FetchFrom(context.Background(), []Source{liar}, b, u, Options{Mode: ModeFullSpine})
			require.Error(t, err, "a peer that served %s was believed", name)
			for i, h := range entries {
				_, err := b.Message(h).Main().Get()
				require.Error(t, err, "entry %d: a refused peer's message was kept", i)
			}
			_, err = b.Message(otherHash).Main().Get()
			require.Error(t, err, "a transaction a refused peer served was kept")

			p, i, err := FetchFrom(context.Background(), []Source{liar, &dbSource{db: src}}, b, u, Options{Mode: ModeFullSpine})
			require.NoError(t, err)
			require.Equal(t, 1, i, "the next peer answered")
			require.NoError(t, p.Keep())
			for i, h := range entries {
				_, err := b.Message(h).Main().Get()
				require.NoError(t, err, "entry %d: the honest peer's message was not kept", i)
			}
		})
	}
}

// TestFullSpine_AMessageOnTwoAccountsSettlesTwice — one transaction is an
// entry on several spine accounts' chains (a change to the validator set is on
// the network definition's, the operators' and the ledger's), and one pass
// holds all of them at once. A message is not an account's: two pending
// accounts that each wrote it into their own batch conflicted when the second
// settled, and the spine never verified
// (TestAJoinCrossesAChangeToTheValidatorSet).
func TestFullSpine_AMessageOnTwoAccountsSettlesTwice(t *testing.T) {
	a := protocol.DnUrl().JoinPath(protocol.Network)
	b := protocol.DnUrl().JoinPath(protocol.Globals)
	src := newObservedDB(t)
	func() {
		batch := src.Begin(true)
		defer batch.Discard()
		txn := new(protocol.Transaction)
		txn.Header.Principal = a
		txn.Body = &protocol.WriteData{Entry: &protocol.DoubleHashDataEntry{Data: [][]byte{[]byte("shared")}}}
		msg := &messaging.TransactionMessage{Transaction: txn}
		h := msg.Hash()
		require.NoError(t, batch.Message(h).Main().Put(msg))
		for _, u := range []*url.URL{a, b} {
			require.NoError(t, batch.Account(u).Main().Put(&protocol.DataAccount{Url: u}))
			require.NoError(t, batch.Account(u).MainChain().Inner().AddEntry(h[:], false)) // the same transaction on both
		}
		require.NoError(t, batch.Commit())
	}()

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()
	var held []*Pending
	for _, u := range []*url.URL{a, b} {
		p, _, err := FetchFrom(context.Background(), []Source{&dbSource{db: src}}, batch, u, Options{Mode: ModeFullSpine})
		require.NoError(t, err)
		held = append(held, p)
	}
	for _, p := range held {
		require.NoError(t, p.Keep(), "%v", p.Account)
	}
	require.NoError(t, batch.Commit())
}

// TestFullSpine_ARefusedAccountKeepsNoMessage — the messages behind an
// account's entries are written with the account and discarded with it
// (executor.md, "Sync" §3). They are proven by their own hashes, so nothing
// false would land; but an account whose state does not hash into the
// anchored root was not the network's, and neither is what came with it
// (review finding 3, note_3896114642).
func TestFullSpine_ARefusedAccountKeepsNoMessage(t *testing.T) {
	src, u, entries := spineWithMessages(t)
	ro := src.Begin(false)
	root, err := ro.GetBptRootHash()
	ro.Discard()
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		root [32]byte
		kept bool
	}{
		{"a root the state does not hash into", [32]byte{1}, false},
		{"the root the state hashes into", root, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := newObservedDB(t)
			b := dst.Begin(true)
			defer b.Discard()
			p, _, err := FetchFrom(context.Background(), []Source{&peer{dbSource: &dbSource{db: src}}}, b, u, Options{
				Mode:      ModeFullSpine,
				Verify:    anchored{root: tc.root},
				Partition: protocol.DnUrl(),
			})
			require.NoError(t, err)
			err = p.Settle(tc.root)
			if !tc.kept {
				require.Error(t, err, "precondition: the account is refused")
			} else {
				require.NoError(t, err)
			}
			for i, h := range entries {
				_, err := b.Message(h).Main().Get()
				if tc.kept {
					require.NoError(t, err, "entry %d: a settled account's message was not kept", i)
				} else {
					require.Error(t, err, "entry %d: a refused account's message was kept", i)
				}
			}
		})
	}
}

// askCounting counts the transactions a peer is asked for by hash.
type askCounting struct {
	*dbSource
	asked int
}

func (c *askCounting) QueryMessage(ctx context.Context, id *url.TxID, q *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error) {
	c.asked++
	return c.dbSource.QueryMessage(ctx, id, q)
}

// TestFullSpine_AStoredFormsTransactionTheNodeHoldsIsNotAskedFor — a stored
// form whose transaction the node already holds (it executed the anchor
// before its gap) is resolved from the node's own store, not by asking the
// peer again (review finding 4, note_3896114642).
func TestFullSpine_AStoredFormsTransactionTheNodeHoldsIsNotAskedFor(t *testing.T) {
	src, u, entries := spineWithMessages(t)

	// The anchor's transaction, as the stored form names it.
	var anchor *messaging.BlockAnchor
	func() {
		b := src.Begin(false)
		defer b.Discard()
		require.NoError(t, b.Message(entries[len(entries)-1]).Main().GetAs(&anchor))
	}()
	ref := anchor.Anchor.(*messaging.SequencedMessage).Message.(*messaging.TransactionMessage).Transaction.Body.(*protocol.RemoteTransaction)
	var txn *messaging.TransactionMessage
	func() {
		b := src.Begin(false)
		defer b.Discard()
		require.NoError(t, b.Message(ref.Hash).Main().GetAs(&txn))
	}()

	for _, holds := range []bool{false, true} {
		dst := newObservedDB(t)
		b := dst.Begin(true)
		if holds {
			require.NoError(t, b.Message(ref.Hash).Main().Put(txn))
		}
		peer := &askCounting{dbSource: &dbSource{db: src}}
		p, _, err := FetchFrom(context.Background(), []Source{peer}, b, u, Options{Mode: ModeFullSpine})
		require.NoError(t, err)
		require.NoError(t, p.Keep())
		if holds {
			require.Zero(t, peer.asked, "the node holds the transaction and asked the peer for it anyway")
		} else {
			require.NotZero(t, peer.asked, "precondition: a node without the transaction asks for it")
		}
		b.Discard()
	}
}

// TestFullSpine_ALocalStubIsNotABody — the node's own copy of a transaction
// a stored form refers to is held to the check a peer's is: a stub under the
// transaction's hash is not its body, so the node asks the peer rather than
// resolving the stored form with it and refusing an honest peer (re-check
// finding 3, note_3896174331).
func TestFullSpine_ALocalStubIsNotABody(t *testing.T) {
	src, u, entries := spineWithMessages(t)
	var anchor *messaging.BlockAnchor
	func() {
		b := src.Begin(false)
		defer b.Discard()
		require.NoError(t, b.Message(entries[len(entries)-1]).Main().GetAs(&anchor))
	}()
	ref := anchor.Anchor.(*messaging.SequencedMessage).Message.(*messaging.TransactionMessage).Transaction

	dst := newObservedDB(t)
	b := dst.Begin(true)
	defer b.Discard()
	body := ref.Body.(*protocol.RemoteTransaction)
	require.NoError(t, b.Message(body.Hash).Main().Put(&messaging.TransactionMessage{Transaction: ref}), "the node holds a stub under the transaction's hash")

	peer := &askCounting{dbSource: &dbSource{db: src}}
	p, _, err := FetchFrom(context.Background(), []Source{peer}, b, u, Options{Mode: ModeFullSpine})
	require.NoError(t, err, "an honest peer was refused because the node's own stub was taken for the body")
	require.NotZero(t, peer.asked, "the node took its own stub for the transaction's body")
	require.NoError(t, p.Keep())

	var txn *messaging.TransactionMessage
	require.NoError(t, b.Message(body.Hash).Main().GetAs(&txn))
	require.NotEqual(t, protocol.TransactionTypeRemote, txn.Transaction.Body.Type(), "the peer's whole transaction replaces the stub")
}
