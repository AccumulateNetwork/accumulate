// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// capture collects log records so a test can assert on what an operator sees.
type capture struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (h *capture) Enabled(context.Context, slog.Level) bool { return true }
func (h *capture) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *capture) WithGroup(string) slog.Handler            { return h }

func (h *capture) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, r.Clone())
	return nil
}

func (h *capture) matching(msg string) []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.recs {
		if strings.Contains(r.Message, msg) {
			out = append(out, r)
		}
	}
	return out
}

func captureWarnings(t *testing.T) *capture {
	t.Helper()
	h := &capture{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

func attrsOf(r slog.Record) map[string]string {
	out := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		out[a.Key] = a.Value.String()
		return true
	})
	return out
}

// A transaction that reaches execution and cannot be parsed must be reported,
// not swallowed.
//
// It used to log at Debug, which is filtered out at the level these nodes run
// at — so a committed transaction could be dropped by the executor and leave
// no trace at all. That is one of the candidate explanations for the 95
// transactions that vanished between acceptance and execution in run
// 20260822T061030Z (#4132), and it must not be the one that hides itself.
func TestProduceBlock_UnparseableTransactionIsReportedNotSwallowed(t *testing.T) {
	logs := captureWarnings(t)

	bridge := newBridge(t, new(fakeExec))

	garbage := []byte{0xde, 0xad, 0xbe, 0xef}
	_, err := bridge.ProduceBlock(context.Background(), BlockParams{
		Index:   9,
		Time:    time.Unix(100, 0),
		Batches: []*types.Batch{types.NewBatch([][]byte{garbage})},
	})
	require.NoError(t, err, "one bad transaction must not fail the block")

	require.NotEmpty(t, logs.matching("could not be unmarshalled"),
		"a committed transaction the executor cannot parse is data loss and must be logged at warn")
}

// Every block that carried transactions accounts for them, so "what reached
// execution" can be compared with "what was submitted" without grepping.
func TestProduceBlock_AccountsForArrivedVersusExecuted(t *testing.T) {
	logs := captureWarnings(t)

	bridge := newBridge(t, new(fakeExec))
	good := envBytes(t, 1)
	garbage := []byte{0x01, 0x02}

	_, err := bridge.ProduceBlock(context.Background(), BlockParams{
		Index:   10,
		Time:    time.Unix(100, 0),
		Batches: []*types.Batch{types.NewBatch([][]byte{good, garbage})},
	})
	require.NoError(t, err)

	recs := logs.matching("Block execution accounting")
	require.NotEmpty(t, recs, "a block carrying transactions must account for them")
	at := attrsOf(recs[0])
	require.Equal(t, "2", at["arrived"], "both transactions arrived")
	require.Equal(t, "1", at["executed"], "only the parseable one executed")
	require.Equal(t, "1", at["unmarshalFailed"])
}

// And it says WHICH CHAIN's block it is accounting for.
//
// A node runs the Directory and a BVN in one process, and this line carried
// no partition — so the two chains' block numbers interleaved in one stream
// and anyone deriving a height from it was silently mixing them. From
// acc-bvn1-val2's log on 2026-09-19, three consecutive lines:
//
//	block=3686 round=8828
//	block=4141 round=8852
//	block=4142 round=8856
//
// That is the Directory at 3686 and BVN1 at 4141, and nothing in the line
// says so. b.partitionID was already in scope four lines above (#4345c).
func TestProduceBlock_AccountingNamesItsPartition(t *testing.T) {
	logs := captureWarnings(t)

	bridge := newBridge(t, new(fakeExec))
	_, err := bridge.ProduceBlock(context.Background(), BlockParams{
		Index:   12,
		Time:    time.Unix(100, 0),
		Batches: []*types.Batch{types.NewBatch([][]byte{envBytes(t, 1)})},
	})
	require.NoError(t, err)

	recs := logs.matching("Block execution accounting")
	require.NotEmpty(t, recs)
	at := attrsOf(recs[0])
	require.Equal(t, "bvn1", at["partition"],
		"the accounting line does not say which chain's block it is (#4345c)")
}

// An empty block says nothing — most blocks on an idle network are empty and
// the accounting must not become its own flood.
func TestProduceBlock_EmptyBlockIsSilent(t *testing.T) {
	logs := captureWarnings(t)

	bridge := newBridge(t, new(fakeExec))
	_, err := bridge.ProduceBlock(context.Background(), BlockParams{
		Index: 11, Time: time.Unix(100, 0),
	})
	require.NoError(t, err)
	require.Empty(t, logs.matching("Block execution accounting"))
}

// The hand-off accounting is exported, not only logged. It was computed per
// block and written to a log line; nothing could chart "arrived minus
// executed", which is the question #4132 exists to answer, so a soak could
// not show whether transactions were being dropped (#4279 review).
func TestProduceBlock_HandoffAccountingIsExported(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(metrics.HandoffTotal)
	metrics.HandoffTotal.Reset()

	f := new(fakeExec)
	// statusFailed counts a status carrying an Error, not merely a code.
	f.statuses = []*protocol.TransactionStatus{{Error: errors.BadRequest.With("refused")}}
	bridge := newBridge(t, f)

	_, err := bridge.ProduceBlock(context.Background(), BlockParams{
		Index: 8, Time: time.Unix(100, 0),
		Batches: []*types.Batch{types.NewBatch([][]byte{
			envBytes(t, 1),
			[]byte("not an envelope"), // unmarshal-failed
			envBytes(t, 2),
		})},
	})
	require.NoError(t, err)

	got := map[string]float64{}
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != "accumulate_dagbft_handoff_transactions_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			var outcome string
			for _, l := range m.GetLabel() {
				if l.GetName() == "outcome" {
					outcome = l.GetValue()
				}
			}
			got[outcome] = m.GetCounter().GetValue()
		}
	}
	require.Equal(t, float64(3), got["arrived"], "every transaction in the batch arrived")
	require.Equal(t, float64(2), got["executed"], "the garbage one did not execute")
	require.Equal(t, float64(1), got["unmarshal-failed"])
	require.Equal(t, float64(2), got["status-failed"], "a status error per executed envelope")
	require.Equal(t, got["arrived"]-got["executed"], got["unmarshal-failed"],
		"arrived minus executed is accounted for")
}
