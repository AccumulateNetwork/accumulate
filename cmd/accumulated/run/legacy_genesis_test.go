// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A genesis snapshot written before #3910 carries a consensus section: a
// CometBFT GenesisDoc. Genesis no longer writes it and nothing reads it, but
// every network deployed before then was started from such a file, and a node
// added to one of them loads it. The fixture is the Directory genesis
// `accumulated init genesis test/docker/docker-network.yml` wrote at
// fc0ccd135, loaded here through the node's own genesis path.
func TestAPreConsensusRemovalGenesisStillLoads(t *testing.T) {
	const file = "testdata/pre-3910-directory-genesis.snap"

	f, err := os.Open(file)
	require.NoError(t, err)
	rd, err := sv2.Open(f)
	require.NoError(t, err)
	var hasConsensus bool
	for _, s := range rd.Sections {
		hasConsensus = hasConsensus || s.Type() == sv2.SectionTypeConsensus
	}
	require.NoError(t, f.Close())
	require.True(t, hasConsensus, "the fixture must carry the legacy consensus section")

	db := database.OpenInMemory(nil)
	s := &DAGBFTService{Partition: &protocol.PartitionInfo{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory}}
	loaded, err := s.loadGenesisIfNeeded(db, file, logging.NewSlogLogger(nil))
	require.NoError(t, err)
	require.True(t, loaded)

	// Genesis state is there: the system ledger at the genesis block.
	var ledger *protocol.SystemLedger
	require.NoError(t, db.View(func(batch *database.Batch) error {
		return batch.Account(protocol.DnUrl().JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	}))
	require.Equal(t, uint64(protocol.GenesisBlock), ledger.Index)
}
