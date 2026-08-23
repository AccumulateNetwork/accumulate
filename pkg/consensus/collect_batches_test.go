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
	return testNodeCfg(t, partition, worker.Config{})
}

func testNodeCfg(t *testing.T, partition string, wc worker.Config) *consensus.Node {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 100}}, 1)
	node, err := consensus.NewNode(consensus.NodeConfig{
		Partition:    partition,
		KeyPair:      priv,
		WorkerConfig: wc,
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

// Retention turns the #4125 halt into a non-event at the node level: a
// certificate that names a batch an EARLIER commit retired is still served,
// because the batch is kept fetchable for a while after it commits.
func TestCollectBatches_RetentionServesALaterCertificate(t *testing.T) {
	captureLogs(t)
	node := testNode(t, "Directory")
	w := node.Workers()[0]

	shared := types.NewBatch([][]byte{[]byte("payment-1")})
	require.NoError(t, w.StoreBatch(shared))
	w.PruneCommitted([]types.BatchDigest{shared.Digest()},
		worker.CommitInfo{Cert: "an-earlier-certificate", Detail: "block 2951 round 240"})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	batches, err := node.CollectBatches(ctx, certFor(t, 246, shared.Digest()))
	require.NoError(t, err, "retention should keep this collectable")
	require.Len(t, batches, 1)
	assert.Equal(t, shared.Digest(), batches[0].Digest())
}

// The second half of the fix, for when retention has already expired: a
// certificate whose missing batch was retired by THAT SAME certificate has
// already been executed here. Waiting would be a permanent halt, so it is
// reported as a re-delivery instead.
func TestCollectBatches_SkipsCertificateItAlreadyExecuted(t *testing.T) {
	logs := captureLogs(t)
	// Retention off, so the batch is genuinely gone and only the tombstone is
	// left to reason from — the state the Directory was actually in.
	node := testNodeCfg(t, "Directory", worker.Config{MaxRetainedBatches: -1})
	w := node.Workers()[0]

	b := types.NewBatch([][]byte{[]byte("payment-1")})
	require.NoError(t, w.StoreBatch(b))

	cert := certFor(t, 260, b.Digest())
	w.PruneCommitted([]types.BatchDigest{b.Digest()},
		worker.CommitInfo{Cert: cert.Digest().String(), Detail: "block 3114 round 260"})

	// The same certificate is delivered a second time.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, err := node.CollectBatches(ctx, cert)

	require.ErrorIs(t, err, consensus.ErrAlreadyExecuted)
	assert.Less(t, time.Since(start), time.Second,
		"it must return at once, not wait out the context")
	require.NotEmpty(t, logs.matching("already executed"))
}

// The skip is narrow on purpose. A batch missing for any OTHER reason must
// still be waited for: skipping there would execute a certificate without its
// transactions and diverge this node from every peer that had them.
func TestCollectBatches_DoesNotSkipWhenADifferentCertificateCommittedTheBatch(t *testing.T) {
	captureLogs(t)
	node := testNodeCfg(t, "Directory", worker.Config{MaxRetainedBatches: -1})
	w := node.Workers()[0]

	b := types.NewBatch([][]byte{[]byte("payment-1")})
	require.NoError(t, w.StoreBatch(b))
	w.PruneCommitted([]types.BatchDigest{b.Digest()},
		worker.CommitInfo{Cert: "a-different-certificate", Detail: "block 3114 round 240"})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := node.CollectBatches(ctx, certFor(t, 260, b.Digest()))

	require.Error(t, err)
	assert.NotErrorIs(t, err, consensus.ErrAlreadyExecuted,
		"a batch committed by someone else is not proof this certificate ran")
	assert.Contains(t, err.Error(), "still missing")
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

// A round does not identify a certificate: every validator authors a header
// per round, so several certificates can share round 260. The 2026-08-21 halt
// was read as "the same certificate delivered twice" purely because the
// pruning certificate and the waiting one were both round 260 — a reading the
// log could not distinguish from two certificates of that round sharing a
// batch. Those want different fixes, so the report names the certificate and
// its author, not just the round.
func TestCollectBatches_NamesTheCertificateNotJustTheRound(t *testing.T) {
	logs := captureLogs(t)
	node := testNodeCfg(t, "Directory", worker.Config{MaxRetainedBatches: -1})

	shared := types.NewBatch([][]byte{[]byte("payment-1")})
	require.NoError(t, node.Workers()[0].StoreBatch(shared))
	node.Workers()[0].PruneCommitted([]types.BatchDigest{shared.Digest()},
		worker.CommitInfo{
			Cert:   "a-different-certificate",
			Detail: "block 3114 round 260 cert aaaaaaaaaaaaaaaa author deadbeef",
		})

	// A DIFFERENT certificate, same round, different author.
	other := certFor(t, 260, shared.Digest())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := node.CollectBatches(ctx, other)
	require.Error(t, err)

	waits := logs.matching("Waiting for batches")
	require.NotEmpty(t, waits)
	at := attrsOf(waits[0])

	require.NotEmpty(t, at["cert"], "the waiting certificate must be identified")
	require.NotEmpty(t, at["author"], "and its author named")
	assert.Equal(t, other.Digest().String()[:16], at["cert"])

	// The decisive comparison: the certificate that pruned the batch is named
	// in the absence string, and it is NOT this one. Same round, different
	// certificate — which is the distinction the round alone cannot express.
	assert.Contains(t, at["absence"], "cert aaaaaaaaaaaaaaaa")
	assert.NotContains(t, at["absence"], at["cert"],
		"pruner and waiter are different certificates of the same round")
	assert.Equal(t, "260", at["round"])
}

// #4159: CollectBatches must not wait forever for batches that are gone from
// the whole network. When the bounded timeout elapses AND no peer ever served
// a missing batch (peerHits==0), it returns ErrBatchesUnrecoverable — never a
// partial set — so the caller halts cleanly and recovers by state-sync.
func TestCollectBatches_UnrecoverableAfterTimeout(t *testing.T) {
	captureLogs(t)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 100}}, 1)
	node, err := consensus.NewNode(consensus.NodeConfig{
		Partition:           "Directory",
		KeyPair:             priv,
		BatchCollectTimeout: 150 * time.Millisecond, // bound the wait for the test
	}, committee, nil, nil) // nil host → no peer fetch → peerHits stays 0
	require.NoError(t, err)

	missing := types.NewBatch([][]byte{[]byte("gone-from-every-node")})
	cert := certFor(t, 4, missing.Digest())

	// Generous context so the bounded TIMEOUT, not ctx, ends the wait.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	batches, err := node.CollectBatches(ctx, cert)
	require.Error(t, err)
	assert.Nil(t, batches, "never a partial set")
	assert.ErrorIs(t, err, consensus.ErrBatchesUnrecoverable)
	assert.Less(t, time.Since(start), 5*time.Second,
		"must give up at the bounded timeout, not wait for the context")
}
