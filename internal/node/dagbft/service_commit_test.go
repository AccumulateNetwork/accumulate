// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The integration layer was at 0.0% coverage (#4119, #4116). The commit
// pipeline tested here is where PruneBatches fires — the commit signal the
// worker re-proposal fix (f91427308) depends on: a batch pruned too broadly
// loses transactions, one pruned too narrowly is re-proposed forever.

// commitAdapter is a recording ConsensusAdapter.
type commitAdapter struct {
	mu     sync.Mutex
	blocks []adapter.BlockParams
	hash   [32]byte
}

func (a *commitAdapter) ProduceBlock(_ context.Context, params adapter.BlockParams) ([32]byte, error) {
	a.mu.Lock()
	a.blocks = append(a.blocks, params)
	a.mu.Unlock()
	return a.hash, nil
}
func (a *commitAdapter) ValidateTransaction([]byte) error                           { return nil }
func (a *commitAdapter) LastBlock() (uint64, [32]byte, error)                       { return 0, [32]byte{}, nil }
func (a *commitAdapter) LastMajorBlock() (uint64, time.Time, bool)                  { return 0, time.Time{}, false }
func (a *commitAdapter) StateHash() [32]byte                                        { return a.hash }
func (a *commitAdapter) Validators() []adapter.ValidatorInfo                        { return nil }
func (a *commitAdapter) OnValidatorSetChange(func([]adapter.ValidatorInfo, uint64)) {}

// newCommitService builds an unstarted Service with a real (unstarted)
// consensus node holding real workers, and a recording adapter.
func newCommitService(t *testing.T, numWorkers int) (*Service, *commitAdapter, ed25519.PublicKey) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 1}}, 1)
	nodeCfg := consensus.NodeConfig{
		Partition:  "bvn1",
		KeyPair:    priv,
		NumWorkers: numWorkers,
	}
	node, err := consensus.NewNode(nodeCfg, committee, nil, nil)
	require.NoError(t, err)

	ca := &commitAdapter{hash: [32]byte{0xAA}}
	svc, err := NewService(ServiceConfig{
		Partition:  &protocol.PartitionInfo{ID: "bvn1", Type: protocol.PartitionTypeBlockValidator},
		NodeConfig: nodeCfg,
		Adapter:    ca,
		EventBus:   events.NewBus(nil),
	})
	require.NoError(t, err)
	svc.node = node
	svc.committee = committee
	svc.ctx = context.Background()
	return svc, ca, pub
}

// commitCert builds a certificate whose payload references the given digests.
func commitCert(author ed25519.PublicKey, round types.Round, ts time.Time, payload []types.PayloadEntry) *types.Certificate {
	header := types.NewHeader(author, round, 1, payload, nil)
	header.Timestamp = ts.UnixNano()
	return &types.Certificate{Header: header}
}

// TestProcessCommittedCertificate_PrunesExactlyTheCommittedBatches is the
// commit↔re-proposal contract: batches named by the committed certificate are
// (a) handed to the executor in canonical payload order and (b) pruned from
// every worker — while batches NOT in the certificate survive, because
// pruning them would silently lose their transactions.
func TestProcessCommittedCertificate_PrunesExactlyTheCommittedBatches(t *testing.T) {
	svc, ca, author := newCommitService(t, 2)
	w0, w1 := svc.node.Workers()[0], svc.node.Workers()[1]

	committed0 := types.NewBatch([][]byte{[]byte("w0-committed")})
	committed1 := types.NewBatch([][]byte{[]byte("w1-committed")})
	survivor := types.NewBatch([][]byte{[]byte("not-in-this-certificate")})
	require.NoError(t, w0.StoreBatch(committed0))
	require.NoError(t, w1.StoreBatch(committed1))
	require.NoError(t, w0.StoreBatch(survivor))

	payload := []types.PayloadEntry{
		{Digest: committed0.Digest(), Worker: w0.ID()},
		{Digest: committed1.Digest(), Worker: w1.ID()},
	}

	err := svc.processCommittedCertificate(commitCert(author, 4, time.Unix(100, 0), payload))
	require.NoError(t, err)

	// (a) The executor got the batches, in canonical payload order.
	require.Len(t, ca.blocks, 1)
	got := ca.blocks[0].Batches
	require.Len(t, got, 2)
	require.Equal(t, committed0.Digest(), got[0].Digest(), "batches must arrive in canonical payload order")
	require.Equal(t, committed1.Digest(), got[1].Digest())
	require.Equal(t, uint64(1), ca.blocks[0].Index, "first block after index 0")

	// (b) Committed batches leave the ACTIVE store, which is what stops
	// re-proposal re-committing them forever...
	require.False(t, w0.HasBatch(committed0.Digest()),
		"a committed batch must leave the active store — or re-proposal re-commits it forever")
	require.False(t, w1.HasBatch(committed1.Digest()))

	// ...but stay fetchable, so a peer that missed this commit still has a
	// source. Deleting them outright stranded any node that fell behind and
	// halted the Directory permanently (#4125, #4128).
	require.True(t, w0.HasRetained(committed0.Digest()),
		"a committed batch must remain servable to peers catching up")
	b, _ := w0.GetBatch(committed0.Digest())
	require.NotNil(t, b, "retention must answer a fetch for a committed batch")

	// ...and only the committed ones: the survivor stays active, awaiting its
	// own certificate.
	require.True(t, w0.HasBatch(survivor.Digest()),
		"a batch outside the certificate must NOT be retired — its transactions would be lost")
	require.False(t, w0.HasRetained(survivor.Digest()))

	// State hash was stamped onto the certificate for cross-node comparison.
	// (The tracker records it; a diverging remote hash for the same round is
	// what halts the node.)
}

// TestProcessCommittedCertificate_BlockTimeFromCertificate pins #4054's time
// rule: block time derives from the certificate's header timestamp — never
// the local clock — and is clamped to be strictly increasing, so a bad author
// clock cannot move time backwards.
func TestProcessCommittedCertificate_BlockTimeFromCertificate(t *testing.T) {
	svc, ca, author := newCommitService(t, 1)
	w := svc.node.Workers()[0]

	mk := func(n byte) *types.Batch { return types.NewBatch([][]byte{{n}}) }

	// Block 1: timestamp from the certificate, verbatim.
	b1 := mk(1)
	require.NoError(t, w.StoreBatch(b1))
	t1 := time.Unix(0, 1_700_000_000_000_000_000)
	require.NoError(t, svc.processCommittedCertificate(commitCert(author, 2, t1,
		[]types.PayloadEntry{{Digest: b1.Digest(), Worker: w.ID()}})))
	require.True(t, ca.blocks[0].Time.Equal(t1.UTC()),
		"block time must come from the certificate header, not the local clock")

	// Block 2: an author clock running BACKWARDS must not move block time
	// backwards — it is clamped to strictly after the previous block.
	b2 := mk(2)
	require.NoError(t, w.StoreBatch(b2))
	t2 := t1.Add(-time.Hour)
	require.NoError(t, svc.processCommittedCertificate(commitCert(author, 4, t2,
		[]types.PayloadEntry{{Digest: b2.Digest(), Worker: w.ID()}})))
	require.True(t, ca.blocks[1].Time.After(ca.blocks[0].Time),
		"a backwards author clock must not move block time backwards")
}

// TestProcessCommittedCertificate_HaltedRefusesToExecute: once the service
// halts (state divergence), committed certificates must NOT keep executing —
// executing past a known divergence compounds it.
func TestProcessCommittedCertificate_HaltedRefusesToExecute(t *testing.T) {
	svc, ca, author := newCommitService(t, 1)
	w := svc.node.Workers()[0]

	b := types.NewBatch([][]byte{[]byte("tx")})
	require.NoError(t, w.StoreBatch(b))

	svc.mu.Lock()
	svc.halted = true
	svc.haltReason = &types.StateDivergenceError{Round: 2}
	svc.mu.Unlock()

	err := svc.processCommittedCertificate(commitCert(author, 2, time.Unix(100, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}}))
	require.Error(t, err, "a halted service must refuse to execute further commits")
	require.Empty(t, ca.blocks, "no block may be produced while halted")

	batch, _ := w.GetBatch(b.Digest())
	require.NotNil(t, batch, "nothing may be pruned while halted")
}

// TestRecordStateHash_DivergenceHalts: the state-hash tracker is the
// cross-node divergence tripwire — a remote validator reporting a DIFFERENT
// hash for the same round must halt this node rather than let two histories
// keep executing.
func TestRecordStateHash_DivergenceHalts(t *testing.T) {
	svc, _, _ := newCommitService(t, 1)

	remote, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	svc.RecordStateHash(6, 3, types.StateHash{0xAA})
	require.False(t, svc.IsHalted())

	// A matching remote hash is harmony.
	div := svc.stateHashTracker.RecordRemoteHash(6, remote, types.StateHash{0xAA})
	require.Nil(t, div)
	require.False(t, svc.IsHalted())

	// A mismatched remote hash for the same round is divergence — halt.
	div = svc.stateHashTracker.RecordRemoteHash(6, remote, types.StateHash{0xBB})
	require.NotNil(t, div, "a mismatched remote hash must be reported as divergence")
	svc.onStateDivergence(div)
	require.True(t, svc.IsHalted(), "state divergence must halt the node")
	require.Error(t, svc.HaltReason())
}

// TestSubmitterService_ErrorMapping: the submitter is the front door for
// every envelope entering a partition's DAG. A malformed envelope is refused
// as BadRequest before touching consensus; an envelope for a service whose
// node is not running is refused rather than silently absorbed. (The
// happy-path accept→batch→commit flow is covered by the commit-pipeline
// tests above and by pkg/consensus's multi-node tests.)
func TestSubmitterService_ErrorMapping(t *testing.T) {
	svc, _, _ := newCommitService(t, 1)
	sub := NewSubmitterService(SubmitterServiceParams{Service: svc})

	// Malformed envelope: refused with BadRequest at normalization.
	_, err := sub.Submit(context.Background(), &messaging.Envelope{
		TxHash: []byte{1, 2, 3}, // invalid hash length
	}, api.SubmitOptions{})
	require.Error(t, err, "a malformed envelope must be refused before touching consensus")

	// A well-formed envelope against a not-started node: refused, not lost.
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.PartitionUrl("bvn1").JoinPath("ledger")
	txn.Body = &protocol.SyntheticDepositCredits{}
	env := &messaging.Envelope{Messages: []messaging.Message{
		&messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: txn},
			Source:      protocol.PartitionUrl("bvn2"),
			Destination: protocol.PartitionUrl("bvn1"),
			Number:      1,
		},
	}}
	_, err = sub.Submit(context.Background(), env, api.SubmitOptions{})
	require.Error(t, err, "an unstarted node must refuse submissions, never absorb them silently")
}

// TestProcessCommittedCertificate_UnavailableBatchBlocksExecution pins the
// availability rule that replaced the old skip (9630ea564): a certificate
// whose batch this node does not hold is NOT executed as a subset —
// CollectBatches waits (and fetches from peers) until it exists. With no
// peers and no batch, the commit must fail on context expiry having produced
// no block and pruned nothing. Skipping instead is how six nodes at the same
// block index produced six different state hashes (#4116/#4119).
func TestProcessCommittedCertificate_UnavailableBatchBlocksExecution(t *testing.T) {
	svc, ca, author := newCommitService(t, 1)
	w := svc.node.Workers()[0]

	held := types.NewBatch([][]byte{[]byte("held")})
	require.NoError(t, w.StoreBatch(held))
	missing := types.NewBatch([][]byte{[]byte("nobody-has-this")})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	svc.ctx = ctx

	err := svc.processCommittedCertificate(commitCert(author, 4, time.Unix(100, 0),
		[]types.PayloadEntry{
			{Digest: held.Digest(), Worker: w.ID()},
			{Digest: missing.Digest(), Worker: w.ID()},
		}))
	require.Error(t, err, "a certificate with an unavailable batch must not execute")
	require.Empty(t, ca.blocks, "no partial block may be produced")

	b, _ := w.GetBatch(held.Digest())
	require.NotNil(t, b, "nothing may be pruned when the commit did not happen")
}
