// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// stopCapture records the `Stream stopped` lines a block writes.
type stopCapture struct{ lines []map[string]any }

func (c *stopCapture) Debug(string, ...interface{})       {}
func (c *stopCapture) Error(string, ...interface{})       {}
func (c *stopCapture) With(...interface{}) logging.Logger { return c }
func (c *stopCapture) Info(msg string, kv ...interface{}) {
	if msg != "Stream stopped" {
		return
	}
	f := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			f[k] = kv[i+1]
		}
	}
	c.lines = append(c.lines, f)
}

// withRealRunPath gives the simulation the executors the block's run goes
// through for a staged entry: MessageIsReady loads the held message from
// staging and the synthetic executor runs it. The sequenced layer stays the
// simulation's.
func (s *stagingSim) withRealRunPath() {
	for _, fn := range messageExecutors {
		typ, x := fn(s.x.ExecutorOptions)
		switch typ {
		case internal.MessageTypeMessageIsReady, messaging.MessageTypeSynthetic:
			s.x.messageExecutors[typ] = x
		}
	}
}

// executeRun runs the stream through Block.executeRuns, the production run,
// and returns how many entries delivered.
func (s *stagingSim) executeRun() int {
	s.t.Helper()
	pos, err := s.b.positionOf(s.str)
	require.NoError(s.t, err)
	run, _ := buildRun(pos, nil, 1024)
	require.NotEmpty(s.t, run, "staging offers the stream a run")
	results := make([]*execute.ProcessResult, 0)
	return s.b.executeRuns([]streamRun{{stream: s.str, run: run}}, results, map[int]bool{})
}

// #4423: the run stopped every block at an entry staging offered as runnable
// and wrote nothing, so a stream froze with waiting=0 and no line named the
// entry. A staged entry that runs without moving its stream is counted every
// time and logged once per stream and number, and again no more often than
// RunStoppedEvery while it stays stopped there.
func TestExecuteRuns_AStagedEntryThatDoesNotMoveTheStreamIsReported(t *testing.T) {
	s := newStagingSim(t, 6)
	s.withRealRunPath()
	c := new(stopCapture)
	s.x.logger.Set(c)
	t0 := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	s.b.Time = t0

	// 2..3 collected ahead of their anchor, with 1 missing; the anchor
	// validates them.
	s.packageArrives(1, 2, 4)
	s.newBlock()
	s.b.Time = t0
	s.anchorExecutes(4, s.rootAt(4))

	// Whatever the cause, the collected original's outer message reads as
	// delivered: the state #4423 left behind. Its run can then execute
	// nothing.
	st, err := s.batch.Transaction2(s.member(1).Hash()).Status().Get()
	require.NoError(t, err)
	st.Code = errors.Delivered
	require.NoError(t, s.batch.Transaction2(s.member(1).Hash()).Status().Put(st))

	s.newBlock()
	s.b.Time = t0
	s.packageArrives(0, 0, 4)
	require.Equal(t, uint64(1), s.delivered())

	metric := mExecRunStopped.WithLabelValues("synthetic", "not-delivered")
	before := testutil.ToFloat64(metric)

	s.newBlock()
	s.b.Time = t0.Add(time.Second)
	require.Zero(t, s.executeRun(), "the entry at 2 executes nothing")
	require.Equal(t, uint64(1), s.delivered())
	require.Equal(t, before+1, testutil.ToFloat64(metric), "counted")
	require.Len(t, c.lines, 1, "and named on the first block")
	require.EqualValues(t, 2, c.lines[0]["number"])
	require.Equal(t, "synthetic", c.lines[0]["ledger"])
	require.Equal(t, "delivered", c.lines[0]["status"], "with what its execution said of itself")

	// The next block, stopped at the same number within the minute: counted,
	// not logged again.
	s.newBlock()
	s.b.Time = t0.Add(30 * time.Second)
	require.Zero(t, s.executeRun())
	require.Equal(t, before+2, testutil.ToFloat64(metric))
	require.Len(t, c.lines, 1, "the same stop is not logged every block")

	// A minute on, still stopped there: logged again.
	s.newBlock()
	s.b.Time = t0.Add(61 * time.Second)
	require.Zero(t, s.executeRun())
	require.Len(t, c.lines, 2, "a stop that lasts is logged once a minute")
	require.EqualValues(t, 2, c.lines[1]["number"])
}

// The #4423 sequence through the production run (Block.executeRuns and
// MessageIsReady) rather than the simulation's: a byte-identical copy of a
// collected package, committed while its entries are not next, must not stop
// the run once they are.
func TestExecuteRuns_4423_DuplicateCopyAfterValidation_Delivers(t *testing.T) {
	s := newStagingSim(t, 6)
	s.withRealRunPath()
	c := new(stopCapture)
	s.x.logger.Set(c)

	s.packageArrives(1, 2, 4)
	s.newBlock()
	s.anchorExecutes(4, s.rootAt(4))
	s.newBlock()
	s.packageArrives(1, 2, 4) // the retried envelope
	s.newBlock()
	s.packageArrives(0, 0, 4)
	require.Equal(t, uint64(1), s.delivered())

	s.newBlock()
	require.Equal(t, 2, s.executeRun(), "2 and 3 deliver")
	require.Equal(t, uint64(3), s.delivered())
	require.Empty(t, c.lines, "and nothing stopped")
}
