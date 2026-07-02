// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tmcfg "github.com/cometbft/cometbft/config"
	tmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/libs/log"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"
)

// #4049: once the node is past genesis (marker present), stripGenesisAppState
// removes the AppState from the genesis doc in the state DB while preserving the
// rest; without the marker it does nothing.
func TestStripGenesisAppState(t *testing.T) {
	dir := t.TempDir()
	cfg := tmcfg.DefaultConfig()
	cfg.SetRoot(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "data"), 0o755))

	seed := &tmtypes.GenesisDoc{
		ChainID:       "accumulate-test",
		InitialHeight: 5,
		AppHash:       tmbytes.HexBytes{0x01, 0x02, 0x03},
		AppState:      json.RawMessage(`{"big":"pretend-2GB-snapshot"}`),
	}
	writeGenesisDoc(t, cfg, seed)

	marker := filepath.Join(dir, "marker")

	// No marker yet → AppState must be preserved (still needed for InitChain).
	stripGenesisAppState(cfg, log.NewNopLogger(), marker)
	require.NotEmpty(t, readGenesisDoc(t, cfg).AppState, "must not strip before the node is past genesis")

	// Marker present → AppState stripped, everything else preserved.
	require.NoError(t, os.WriteFile(marker, []byte("ok"), 0o644))
	stripGenesisAppState(cfg, log.NewNopLogger(), marker)

	got := readGenesisDoc(t, cfg)
	require.Empty(t, got.AppState, "AppState should be stripped")
	require.Equal(t, "accumulate-test", got.ChainID)
	require.Equal(t, int64(5), got.InitialHeight)
	require.Equal(t, tmbytes.HexBytes{0x01, 0x02, 0x03}, got.AppHash)

	// Idempotent: a second call is a no-op and does not error.
	stripGenesisAppState(cfg, log.NewNopLogger(), marker)
	require.Empty(t, readGenesisDoc(t, cfg).AppState)
}

func writeGenesisDoc(t *testing.T, cfg *tmcfg.Config, doc *tmtypes.GenesisDoc) {
	t.Helper()
	db, err := tmcfg.DefaultDBProvider(&tmcfg.DBContext{ID: "state", Config: cfg})
	require.NoError(t, err)
	defer db.Close()
	b, err := cmtjson.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, db.SetSync(cometGenesisDocKey, b))
}

func readGenesisDoc(t *testing.T, cfg *tmcfg.Config) *tmtypes.GenesisDoc {
	t.Helper()
	db, err := tmcfg.DefaultDBProvider(&tmcfg.DBContext{ID: "state", Config: cfg})
	require.NoError(t, err)
	defer db.Close()
	b, err := db.Get(cometGenesisDocKey)
	require.NoError(t, err)
	require.NotEmpty(t, b)
	var doc *tmtypes.GenesisDoc
	require.NoError(t, cmtjson.Unmarshal(b, &doc))
	return doc
}
