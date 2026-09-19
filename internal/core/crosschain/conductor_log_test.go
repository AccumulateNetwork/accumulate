// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// recorder keeps every record written to the default logger, so a test can
// read the lines a node would have written to its log stream.
type recorder struct {
	attrs   []slog.Attr
	records *[]slog.Record
}

func (r *recorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *recorder) Handle(_ context.Context, rec slog.Record) error {
	// Fold in attributes carried by WithAttrs so the recorded record holds
	// everything a handler would have printed.
	c := rec.Clone()
	for _, a := range r.attrs {
		c.AddAttrs(a)
	}
	*r.records = append(*r.records, c)
	return nil
}

func (r *recorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &recorder{attrs: append(append([]slog.Attr{}, r.attrs...), attrs...), records: r.records}
}

func (r *recorder) WithGroup(string) slog.Handler { return r }

// attr reads one attribute off a record, reporting whether it was there.
func attr(rec slog.Record, key string) (slog.Value, bool) {
	var v slog.Value
	var ok bool
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			v, ok = a.Value, true
			return false
		}
		return true
	})
	return v, ok
}

// captureLog makes the default logger record every line for the duration of
// the test, and returns the slice it records into.
func captureLog(t *testing.T) *[]slog.Record {
	t.Helper()
	records := new([]slog.Record)
	prev := slog.Default()
	slog.SetDefault(slog.New(&recorder{records: records}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return records
}

// nullDispatcher accepts and drops, so the conductor's send path runs to the
// end without a network.
type nullDispatcher struct{}

func (nullDispatcher) Submit(context.Context, *url.URL, *messaging.Envelope) error { return nil }
func (nullDispatcher) Close()                                                      {}

func (nullDispatcher) Send(context.Context) <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}

// anchorLogDB seeds the state one block of a partition leaves behind: a system
// ledger holding the anchor for that block, an anchor ledger with a sequence
// number, and a root chain with something in it.
func anchorLogDB(t *testing.T, part *protocol.PartitionInfo, block uint64) *database.Database {
	t.Helper()
	partUrl := protocol.PartitionUrl(part.ID)
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	var anchor protocol.AnchorBody
	if part.Type == protocol.PartitionTypeDirectory {
		anchor = new(protocol.DirectoryAnchor)
	} else {
		anchor = new(protocol.BlockValidatorAnchor)
	}
	anchor.GetPartitionAnchor().Source = partUrl
	anchor.GetPartitionAnchor().MinorBlockIndex = block

	ledger := new(protocol.SystemLedger)
	ledger.Url = partUrl.JoinPath(protocol.Ledger)
	ledger.Index = block
	ledger.Anchor = anchor
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))

	anchors := new(protocol.AnchorLedger)
	anchors.Url = partUrl.JoinPath(protocol.AnchorPool)
	anchors.MinorBlockSequenceNumber = 12
	require.NoError(t, batch.Account(anchors.Url).Main().Put(anchors))

	chain, err := batch.Account(ledger.Url).RootChain().Get()
	require.NoError(t, err)
	require.NoError(t, chain.AddEntry(make([]byte, 32), false))

	require.NoError(t, batch.Commit())
	return db
}

// anchorLogGlobals is a two-partition network, so a Directory conductor
// anchors to more than itself.
func anchorLogGlobals() *network.GlobalValues {
	g := new(network.GlobalValues)
	g.ExecutorVersion = protocol.ExecutorVersionLatest
	g.Network = &protocol.NetworkDefinition{
		NetworkName: "anchor-log",
		Version:     1,
		Partitions: []*protocol.PartitionInfo{
			{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory},
			{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
		},
	}
	return g
}

// runOneBlock drives a conductor for one partition through the production
// entry point: the event bus it subscribes to in Start, carrying the block
// event an executor publishes. Nothing in the test tells the conductor which
// partition it is beyond the Partition field a node sets.
func runOneBlock(t *testing.T, part *protocol.PartitionInfo) {
	t.Helper()
	const block = 500
	c := &Conductor{
		Partition:    part,
		ValidatorKey: acctesting.GenerateKey(t.Name(), part.ID),
		Database:     anchorLogDB(t, part, block),
		Dispatcher:   nullDispatcher{},
		RunTask:      func(f func()) { f() },
	}

	bus := events.NewBus(nil)
	require.NoError(t, c.Start(bus))
	require.NoError(t, bus.Publish(events.WillChangeGlobals{New: anchorLogGlobals()}))
	require.NoError(t, bus.Publish(execute.WillBeginBlock{BlockParams: execute.BlockParams{
		Context: context.Background(),
		Index:   block + 1,
		Time:    time.Now(),
	}}))
}

// anchorLines picks the sends out of what was logged.
func anchorLines(records *[]slog.Record) []slog.Record {
	var lines []slog.Record
	for _, rec := range *records {
		if rec.Message == "Sending an anchor" {
			lines = append(lines, rec)
		}
	}
	return lines
}

// Every container runs two nodes — a DN node and a BVN node — in one process,
// sharing one log stream, and both anchor to acc://dn.acme. Without the source
// partition on the line, the two nodes' sends for the same block cannot be
// told apart (#4370).
func TestSendingAnAnchorNamesItsSource(t *testing.T) {
	for _, part := range []*protocol.PartitionInfo{
		{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory},
		{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
	} {
		t.Run(part.ID, func(t *testing.T) {
			records := captureLog(t)
			runOneBlock(t, part)

			lines := anchorLines(records)
			require.NotEmpty(t, lines, "the conductor must have sent an anchor")
			for _, line := range lines {
				dest, ok := attr(line, "destination")
				require.True(t, ok, "the line names its destination")

				src, ok := attr(line, "source")
				require.Truef(t, ok, "the line to %v names no source partition", dest)
				require.Equalf(t, part.ID, src.String(),
					"the line to %v names %v as its source, not this conductor's partition", dest, src)
			}
		})
	}
}

// The DN node and the BVN node of one container both anchor to acc://dn.acme
// for the same block. Grouping the two nodes' lines by (source, destination,
// block) must separate them; grouping by (destination, block) alone cannot.
func TestTwoNodesOneLogStreamAreToldApart(t *testing.T) {
	records := captureLog(t)
	runOneBlock(t, &protocol.PartitionInfo{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory})
	runOneBlock(t, &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator})

	type key struct{ source, destination, block string }
	toDn := map[key]int{}
	for _, line := range anchorLines(records) {
		dest, ok := attr(line, "destination")
		require.True(t, ok)
		if dest.String() != protocol.DnUrl().String() {
			continue
		}
		src, ok := attr(line, "source")
		require.Truef(t, ok, "a line to %v names no source partition", dest)
		blk, ok := attr(line, "block")
		require.True(t, ok)
		toDn[key{src.String(), dest.String(), blk.String()}]++
	}

	require.Len(t, toDn, 2, "the two nodes' anchors to the Directory must be two groups, not one")
	for k, n := range toDn {
		require.Equalf(t, 1, n, "group %+v holds more than one send", k)
	}
}

// The attribute has to survive the handler a node actually runs, not only the
// recorder: #4365's reader parses the rendered line. This renders one send
// through the daemon's own chain — the module-level slog handler writing
// through the console writer — and logs it, so the line a parser is written
// against is in this test's output.
func TestSendingAnAnchorRendersSource(t *testing.T) {
	buf := new(bytes.Buffer)
	h, err := logging.NewSlogHandler(logging.SlogConfig{DefaultLevel: slog.LevelInfo}, logging.ConsoleSlogWriter(buf, false))
	require.NoError(t, err)
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	runOneBlock(t, &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator})

	var line string
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.Contains(l, "Sending an anchor") {
			line = l
			break
		}
	}
	require.NotEmpty(t, line, "the conductor must have logged its send")
	t.Log(line)
	require.Contains(t, line, "source=BVN1")
}
