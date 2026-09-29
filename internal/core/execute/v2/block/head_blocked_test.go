// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// headCapture records the `Stream head blocked` lines a block writes.
type headCapture struct{ lines []map[string]any }

func (c *headCapture) Debug(string, ...interface{})       {}
func (c *headCapture) Error(string, ...interface{})       {}
func (c *headCapture) With(...interface{}) logging.Logger { return c }
func (c *headCapture) Info(msg string, kv ...interface{}) {
	if msg != "Stream head blocked" {
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

// #4465: after a whole-network restart a stream held every number from
// Delivered+1 up and ran none, with waiting=0, and nothing said which of
// runnable's conditions refused the head. A held, unrunnable head is counted
// at every stage and logged once per stream, number and reason, naming the
// held hash, the validated hash (or none), and the proof and anchor it waits
// for.
func TestBuildRun_AHeldHeadThatMayNotRunIsReported(t *testing.T) {
	t.Run("collected under a package proof whose anchor has not executed", func(t *testing.T) {
		s := newStagingSim(t, 6)
		c := new(headCapture)
		s.x.logger.Set(c)
		t0 := time.Date(2026, 9, 29, 5, 0, 0, 0, time.UTC)
		s.b.Time = t0

		s.packageArrives(0, 1, 4)
		require.True(t, s.collected(1), "held ahead of its anchor")

		metric := mExecHeadBlocked.WithLabelValues("synthetic", headNotValidated)
		before := testutil.ToFloat64(metric)
		require.Empty(t, s.run())
		require.Equal(t, before+1, testutil.ToFloat64(metric), "counted")
		require.Len(t, c.lines, 1, "and named")
		l := c.lines[0]
		require.EqualValues(t, 1, l["number"])
		require.Equal(t, headNotValidated, l["reason"])
		h := s.seqs[0].Hash()
		require.Equal(t, hex.EncodeToString(h[:]), l["held"])
		require.Equal(t, "none", l["validated"])
		require.Equal(t, false, l["own-proof"])

		// The same stage again within the minute: counted, not logged again
		s.b.Time = t0.Add(30 * time.Second)
		require.Empty(t, s.run())
		require.Equal(t, before+2, testutil.ToFloat64(metric))
		require.Len(t, c.lines, 1, "the same block is not logged every stage")

		// Past the minute, logged again
		s.b.Time = t0.Add(2 * time.Minute)
		require.Empty(t, s.run())
		require.Len(t, c.lines, 2)

		// Its anchor executes: the head runs and nothing more is said
		s.anchorExecutes(4, s.rootAt(4))
		require.Equal(t, []uint64{1, 2}, s.run())
		require.Len(t, c.lines, 2)
	})

	t.Run("collected with its own proof whose anchor has not executed", func(t *testing.T) {
		s := newStagingSim(t, 6)
		c := new(headCapture)
		s.x.logger.Set(c)
		s.b.Time = time.Date(2026, 9, 29, 5, 0, 0, 0, time.UTC)

		m := s.memberWithProof(0, 3)
		s.process([]messaging.Message{m}, m)
		require.True(t, s.collected(1), "held ahead of its anchor")

		require.Empty(t, s.run())
		require.Len(t, c.lines, 1)
		l := c.lines[0]
		require.EqualValues(t, 1, l["number"])
		require.Equal(t, headOwnProofAnchorAbsent, l["reason"])
		require.Equal(t, "none", l["validated"])
		require.Equal(t, true, l["own-proof"])
		require.Equal(t, hex.EncodeToString(s.rootAt(3)), l["proof-anchor"])
		require.EqualValues(t, 3, l["proof-anchor-block"])
		require.Equal(t, false, l["anchor-executed"])
	})
}
