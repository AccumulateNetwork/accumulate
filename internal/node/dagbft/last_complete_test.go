// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/lastcomplete"
)

func fields(kv []interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

// The halt message carries the partition's last complete block (#4512).
func TestTheHaltNamesTheLastCompleteBlock(t *testing.T) {
	s, _ := newLivenessService(t)
	require.Empty(t, s.lastComplete(), "no source configured: nothing is said")

	s.config.LastComplete = func(context.Context) lastcomplete.Answer {
		return lastcomplete.Answer{Known: true, Block: 33, Root: [32]byte{0xab}, Executed: 34, Ahead: 1, State: lastcomplete.NotObtainable}
	}
	f := fields(s.lastComplete())
	require.EqualValues(t, 33, f["lastCompleteBlock"])
	require.Contains(t, f["lastCompleteRoot"], "ab00")
	require.EqualValues(t, 34, f["executed"])
	require.EqualValues(t, 1, f["executedPastIt"])
	require.Contains(t, f["stateAsOfIt"], "not obtainable")
}

// A history that could not be read is said with the error, not as a bare state.
func TestTheHaltSaysWhyTheHistoryCouldNotBeRead(t *testing.T) {
	s, _ := newLivenessService(t)
	s.config.LastComplete = func(context.Context) lastcomplete.Answer {
		return lastcomplete.Answer{Known: true, Block: 33, State: lastcomplete.Unreadable, StateErr: "disk gone"}
	}
	f := fields(s.lastComplete())
	require.Contains(t, f["stateAsOfIt"], "unreadable")
	require.Equal(t, "disk gone", f["stateAsOfItWhy"])
}

// A halt that cannot read the Directory says N is unknown and why; it does
// not offer a block from its own state.
func TestTheHaltSaysWhenTheLastCompleteBlockIsUnknown(t *testing.T) {
	s, _ := newLivenessService(t)
	s.config.LastComplete = func(context.Context) lastcomplete.Answer {
		return lastcomplete.Answer{Executed: 34, Why: "the Directory could not be read: no peer"}
	}
	f := fields(s.lastComplete())
	require.Equal(t, "unknown", f["lastCompleteBlock"])
	require.Contains(t, f["lastCompleteWhy"], "no peer")
	require.EqualValues(t, 34, f["executed"])
}
