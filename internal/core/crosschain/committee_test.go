// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// linesNamed picks one message out of what was logged.
func linesNamed(records *[]slog.Record, msg string) []slog.Record {
	var lines []slog.Record
	for _, rec := range *records {
		if rec.Message == msg {
			lines = append(lines, rec)
		}
	}
	return lines
}

// A node whose key is inactive on a partition is in no committee there, and a
// node in no committee neither signs nor dispatches an anchor for that
// partition (executor.md, "Sync" step 5: a follower "does not vote or
// propose"). Measured on run 20260919T191634Z: acc-bvn3-fol1 dispatched 1,997
// anchors and every one was refused at msg_block_anchor.go:285 (#4367).
//
// Both partition types, because the container runs two nodes and the run
// showed both of them doing it: its Directory node sent to all four
// partitions and its BVN3 node to the Directory.
func TestANodeOutsideTheCommitteeDispatchesNoAnchor(t *testing.T) {
	for _, part := range []*protocol.PartitionInfo{
		{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory},
		{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
	} {
		t.Run(part.ID, func(t *testing.T) {
			// The same conductor, the same block, the same database: the
			// only difference is whether the key is active in the
			// NetworkDefinition.
			records := captureLog(t)
			active := runOneBlockAs(t, part, true)
			require.NotEmpty(t, anchorLines(records), "a validator must still send its anchor")
			require.NotEmpty(t, active.submissions(), "a validator must still dispatch its anchor")

			records = captureLog(t)
			follower := runOneBlockAs(t, part, false)
			require.Empty(t, anchorLines(records),
				"a node in no committee logged a send")
			require.Empty(t, follower.submissions(),
				"a node in no committee dispatched an anchor")
		})
	}
}

// The gate removes the line gate 0's root comparison reads (followerlog.py:84
// keys `Sending an anchor` by (source, destination, block)), so the node that
// no longer sends still states the root it computed — once per block, with
// the same fields, and with the same value the validator's send carries.
//
// Without this the fix would blind the instrument that measures it: a
// follower's roots were only visible on run 20260919T191634Z because it was
// misbehaving.
func TestANodeOutsideTheCommitteeStatesTheRootItComputed(t *testing.T) {
	part := &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}

	records := captureLog(t)
	runOneBlockAs(t, part, true)
	sent := anchorLines(records)
	require.Len(t, sent, 1, "a BVN validator anchors to the Directory, once")

	records = captureLog(t)
	runOneBlockAs(t, part, false)
	stated := linesNamed(records, "Anchor not sent")
	require.Len(t, stated, 1, "the node states the root it computed, once per block")

	for _, key := range []string{"source", "block", "seq", "root", "bpt"} {
		want, ok := attr(sent[0], key)
		require.Truef(t, ok, "the validator's line carries no %s", key)
		got, ok := attr(stated[0], key)
		require.Truef(t, ok, "the statement carries no %s", key)
		require.Equalf(t, want.String(), got.String(),
			"the statement's %s differs from the send's", key)
	}
	mod, ok := attr(stated[0], "module")
	require.True(t, ok, "the statement carries no module")
	require.Equal(t, "conductor", mod.String())
}

// recordingRanger records every span it is asked for and answers nothing, so
// a test can see that a request was made without standing up a source.
type recordingRanger struct{ asked int }

func (r *recordingRanger) Sequence(context.Context, *url.URL, *url.URL, uint64, private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	r.asked++
	return nil, errors.NotFound
}

func (r *recordingRanger) SequenceRange(context.Context, *url.URL, *url.URL, uint64, uint64, private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	r.asked++
	return nil, errors.NotFound
}

// gapDB seeds a BVN with a hole in its anchor stream: the ledgers requestGaps
// reads, with the Directory's anchors delivered through 1.
func gapDB(t *testing.T, part *protocol.PartitionInfo, block uint64) *database.Database {
	t.Helper()
	db := anchorLogDB(t, part, block)
	partUrl := protocol.PartitionUrl(part.ID)
	batch := db.Begin(true)
	defer batch.Discard()

	var anchors *protocol.AnchorLedger
	require.NoError(t, batch.Account(partUrl.JoinPath(protocol.AnchorPool)).Main().GetAs(&anchors))
	anchors.Partition(protocol.DnUrl()).Delivered = 1
	require.NoError(t, batch.Account(anchors.Url).Main().Put(anchors))

	synth := new(protocol.SyntheticLedger)
	synth.Url = partUrl.JoinPath(protocol.Synthetic)
	require.NoError(t, batch.Account(synth.Url).Main().Put(synth))

	require.NoError(t, batch.Commit())
	return db
}

// A node not in the committee still requests its own gaps. The healing pull
// is membership-gated (requester.go, selectedToPull → cadence.go,
// partitionValidators) and the follower on run 20260919T191634Z therefore
// requested nothing at all while its four peers requested 351 times: a
// synthetic or anchor missing on it is a hole nothing closes, and
// `followerHeals` reading 0 is indistinguishable from calm.
//
// Driven through the production entry point — Start(bus), WillChangeGlobals,
// WillBeginBlock — not by calling requestGaps.
func TestANodeOutsideTheCommitteeAsksForItsOwnGaps(t *testing.T) {
	part := &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}
	// The block about to begin must be an activation, because the cadence is
	// the block index (cadence.go, healActivates); the ledger is the block
	// before it.
	const block = 499
	require.True(t, healActivates(block+1), "the block under test must be an activation")

	asks := func(active bool) int {
		key := acctesting.GenerateKey(t.Name(), part.ID)
		ranger := new(recordingRanger)

		staging := execute.NewStaging()
		tx := staging.Begin()
		// Anchor 3 held with anchor 2 missing: a hole, which is asked for on
		// sight rather than after a stillness window (#4288).
		stream := execute.StreamID{Ledger: protocol.PartitionUrl(part.ID).JoinPath(protocol.AnchorPool), Source: protocol.DnUrl()}
		seq := &messaging.SequencedMessage{Number: 3, Source: protocol.DnUrl(), Destination: protocol.PartitionUrl(part.ID)}
		tx.Hold(stream, 3, &execute.Held{ID: seq.ID(), Message: seq, Hash: seq.Hash()})
		tx.Commit()

		c := &Conductor{
			Partition:    part,
			ValidatorKey: key,
			Database:     gapDB(t, part, block),
			Dispatcher:   new(countingDispatcher),
			Sequencer:    ranger,
			Staging:      staging,
			RunTask:      func(f func()) { f() },
		}

		bus := events.NewBus(nil)
		require.NoError(t, c.Start(bus))
		globals := anchorLogGlobals(ed25519.PrivateKey(key).Public().(ed25519.PublicKey), active)
		require.NoError(t, bus.Publish(events.WillChangeGlobals{New: globals}))
		require.NoError(t, bus.Publish(execute.WillBeginBlock{BlockParams: execute.BlockParams{
			Context: context.Background(),
			Index:   block + 1,
			Time:    time.Now(),
		}}))
		return ranger.asked
	}

	require.NotZero(t, asks(true), "a validator asks — otherwise this test measures nothing")
	require.NotZero(t, asks(false), "a node in no committee never asked for its own gap")
}

// The selection among the committee is unchanged: a validator asks only when
// the previous block's hash names it, so a partition's requests stay at two
// per activation (healing.md, "Who asks, and when"). The fix adds the node
// the selection cannot name; it does not make everyone ask.
func TestSelectionAmongTheCommitteeIsUnchanged(t *testing.T) {
	part := &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}
	ledger := &protocol.SystemLedger{Index: 500}

	// Eight validators, all active, and one of them is us in turn.
	var keys []ed25519.PrivateKey
	for i := 0; i < 8; i++ {
		keys = append(keys, acctesting.GenerateKey(t.Name(), i))
	}
	globals := new(network.GlobalValues)
	globals.ExecutorVersion = protocol.ExecutorVersionLatest
	globals.Network = &protocol.NetworkDefinition{NetworkName: "selection", Version: 1}
	globals.Network.AddPartition(part.ID, part.Type)
	for _, k := range keys {
		globals.Network.AddValidator(k.Public().(ed25519.PublicKey), part.ID, true)
	}

	selected := 0
	for _, k := range keys {
		c := &Conductor{Partition: part, ValidatorKey: k}
		c.Globals.Store(globals)
		if c.selectedToPull(ledger) {
			selected++
		}
	}
	require.Equal(t, sendersPerActivation, selected,
		"a committee of eight must still send exactly two requests")
}

// And the node the selection cannot name is named by nothing else either: a
// node whose key is absent from the definition entirely — not merely
// inactive — is also outside the committee and also fills its own holes.
func TestANodeAbsentFromTheDefinitionAsksForItsOwnGaps(t *testing.T) {
	part := &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}
	key := acctesting.GenerateKey(t.Name(), "absent")
	globals := anchorLogGlobals(acctesting.GenerateKey(t.Name(), "other").Public().(ed25519.PublicKey), true)

	c := &Conductor{Partition: part, ValidatorKey: key}
	c.Globals.Store(globals)
	require.True(t, c.selectedToPull(&protocol.SystemLedger{Index: 500}))
}
