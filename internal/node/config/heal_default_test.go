// Copyright 2026 The Accumulate Authors
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

// TestHealingEnableRoundTrip locks down the default-on behavior across a real
// TOML store/load. The critical case is a config with no explicit healing.enable
// (every currently-deployed mainnet node): it must load as nil so the run path
// treats it as on. An explicit false must survive as the break-glass off switch.
func TestHealingEnableRoundTrip(t *testing.T) {
	roundTrip := func(set *bool) *bool {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "config"), 0777))

		cfg := Default("unittest", protocol.PartitionTypeBlockValidator, Follower, t.Name())
		cfg.SetRoot(dir)
		// Match TestPersistence: empty slices round-trip as empty, not nil.
		cfg.StateSync.RPCServers = []string{}
		cfg.Accumulate.P2P.Listen = []multiaddr.Multiaddr{}
		cfg.Accumulate.P2P.BootstrapPeers = []multiaddr.Multiaddr{}

		cfg.Accumulate.Healing.Enable = set
		require.NoError(t, Store(cfg))

		lcfg, err := Load(dir)
		require.NoError(t, err)
		return lcfg.Accumulate.Healing.Enable
	}

	// Deployed-mainnet case: no explicit value → nil → run path defaults it on.
	require.Nil(t, roundTrip(nil), "absent healing.enable must load as nil (defaults on)")

	// Explicit off is preserved (break-glass kill-switch).
	off := false
	got := roundTrip(&off)
	require.NotNil(t, got, "explicit false must survive round-trip")
	require.False(t, *got)

	// Explicit on is preserved.
	on := true
	got = roundTrip(&on)
	require.NotNil(t, got, "explicit true must survive round-trip")
	require.True(t, *got)
}
