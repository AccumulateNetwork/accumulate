// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"log/slog"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

type state[S any] interface {
	// execute processes the message and returns the next state and messages
	// that should be sent.
	execute(Message) (S, []Message, error)
}

func executeState[S state[S]](ctx context.Context, s S, msg Message) (S, []Message, error) {
	var allOut []Message
again:
	// Execute the message
	r, out, err := s.execute(msg)
	allOut = append(allOut, out...)
	if err != nil {
		return s, allOut, err
	}
	var z S
	if any(r) == any(z) {
		return z, allOut, nil
	}

	// Did we transition into the next state?
	if any(r) == any(s) {
		return s, allOut, nil
	}

	slog.DebugContext(ctx, "Transitioning", "to", logging.TypeOf(s))
	s = r

	// Keep stepping until the state doesn't change. Under normal circumstances
	// this won't do anything. However, this is necessary if there are 1 or 2
	// validators in the network. If there is only one validator, we must
	// proceed directly through the phases of consensus instead of waiting to
	// receive votes. Without this loop, each state would need logic to handle
	// that scenario.
	msg = nil
	goto again
}
