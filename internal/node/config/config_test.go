// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "config"), 0777))

	// Create
	cfg := Default("unittest", protocol.PartitionTypeBlockValidator, Follower, t.Name())
	cfg.SetRoot(dir)
	cfg.Accumulate.API.ListenAddress = "api-listen"

	// Slice values are unmarshalled as empty. This avoids issues with empty
	// slice != nil.
	cfg.StateSync.RPCServers = []string{}
	cfg.Accumulate.P2P.Listen = []multiaddr.Multiaddr{}
	cfg.Accumulate.P2P.BootstrapPeers = []multiaddr.Multiaddr{}

	// Store
	require.NoError(t, Store(cfg))

	// Load
	lcfg, err := Load(dir)
	require.NoError(t, err)

	// Should be equal
	require.Equal(t, cfg, lcfg)
}
