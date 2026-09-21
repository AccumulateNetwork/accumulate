// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A CLOSED SEQUENCER LETS ITS DATABASE CLOSE.
//
// A commit that prepares an anchor leaves the sequencer holding a read view
// of that block, and the view is only replaced by the next such commit. Once
// blocks stop, the last view is held for good, and a LevelDB store waits for
// every open view before it closes: a node's Stop hung on it. The commit is
// delivered through the event bus, as block production delivers it.
func TestAClosedSequencerLetsItsDatabaseClose(t *testing.T) {
	db, err := database.OpenLevelDB(filepath.Join(t.TempDir(), "db"), nil)
	require.NoError(t, err)

	const block = 5
	ledgerUrl := protocol.PartitionUrl(servingPartition).JoinPath(protocol.Ledger)
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		anchor := new(protocol.BlockValidatorAnchor)
		anchor.MinorBlockIndex = block
		return batch.Account(ledgerUrl).Main().Put(&protocol.SystemLedger{
			Url:    ledgerUrl,
			Index:  block,
			Anchor: anchor,
		})
	}))

	bus := events.NewBus(nil)
	s := NewSequencer(SequencerParams{
		Partition: servingPartition,
		Cache:     synthcache.New(0),
		EventBus:  bus,
		Database:  db,
	})
	require.NoError(t, bus.Publish(events.DidCommitBlock{Index: block, Round: 1}))
	s.viewMu.Lock()
	captured := s.provable != nil
	s.viewMu.Unlock()
	require.True(t, captured, "the commit did not capture a provable view")

	s.Close()

	// A commit after Close captures nothing.
	require.NoError(t, bus.Publish(events.DidCommitBlock{Index: block, Round: 1}))

	closed := make(chan error, 1)
	go func() { closed <- db.Close() }()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the database did not close: the sequencer still holds a view")
	}
}
