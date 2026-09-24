// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"bytes"
	"crypto/ed25519"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
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

// The selection among the committee is unchanged: a validator asks only when
// the previous block's hash names it, so a partition's requests stay at two
// per activation (healing.md, "Who asks, and when"). The fix adds the node
// the selection cannot name; it does not make everyone ask.
func TestSelectionAmongTheCommitteeIsUnchanged(t *testing.T) {
	part := &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}
	seed := []byte("any agreed seed")

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
		if c.selectedToPull(seed) {
			selected++
		}
	}
	require.Equal(t, sendersPerActivation, selected,
		"a committee of eight must still send exactly two requests")
}

// The statement has to survive the handler a node actually runs, not only the
// recorder: followerlog.py parses the rendered line. This renders one through
// the daemon's own chain — the module-level slog handler writing through the
// console writer — and logs it, so the line the reader is written against is
// in this test's output.
func TestAnchorNotSentRenders(t *testing.T) {
	buf := new(bytes.Buffer)
	h, err := logging.NewSlogHandler(logging.SlogConfig{DefaultLevel: slog.LevelInfo}, logging.ConsoleSlogWriter(buf, false))
	require.NoError(t, err)
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	runOneBlockAs(t, &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}, false)

	var line string
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.Contains(l, "Anchor not sent") {
			line = l
			break
		}
	}
	require.NotEmpty(t, line, "the node must have logged the root it computed")
	t.Log(line)
	for _, want := range []string{"module=conductor", "source=BVN1", "block=", "seq=", "root=", "bpt="} {
		require.Containsf(t, line, want, "the rendered line carries no %s", want)
	}
}
