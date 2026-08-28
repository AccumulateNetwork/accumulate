// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"context"
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
