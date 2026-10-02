// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// A configuration with no p2p.key must not start under a new, random peer ID
// each time (#4490). A follower is given a key, written into its file, so
// every start after the first has the same identity; a validator, whose peer
// ID its committee and its peers' bootstrap lists name, refuses to start.
func TestMissingP2PKey(t *testing.T) {
	write := func(t *testing.T, c Configuration) (string, []byte) {
		file := filepath.Join(t.TempDir(), "accumulate.toml")
		cfg := &Config{Network: "KeyTest", Configurations: []Configuration{c}}
		require.NoError(t, cfg.SaveTo(file))
		b, err := os.ReadFile(file)
		require.NoError(t, err)
		require.NotContains(t, string(b), "[p2p.key]")
		return file, b
	}
	start := func(t *testing.T, file string) (*Instance, *Config, error) {
		cfg := new(Config)
		require.NoError(t, cfg.LoadFrom(file))
		inst, err := New(context.Background(), cfg)
		return inst, cfg, err
	}

	t.Run("Follower", func(t *testing.T) {
		file, orig := write(t, &FollowerConfiguration{Mode: CoreValidatorModeDN, Listen: multiaddr.StringCast("/tcp/16591")})

		first, cfg, err := start(t, file)
		require.NoError(t, err)
		key, ok := cfg.P2P.Key.(*RawPrivateKey)
		require.True(t, ok, "the follower's key is %T", cfg.P2P.Key)

		// The key is in the file, and what the operator wrote is kept.
		b, err := os.ReadFile(file)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(string(b), string(orig)), "the configuration was rewritten, not added to")
		require.Contains(t, string(b), key.Address)

		second, cfg, err := start(t, file)
		require.NoError(t, err)
		require.Equal(t, key.Address, cfg.P2P.Key.(*RawPrivateKey).Address)
		require.Equal(t, first.id, second.id, "the second start has a different identity")
	})

	t.Run("Validator", func(t *testing.T) {
		file, orig := write(t, &CoreValidatorConfiguration{Mode: CoreValidatorModeDN, Listen: multiaddr.StringCast("/tcp/16591")})

		_, _, err := start(t, file)
		require.Error(t, err)
		require.Contains(t, err.Error(), "p2p.key")

		b, err := os.ReadFile(file)
		require.NoError(t, err)
		require.Equal(t, orig, b, "a refused start changed the configuration")
	})
}
