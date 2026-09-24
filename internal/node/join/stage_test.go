// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The gap is read against the Delivered the PULLED state holds, not the
// one staging remembers: B+1 carries #105 of a stream whose pulled Delivered
// is 103, and nothing is held at 104 — an entry from before the node was
// listening, which its peers hold. Once the pulled state says 104 was
// delivered, the stream runs contiguously and there is no gap (executor
// spec, "Sync", step 4).
func TestExecutorStage_AGapIsReadAgainstThePulledDelivered(t *testing.T) {
	source := protocol.PartitionUrl("BVN0")
	ledgerUrl := protocol.PartitionUrl("BVN1").JoinPath(protocol.Synthetic)
	stream := execute.StreamID{Ledger: ledgerUrl, Source: source}

	staging := execute.NewStaging()
	tx := staging.Begin()
	seq := &messaging.SequencedMessage{Number: 105, Source: source, Destination: protocol.PartitionUrl("BVN1")}
	tx.Hold(stream, 105, &execute.Held{ID: seq.ID(), Message: seq, Hash: seq.Hash()})
	tx.Commit()

	db := database.OpenInMemory(nil)
	deliver := func(n uint64) {
		t.Helper()
		batch := db.Begin(true)
		defer batch.Discard()
		ledger := new(protocol.SyntheticLedger)
		ledger.Url = ledgerUrl
		ledger.Partition(source).Delivered = n
		require.NoError(t, batch.Account(ledgerUrl).Main().Put(ledger))
		require.NoError(t, batch.Commit())
	}

	stage := &ExecutorStage{Staging: staging, Database: db}

	deliver(103)
	gaps, err := stage.HasGap(21)
	require.NoError(t, err)
	require.NotEmpty(t, gaps, "104 is missing between the pulled Delivered and what is held")

	deliver(104)
	gaps, err = stage.HasGap(22)
	require.NoError(t, err)
	require.Empty(t, gaps, "the pulled state says 104 executed, so 105 is next")
}

// Run 20260924T111811Z: "The next block has a gap; advancing the sync
// block=158 synced=157", on every BVN restart, and nothing in the line said
// which stream or which number (#4432). HasGap answers every stream with a
// gap -- its pulled Delivered, the run of numbers nothing is held for, the
// highest number held and the highest validated -- and the line prints it.
// A stream with no gap is not in the answer.
func TestExecutorStage_AGapNamesItsStreamAndNumbers(t *testing.T) {
	bvn1 := protocol.PartitionUrl("BVN1")
	synth := bvn1.JoinPath(protocol.Synthetic)
	gapped := execute.StreamID{Ledger: synth, Source: protocol.PartitionUrl("BVN0")}
	whole := execute.StreamID{Ledger: synth, Source: protocol.PartitionUrl("BVN2")}

	staging := execute.NewStaging()
	tx := staging.Begin()
	hold := func(id execute.StreamID, n uint64) {
		seq := &messaging.SequencedMessage{Number: n, Source: id.Source, Destination: bvn1}
		tx.Hold(id, n, &execute.Held{ID: seq.ID(), Message: seq, Hash: seq.Hash()})
	}
	hold(gapped, 107)
	hold(gapped, 109)
	hold(whole, 41)
	tx.Commit()

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = synth
	ledger.Partition(gapped.Source).Delivered = 103
	ledger.Partition(whole.Source).Delivered = 40
	require.NoError(t, batch.Account(synth).Main().Put(ledger))
	require.NoError(t, batch.Commit())

	stage := &ExecutorStage{Staging: staging, Database: db}
	gaps, err := stage.HasGap(158)
	require.NoError(t, err)
	require.Equal(t, []StreamGap{{
		Stream:    gapped,
		Delivered: 103,
		Missing:   104,
		MissingTo: 106,
		Held:      109,
	}}, gaps, "one stream has a gap: 104-106 missing above Delivered 103, 109 held; BVN2's 41 follows its 40")
	require.Equal(t, "bvn-BVN0.acme->bvn-BVN1.acme/synthetic delivered=103 missing=104-106 held=109 validated=0",
		gaps[0].String())
}
