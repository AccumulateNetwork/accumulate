// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// recordedLog keeps every record a logger is given, by message.
type recordedLog struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordedLog) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordedLog) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *recordedLog) WithGroup(string) slog.Handler            { return h }
func (h *recordedLog) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

// said returns the attributes of every record with the message, as strings.
func (h *recordedLog) said(msg string) []map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []map[string]string
	for _, r := range h.records {
		if r.Message != msg {
			continue
		}
		m := map[string]string{}
		r.Attrs(func(a slog.Attr) bool {
			m[a.Key] = a.Value.String()
			return true
		})
		out = append(out, m)
	}
	return out
}

// blockingPull is a state whose first pull does not return until it is
// released: a call that a peer never answers.
type blockingPull struct {
	fakeState
	release chan struct{}
}

func (s *blockingPull) Pull(ctx context.Context) error {
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return s.fakeState.Pull(ctx)
}

// A join that stays on one step says what it is waiting on, and says when it
// stops (#4484). Two of three followers' Directory joins of Docker run
// 20260929T152146Z logged nothing for about six minutes before "Pulling the
// state": every step before the pull logged only when it failed.
func TestJoin_SaysWhatItIsWaitingOn(t *testing.T) {
	h := new(recordedLog)
	state := &blockingPull{fakeState: fakeState{matchAt: 5, matchFrom: 1}, release: make(chan struct{})}
	buf := &fakeBuffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, Options{
			Partition: "BVN0",
			Buffer:    buf,
			Stage:     &fakeStage{},
			State:     state,
			Peers:     &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}},
			Logger:    slog.New(h),
			Retry:     time.Millisecond,
			WaitSay:   20 * time.Millisecond,
		})
		done <- err
	}()

	require.Eventually(t, func() bool { return len(h.said("The join is waiting")) > 0 }, 2*time.Second, time.Millisecond,
		"a join held in its pull said nothing")
	w := h.said("The join is waiting")[0]
	require.Equal(t, "pulling the state", w["on"])
	require.NotEmpty(t, w["for"])

	close(state.release)
	require.NoError(t, <-done)
	ended := h.said("The join is no longer waiting")
	require.NotEmpty(t, ended, "the wait was said and its end was not")
	require.Equal(t, "pulling the state", ended[0]["on"])
}

// heldLedger is a peer that never answers for the partition's ledger.
type heldLedger struct {
	pull.Source
	name string
}

func (h heldLedger) QueryAccount(ctx context.Context, _ *url.URL, _ *api.DefaultQuery) (*api.AccountRecord, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (h heldLedger) String() string { return h.name }

// Inside a pull, the step names the peer it is asking: the one call to name
// when the join waits on a peer that does not answer (#4484).
func TestPull_SaysWhichPeerItIsWaitingOn(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	s := quietState(t, here, &oneSource{part: here, src: heldLedger{name: "peer held"}})
	tr, err := tracker.New(s.db, here)
	require.NoError(t, err)
	s.tracker = tr
	s.anchors = noAnchorsRead{}
	now := time.Unix(1000, 0)
	var mu sync.Mutex
	h := new(recordedLog)
	s.wait.log = slog.New(h)
	s.wait.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = s.Pull(ctx) }()

	require.Eventually(t, func() bool {
		s.wait.mu.Lock()
		defer s.wait.mu.Unlock()
		return s.wait.what == stepPeersBlock && len(s.wait.attrs) > 0
	}, 2*time.Second, time.Millisecond)
	mu.Lock()
	now = now.Add(6 * time.Minute)
	mu.Unlock()
	s.wait.say(time.Minute)

	said := h.said("The join is waiting")
	require.Len(t, said, 1)
	require.Equal(t, stepPeersBlock, said[0]["on"])
	require.Equal(t, "6m0s", said[0]["for"])
	require.Equal(t, "peer held", said[0]["peer"], "the line does not name the peer that does not answer")

	// Said once a minute, not once a tick.
	s.wait.say(time.Minute)
	require.Len(t, h.said("The join is waiting"), 1)

	cancel()
	<-done
}
