// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmtlog "github.com/cometbft/cometbft/libs/log"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"
)

// #4049: after the first read, the genesis snapshot is no longer required — the
// cached marker satisfies the Info() genesis check.
func TestGenesisMarker_BootsWithoutSnapshot(t *testing.T) {
	dir := t.TempDir()
	appHash := make([]byte, 32)
	for i := range appHash {
		appHash[i] = byte(i)
	}

	calls := 0
	app := &Accumulator{
		AccumulatorOptions: AccumulatorOptions{
			RootDir:   dir,
			Partition: "BVN0",
			Genesis: func() (*tmtypes.GenesisDoc, error) {
				calls++
				return &tmtypes.GenesisDoc{InitialHeight: 11, AppHash: tmbytes.HexBytes(appHash)}, nil
			},
		},
		logger: cmtlog.NewNopLogger(),
	}

	// First read: from the snapshot; writes the marker.
	h, ah, err := app.genesisCheck()
	require.NoError(t, err)
	require.Equal(t, int64(11), h)
	require.Equal(t, appHash, ah)
	require.Equal(t, 1, calls)
	require.FileExists(t, app.genesisMarkerPath())

	// Simulate the snapshot being gone: Genesis now errors.
	app.Genesis = func() (*tmtypes.GenesisDoc, error) {
		return nil, fmt.Errorf("snapshot deleted")
	}

	// Still works — served from the marker, Genesis is NOT called again.
	h2, ah2, err := app.genesisCheck()
	require.NoError(t, err)
	require.Equal(t, int64(11), h2)
	require.Equal(t, appHash, ah2)
	require.Equal(t, 1, calls, "snapshot must not be read again once the marker exists")
}

// #4049: the snapshot is deleted only after the marker is durably persisted.
func TestGenesisMarker_DeleteSnapshotGatedOnMarker(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "bvn0-genesis.snap")
	require.NoError(t, os.WriteFile(snap, []byte("genesis"), 0o644))

	app := &Accumulator{
		AccumulatorOptions: AccumulatorOptions{RootDir: dir, Partition: "BVN0", GenesisPath: snap},
		logger:             cmtlog.NewNopLogger(),
	}

	// No marker yet → must NOT delete (otherwise a crash before the marker is
	// written would leave the node unbootable).
	app.maybeDeleteGenesisSnapshot()
	require.FileExists(t, snap)

	// Marker present → safe to delete.
	require.NoError(t, os.WriteFile(app.genesisMarkerPath(), []byte(`{"initialHeight":11}`), 0o644))
	app.maybeDeleteGenesisSnapshot()
	require.NoFileExists(t, snap)
}
