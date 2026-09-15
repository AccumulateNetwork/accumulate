// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAnchorCopiesStoreBodyOnce: N validator copies of one anchor arriving in
// one block store the anchor transaction once, under its own hash; each copy
// is stored as its signature over a placeholder; and the validator signature
// set is written once for the block (#4224).
func TestAnchorCopiesStoreBodyOnce(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	const bvnCount, valCount = 1, 3
	rec := newRecordingStores()
	opts := []simulator.Option{
		simulator.SimpleNetwork(t.Name(), bvnCount, valCount),
		simulator.Genesis(GenesisTime),
		simulator.WithDatabase(rec.open),
	}

	// Capture the BVN's anchors to the Directory, one copy per signer, and
	// keep them from being delivered
	var anchorsMu sync.Mutex
	var anchors []*messaging.BlockAnchor
	opts = append(opts, simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
		anchorsMu.Lock()
		defer anchorsMu.Unlock()
		for _, m := range env.Messages {
			blk, ok := m.(*messaging.BlockAnchor)
			if !ok {
				continue
			}
			seq := blk.Anchor.(*messaging.SequencedMessage)
			txn := seq.Message.(*messaging.TransactionMessage)
			anchor, ok := txn.Transaction.Body.(*BlockValidatorAnchor)
			if !ok || anchor.MinorBlockIndex <= 10 {
				continue
			}
			for _, have := range anchors {
				if bytes.Equal(have.Signature.GetPublicKey(), blk.Signature.GetPublicKey()) {
					return false, nil
				}
			}
			anchors = append(anchors, blk)
			return false, nil
		}
		return true, nil
	}))

	sim := NewSim(t, opts...)
	sim.SetRoute(alice, "BVN0")
	sim.StepN(50)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	sim.SubmitTxnSuccessfully(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateTokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(True(func(*Harness) bool {
		anchorsMu.Lock()
		defer anchorsMu.Unlock()
		return len(anchors) >= valCount
	}))

	txid := anchors[0].Anchor.(*messaging.SequencedMessage).Message.ID()
	txnHash := txid.Hash()

	// Every copy in one block, to the Directory. A submission is included in
	// the block after the next, so two steps; that the set is written once
	// below proves the copies landed in one block — a copy alone in a block
	// writes the set at that block's close.
	dn := rec.get(Directory, 0)
	dn.reset()
	for _, a := range anchors {
		sim.SubmitSuccessfully(&messaging.Envelope{Messages: []messaging.Message{a}})
	}
	sim.StepN(2)

	// The anchor executed in that block
	View(t, sim.DatabaseFor(txid.Account()), func(batch *database.Batch) {
		st, err := batch.Transaction(txnHash[:]).Status().Get()
		require.NoError(t, err)
		require.True(t, st.Delivered(), "the anchor should execute once the copies reach the quorum")
	})

	// The body is stored once, under the transaction's hash; no copy stores
	// it again
	var bodies, placeholders int
	for _, w := range dn.all() {
		if w.key.Len() != 3 || w.key.Get(0) != "Message" || w.key.Get(2) != "Main" {
			continue
		}
		msg, err := messaging.UnmarshalMessage(w.value)
		if err != nil {
			continue
		}
		switch msg := msg.(type) {
		case *messaging.TransactionMessage:
			if msg.Hash() == txnHash {
				bodies++
			}
		case *messaging.BlockAnchor:
			seq := msg.Anchor.(*messaging.SequencedMessage)
			txn := seq.Message.(*messaging.TransactionMessage)
			if txn.Transaction.Body.Type() == TransactionTypeRemote {
				placeholders++
			} else {
				t.Errorf("anchor copy from %x stored with the full body", msg.Signature.GetPublicKey()[:4])
			}
		}
	}
	require.Equal(t, 1, bodies, "the anchor transaction should be stored once")
	require.GreaterOrEqual(t, placeholders, 2, "each copy that counted should be stored as its signature")

	// The signature set is written once for the block
	var sets int
	for _, w := range dn.all() {
		if w.key.Len() == 5 && w.key.Get(0) == "Account" && w.key.Get(2) == "Transaction" && w.key.Get(3) == txnHash && w.key.Get(4) == "ValidatorSignatures" {
			sets++
		}
	}
	require.Equal(t, 1, sets, "the validator signature set should be written once per block")
}

// recordingStores wraps each node's memory store to record the values the
// database commits, by key.
type recordingStores struct {
	mu     sync.Mutex
	stores map[string]*recordingStore
}

func newRecordingStores() *recordingStores {
	return &recordingStores{stores: map[string]*recordingStore{}}
}

func (r *recordingStores) open(partition *PartitionInfo, node int, _ logging.Logger) keyvalue.Beginner {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := &recordingStore{Beginner: memory.New(nil), name: recordingKey(partition.ID, node)}
	if _, ok := r.stores[recordingKey(partition.ID, node)]; ok {
		panic("store opened twice for " + recordingKey(partition.ID, node))
	}
	r.stores[recordingKey(partition.ID, node)] = s
	return s
}

func (r *recordingStores) get(partition string, node int) *recordingStore {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stores[recordingKey(partition, node)]
}

func recordingKey(partition string, node int) string {
	return partition + "/" + string(rune('0'+node))
}

type recordedWrite struct {
	key   *record.Key
	value []byte
}

type recordingStore struct {
	keyvalue.Beginner
	name   string
	mu     sync.Mutex
	writes []recordedWrite
}

func (s *recordingStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = nil
}

func (s *recordingStore) all() []recordedWrite {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedWrite(nil), s.writes...)
}

func (s *recordingStore) record(w []recordedWrite) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = append(s.writes, w...)
}

func (s *recordingStore) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return &recordingChangeSet{ChangeSet: s.Beginner.Begin(prefix, writable), rec: s}
}

// recordingChangeSet records what is committed, not what is put: the
// simulator executes a block more than once and discards all but one.
type recordingChangeSet struct {
	keyvalue.ChangeSet
	rec     *recordingStore
	parent  *recordingChangeSet
	pending []recordedWrite
}

func (c *recordingChangeSet) Put(key *record.Key, value []byte) error {
	c.pending = append(c.pending, recordedWrite{key, value})
	return c.ChangeSet.Put(key, value)
}

func (c *recordingChangeSet) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return &recordingChangeSet{ChangeSet: c.ChangeSet.Begin(prefix, writable), rec: c.rec, parent: c}
}

func (c *recordingChangeSet) Commit() error {
	if c.parent != nil {
		c.parent.pending = append(c.parent.pending, c.pending...)
	} else {
		// One record per key per commit: the batch may put a key more than
		// once on its way down, and the store keeps the last
		seen := map[record.KeyHash]int{}
		var records []recordedWrite
		for _, w := range c.pending {
			if i, ok := seen[w.key.Hash()]; ok {
				records[i] = w
				continue
			}
			seen[w.key.Hash()] = len(records)
			records = append(records, w)
		}
		c.rec.record(records)
	}
	c.pending = nil
	return c.ChangeSet.Commit()
}

func (c *recordingChangeSet) Discard() {
	c.pending = nil
	c.ChangeSet.Discard()
}
