// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// captureHandler collects log records so a test can assert on what an operator
// would actually see.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *captureHandler) WithGroup(string) slog.Handler            { return h }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) matching(msg string) []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.records {
		if strings.Contains(r.Message, msg) {
			out = append(out, r)
		}
	}
	return out
}

func attrsOf(r slog.Record) map[string]string {
	out := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		out[a.Key] = a.Value.String()
		return true
	})
	return out
}

// captureLogs redirects slog for the duration of a test.
func captureLogs(t *testing.T) *captureHandler {
	t.Helper()
	h := &captureHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

func testNode(t *testing.T, partition string) *consensus.Node {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 100}}, 1)
	node, err := consensus.NewNode(consensus.NodeConfig{
		Partition: partition,
		KeyPair:   priv,
	}, committee, nil, nil)
	require.NoError(t, err)
	return node
}

// certFor builds a certificate whose payload names the given digests.
func certFor(t *testing.T, round types.Round, digests ...types.BatchDigest) *types.Certificate {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	payload := make([]types.PayloadEntry, 0, len(digests))
	for _, d := range digests {
		payload = append(payload, types.PayloadEntry{Digest: d, Worker: 0})
	}
	return &types.Certificate{Header: types.NewHeader(pub, round, 0, payload, nil)}
}

// CollectBatches must never return a partial set: a certificate executed
// without some of its batches diverges this node's state from every node that
// had them. When the batch cannot be found it waits, and the caller's context
// is what ends the wait.
func TestCollectBatches_NeverReturnsPartial(t *testing.T) {
	captureLogs(t)
	node := testNode(t, "Directory")

	missing := types.NewBatch([][]byte{[]byte("never-delivered")})
	cert := certFor(t, 246, missing.Digest())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	batches, err := node.CollectBatches(ctx, cert)
	require.Error(t, err, "collection must fail rather than return a short set")
	assert.Nil(t, batches)
	assert.Contains(t, err.Error(), "246", "the error should name the round that is stuck")
	assert.Contains(t, err.Error(), "1 still missing")
}

// A batch the node holds is returned in payload order without any waiting.
func TestCollectBatches_ReturnsStoredBatches(t *testing.T) {
	captureLogs(t)
	node := testNode(t, "Directory")

	b := types.NewBatch([][]byte{[]byte("tx-1")})
	require.NoError(t, node.Workers()[0].StoreBatch(b))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	batches, err := node.CollectBatches(ctx, certFor(t, 10, b.Digest()))
	require.NoError(t, err)
	require.Len(t, batches, 1)
	assert.Equal(t, b.Digest(), batches[0].Digest())
}

// The wedge from #4125, end to end at the node level: a batch pruned by an
// earlier commit is named by a later certificate. The executor cannot proceed
// — that part is by design — but its log must say the batch was pruned and by
// which block, rather than repeating "missing=1".
func TestCollectBatches_ReportsThatTheBatchWasPruned(t *testing.T) {
	logs := captureLogs(t)
	node := testNode(t, "Directory")
	w := node.Workers()[0]

	shared := types.NewBatch([][]byte{[]byte("payment-1")})
	require.NoError(t, w.StoreBatch(shared))

	// An earlier certificate committed and the executor pruned its payload.
	w.PruneBatchesAt([]types.BatchDigest{shared.Digest()}, "block 2951 round 240")

	// A later certificate names the same digest.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := node.CollectBatches(ctx, certFor(t, 246, shared.Digest()))
	require.Error(t, err)

	waits := logs.matching("Waiting for batches")
	require.NotEmpty(t, waits, "a stalled collection must report itself")

	at := attrsOf(waits[0])
	assert.Equal(t, shared.Digest().String(), at["digest"], "the missing batch must be named")
	assert.Contains(t, at["absence"], worker.GonePruned,
		"the report must say the batch was pruned, not merely that it is missing")
	assert.Contains(t, at["absence"], "block 2951 round 240",
		"and name the commit that deleted it")
	assert.Equal(t, "246", at["round"])
}

// A batch that was never stored here reports as such, so it is not mistaken
// for one this node deleted.
func TestCollectBatches_ReportsNeverStored(t *testing.T) {
	logs := captureLogs(t)
	node := testNode(t, "BVN1")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := node.CollectBatches(ctx,
		certFor(t, 984, types.NewBatch([][]byte{[]byte("elsewhere")}).Digest()))
	require.Error(t, err)

	waits := logs.matching("Waiting for batches")
	require.NotEmpty(t, waits)
	assert.Equal(t, worker.GoneUnknown, attrsOf(waits[0])["absence"])
}

// The wait must not drown the log it is meant to flag. The 2026-08-21 halt
// emitted 190,500 identical lines in twelve minutes — about 4,500 a minute,
// unthrottled — which is the same defect #4123 fixed for the stall report.
// Retries continue at their own pace; only the logging is rate limited.
func TestCollectBatches_DoesNotFloodTheLog(t *testing.T) {
	logs := captureLogs(t)
	node := testNode(t, "Directory")

	// 2s of waiting at a 50ms retry is ~40 passes; the limit is one line per
	// 10s, so a single line is all an operator should get.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := node.CollectBatches(ctx,
		certFor(t, 246, types.NewBatch([][]byte{[]byte("gone")}).Digest()))
	require.Error(t, err)

	waits := logs.matching("Waiting for batches")
	assert.Len(t, waits, 1, "one line per stall window, not one per retry")

	// The line still has to show that this is not the first attempt.
	attempts := attrsOf(waits[0])["attempts"]
	assert.Equal(t, "1", attempts, "the first line reports the first attempt")
}
