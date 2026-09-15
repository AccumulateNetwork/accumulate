// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/fastsync"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// TestGenesisAnchorFromSnapshot covers the walk's only out-of-band trust input.
// A verifier is handed a snapshot, extracts the trust anchor from it, and
// checks that anchor against a hash pinned somewhere the network does not
// control. This proves the extraction and the hash — not the pinning, which is
// an operational act no code can perform.
func TestGenesisAnchorFromSnapshot(t *testing.T) {
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 3)
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	// What the walk would start from, read directly from the node
	direct := loadDirectoryGlobals(t, sim)
	wantHash, err := fastsync.GenesisAnchorHash(direct)
	require.NoError(t, err)

	// Collect a snapshot, as a node serving one would
	buf := new(ioutil.Buffer)
	require.NoError(t, sim.Database(Directory).View(func(batch *database.Batch) error {
		_, err := batch.Collect(buf, DnUrl(), &database.CollectOptions{})
		return err
	}))

	// A verifier extracts the anchor from the snapshot it was handed
	fromSnap, err := fastsync.LoadGenesisGlobals(ioutil.NewBuffer(buf.Bytes()), config.NetworkUrl{URL: DnUrl()})
	require.NoError(t, err)
	require.NotNil(t, fromSnap.Network)
	require.NotNil(t, fromSnap.Globals)

	// and checks it against the pinned hash
	gotHash, err := fastsync.GenesisAnchorHash(fromSnap)
	require.NoError(t, err)
	require.Equal(t, wantHash, gotHash,
		"the anchor extracted from the snapshot must hash to what a publisher pinned")

	// The hash is reproducible, which is what makes pinning it meaningful
	again, err := fastsync.GenesisAnchorHash(fromSnap)
	require.NoError(t, err)
	require.Equal(t, gotHash, again)

	// A tampered validator set must not pass the pinned hash
	tampered := fromSnap.Copy()
	require.NotEmpty(t, tampered.Network.Validators)
	tampered.Network.Validators[0].PublicKey[0]++
	badHash, err := fastsync.GenesisAnchorHash(tampered)
	require.NoError(t, err)
	require.NotEqual(t, wantHash, badHash,
		"a forged validator set must not hash to the pinned anchor")

	// And the anchor is usable: the walk starts from it
	spine, err := fastsync.NewSpine(fromSnap, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), spine.NextMajor)
}
