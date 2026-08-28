// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// captureLogger records Error lines so a test can assert on what an operator
// would actually see.
type captureLogger struct {
	mu   sync.Mutex
	errs []logLine
}

type logLine struct {
	msg string
	kv  map[string]interface{}
}

func (l *captureLogger) record(msg string, keyvals []interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	kv := map[string]interface{}{}
	for i := 0; i+1 < len(keyvals); i += 2 {
		k, _ := keyvals[i].(string)
		kv[k] = keyvals[i+1]
	}
	l.errs = append(l.errs, logLine{msg: msg, kv: kv})
}

func (l *captureLogger) Debug(msg string, keyvals ...interface{}) {}
func (l *captureLogger) Info(msg string, keyvals ...interface{})  {}
func (l *captureLogger) Error(msg string, keyvals ...interface{}) { l.record(msg, keyvals) }
func (l *captureLogger) With(keyvals ...interface{}) logging.Logger {
	return l
}

func (l *captureLogger) lines() []logLine {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]logLine, len(l.errs))
	copy(out, l.errs)
	return out
}

func newLivenessService(t *testing.T) (*Service, *captureLogger) {
	t.Helper()
	log := new(captureLogger)
	s := &Service{
		config: ServiceConfig{
			Partition: &protocol.PartitionInfo{ID: "Directory"},
		},
	}
	s.logger.L = log
	return s, log
}

// A partition that has never produced a block must still be reported. The
// previous check keyed off the last block timestamp and skipped while it was
// zero, so a Directory frozen at its startup height logged nothing at all.
func TestStallReportedWhenNoBlockEverProduced(t *testing.T) {
	s, log := newLivenessService(t)
	s.startedAt = time.Now().Add(-blockStallThreshold - time.Second)

	s.checkBlockLiveness()

	lines := log.lines()
	require.Len(t, lines, 1, "a partition that never produced a block must be reported")
	require.Equal(t, "Partition stalled: no block produced", lines[0].msg)
	require.Equal(t, "Directory", lines[0].kv["partition"])
	require.Equal(t, false, lines[0].kv["everProducedBlock"])
}

func TestNoStallReportedBeforeThreshold(t *testing.T) {
	s, log := newLivenessService(t)
	s.startedAt = time.Now().Add(-blockStallThreshold + 2*time.Second)
	s.lastBlockAt = time.Now().Add(-blockStallThreshold + 2*time.Second)

	s.checkBlockLiveness()

	require.Empty(t, log.lines(), "must stay quiet inside the threshold")
}

func TestStallReportedAfterThreshold(t *testing.T) {
	s, log := newLivenessService(t)
	s.startedAt = time.Now().Add(-time.Hour)
	s.lastBlockAt = time.Now().Add(-blockStallThreshold - time.Millisecond)
	s.lastBlockIndex = 121

	s.checkBlockLiveness()

	lines := log.lines()
	require.Len(t, lines, 1)
	require.Equal(t, "Partition stalled: no block produced", lines[0].msg)
	require.Equal(t, uint64(121), lines[0].kv["lastBlock"])
	require.NotContains(t, lines[0].kv, "everProducedBlock",
		"a partition that did produce blocks must not be labelled as never having produced one")
}

// The loop ticks every 100ms. Without rate limiting a real stall emits ten
// identical lines per second and buries the log it is meant to flag.
func TestStallReportIsRateLimited(t *testing.T) {
	s, log := newLivenessService(t)
	s.startedAt = time.Now().Add(-time.Hour)
	s.lastBlockAt = time.Now().Add(-blockStallThreshold - time.Millisecond)

	for i := 0; i < 25; i++ {
		s.checkBlockLiveness()
	}
	require.Len(t, log.lines(), 1, "a continuing stall must report once per repeat interval")

	// Age the last report past the repeat interval.
	s.lastStallLog = time.Now().Add(-blockStallRepeat - time.Millisecond)
	s.checkBlockLiveness()
	require.Len(t, log.lines(), 2, "a stall that persists must be re-reported")
}

func TestRecoveryIsReportedAndClearsStall(t *testing.T) {
	s, log := newLivenessService(t)
	s.startedAt = time.Now().Add(-time.Hour)
	s.lastBlockAt = time.Now().Add(-blockStallThreshold - time.Second)

	s.checkBlockLiveness()
	require.Len(t, log.lines(), 1)
	require.False(t, s.stallSince.IsZero(), "stall must be latched")

	s.noteBlockProduced(122, 15925)

	lines := log.lines()
	require.Len(t, lines, 2)
	require.Equal(t, "Partition resumed producing blocks", lines[1].msg)
	require.Equal(t, uint64(122), lines[1].kv["block"])
	require.True(t, s.stallSince.IsZero(), "stall must clear once a block lands")

	// A healthy partition then stays quiet.
	s.checkBlockLiveness()
	require.Len(t, log.lines(), 2)
}

// Before Start stamps startedAt there is nothing to measure against; the
// watchdog must not report a stall for a service that has not started.
func TestNoStallBeforeStart(t *testing.T) {
	s, log := newLivenessService(t)
	s.checkBlockLiveness()
	require.Empty(t, log.lines())
}

// The watchdog must keep reporting while block production is wedged. Producing
// a block blocks until every batch named by the certificate is collected, so a
// partition stuck on a missing batch stops draining its loop — if the watchdog
// shared that loop it would fall silent in the one case it exists to catch.
func TestWatchdogFiresWhileBlockProductionIsWedged(t *testing.T) {
	s, log := newLivenessService(t)
	s.startedAt = time.Now().Add(-blockStallThreshold - time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.ctx = ctx

	// Stand in for the wedged block production loop: a goroutine that is
	// blocked forever and will never service a ticker.
	wedged := make(chan struct{})
	go func() { <-wedged }()
	defer close(wedged)

	s.wg.Add(1)
	go s.livenessLoop()

	require.Eventually(t, func() bool {
		for _, l := range log.lines() {
			if l.msg == "Partition stalled: no block produced" {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond,
		"the watchdog must report a stall without help from the block production loop")

	cancel()
	s.wg.Wait()
}
