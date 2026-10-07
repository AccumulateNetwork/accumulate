// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// lockedBuffer is a log sink the backfill's goroutine writes and the test
// reads.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestTheBackfillWaitsForEveryPartitionOfTheNode — #4550, run
// 20261007T141334Z: a node's Directory matched in two minutes and began
// backfilling history (ACME's main chain, 377k entries) while its BVN was
// still pulling state from the same validators: 850 chain queries a minute,
// three times the BVN pull's validator time. History is collected after the
// match, and the node is not past its match until every partition it runs
// is ACTIVE: the backfill runs behind the gate the daemon hands in.
func TestTheBackfillWaitsForEveryPartitionOfTheNode(t *testing.T) {
	here := protocol.DnUrl()
	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	var buf lockedBuffer
	var others atomic.Bool // the node's other partition: ACTIVE or not
	s := &PulledState{
		partition:    here,
		db:           db,
		sources:      &fixedSources{partition: here, srcs: []pull.Source{}},
		machine:      nodestate.New(here),
		log:          slog.New(slog.NewTextHandler(&buf, nil)),
		backfillWhen: others.Load,
	}
	require.True(t, s.machine.PromoteToActive([32]byte{1}, 10), "precondition: this partition is ACTIVE")
	s.markHeadOnly(here.JoinPath(protocol.AnchorPool))
	require.Equal(t, 1, s.BackfillLeft())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.Backfill(ctx) }()

	// The other partition is not ACTIVE: the backfill asks for nothing.
	time.Sleep(300 * time.Millisecond)
	require.NotContains(t, buf.String(), "entries could not be brought in",
		"the backfill ran while another partition of the node was still joining")
	require.Equal(t, 1, s.BackfillLeft())

	// Every partition ACTIVE: the backfill runs (no peer here serves the
	// account, so it says it could not bring it in yet).
	others.Store(true)
	deadline := time.Now().Add(2*backfillPause + time.Second)
	for !strings.Contains(buf.String(), "entries could not be brought in") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	require.Contains(t, buf.String(), "entries could not be brought in", "the backfill did not run once every partition was ACTIVE")
	cancel()
	<-done
}
