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
	"time"
)

// DefaultWaitSay is how long the join may stay on one step before it says
// what it is waiting on, and how often it says it again while it stays.
const DefaultWaitSay = time.Minute

// waiting is the step the join is on and since when, so that a join that
// stays on one step says so (#4484). Two of three followers' Directory joins
// of Docker run 20260929T152146Z logged nothing for about six minutes before
// "Pulling the state", and nothing in the log could say which call they were
// in. Every step before the pull logs only when it fails, so a call that is
// slow, or never answers, was silent.
//
// A step is named before each call that asks a peer. Naming the step it is
// already on updates what it is asking and counts the ask, without moving
// since: a step that asks three hundred slow questions is as visible as one
// that waits on one question for ever, and the count tells them apart.
type waiting struct {
	mu    sync.Mutex
	log   *slog.Logger
	now   func() time.Time
	what  string
	attrs []any
	asks  int
	since time.Time
	said  time.Time // when this step was last said; zero if it has not been
}

func (w *waiting) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

// on names the step the join is on and what it is asking now. An empty step
// is not waiting: the join is executing.
func (w *waiting) on(what string, attrs ...any) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if what == w.what && what != "" {
		w.attrs = attrs
		w.asks++
		return
	}
	now := w.clock()
	if !w.said.IsZero() && w.log != nil {
		w.log.Info("The join is no longer waiting", "on", w.what, "after", now.Sub(w.since).Round(time.Second), "asks", w.asks)
	}
	w.what, w.attrs, w.asks, w.since, w.said = what, attrs, 1, now, time.Time{}
}

// say logs the step if the join has been on it for at least every, and has
// not said it in the last every.
func (w *waiting) say(every time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.clock()
	if w.what == "" || w.log == nil || now.Sub(w.since) < every || !w.said.IsZero() && now.Sub(w.said) < every {
		return
	}
	w.said = now
	args := append([]any{"on", w.what, "for", now.Sub(w.since).Round(time.Second), "asks", w.asks}, w.attrs...)
	w.log.Info("The join is waiting", args...)
}

// watch says the step every so often until ctx ends.
func (w *waiting) watch(ctx context.Context, every time.Duration) {
	tick := every / 6
	if tick <= 0 {
		tick = time.Millisecond
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.say(every)
		}
	}
}

// waitingOf is the state's own step when it keeps one, so that the steps
// inside a pull and the steps of the join are one record.
func waitingOf(state State) *waiting {
	if s, ok := state.(interface{ joinWaiting() *waiting }); ok {
		return s.joinWaiting()
	}
	return new(waiting)
}
