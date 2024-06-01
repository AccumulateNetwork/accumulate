// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestExecutorConsistency(t *testing.T) {
	// Load test data
	var testData struct {
		FinalHeight uint64                   `json:"finalHeight"`
		Network     *accumulated.NetworkInit `json:"network"`
		Genesis     map[string][]byte        `json:"genesis"`

		Steps []struct {
			Block     uint64                           `json:"block"`
			Envelopes map[string][]*messaging.Envelope `json:"envelopes"`
			Snapshots map[string][]byte                `json:"snapshots"`
		} `json:"steps"`
	}
	b, err := os.ReadFile("../testdata/executor-v1-consistency.json.xz")
	require.NoError(t, err)
	r, err := xz.NewReader(bytes.NewBuffer(b))
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(r).Decode(&testData))

	// Start the simulator
	sim, err := simulator.New(
		simulator.WithLogger(acctesting.NewTestLogger(t)),
		simulator.WithNetwork(testData.Network),
		simulator.SnapshotMap(testData.Genesis),
		simulator.DropDispatchedMessages(),
		simulator.InitialAcmeSupply(nil),
	)
	require.NoError(t, err)

	// Step the simulator in step with the gold file
	for _, step := range testData.Steps {
		require.Equal(t, sim.BlockIndex(protocol.Directory), step.Block-1)

		// Resubmit every user transaction
		for part, envs := range step.Envelopes {
			for _, env := range envs {
				_, err = sim.SubmitTo(part, env)
				require.NoError(t, err)
			}
		}

		// Step
		require.NoError(t, sim.Step())

		// Verify the root hashes of each partition
		for _, part := range sim.Partitions() {
			View(t, sim.Database(part.ID), func(batch *database.Batch) {
				compareSnapshot(t, part.ID, batch, step.Snapshots[part.ID])
			})
		}
		if t.Failed() {
			t.FailNow()
		}
	}

	// Run the simulator until the final height matches
	require.LessOrEqual(t, sim.BlockIndex(protocol.Directory), testData.FinalHeight)
	for sim.BlockIndex(protocol.Directory) < testData.FinalHeight {
		require.NoError(t, sim.Step())
	}

}

func compareSnapshot(t *testing.T, partition string, batch *database.Batch, buf []byte) {
	v := new(snapVisitor)
	v.TB = t
	v.partition = partition
	v.batch = batch
	err := snapshot.Visit(ioutil2.NewBuffer(buf), v)
	require.NoError(t, err)
}

type snapVisitor struct {
	testing.TB
	partition string
	batch     *database.Batch
	ok        bool
}

func (v *snapVisitor) VisitHeader(h *snapshot.Header) error {
	hash, err := v.batch.BPT().GetRootHash()
	if err != nil {
		return err
	}
	v.ok = assert.Equal(v, hash[:], h.RootHash[:], "%v block %v", v.partition, h.Height)
	return nil
}

func (v *snapVisitor) VisitAccount(a *snapshot.Account, i int) error {
	if a == nil || v.ok {
		return nil
	}

	b, err := snapshot.CollectAccount(v.batch.Account(a.Url), true)
	require.NoError(v, err)
	c, err := json.MarshalIndent(a, "", "  ")
	require.NoError(v, err)
	d, err := json.MarshalIndent(b, "", "  ")
	require.NoError(v, err)
	assert.Equalf(v, string(c), string(d), "Account %v", a.Url)

	return nil
}
