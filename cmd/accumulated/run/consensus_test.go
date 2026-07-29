// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"strings"
	"testing"

	"github.com/multiformats/go-multiaddr"
)

// TestConsensusExternalAddress verifies that the CometBFT external (advertised)
// address is derived from the operator-configured [p2p] external multiaddr,
// using this partition's consensus port. Without this the address is empty,
// CometBFT mis-advertises, and PEX propagates the wrong address network-wide.
func TestConsensusExternalAddress(t *testing.T) {
	c := &ConsensusService{Listen: multiaddr.StringCast("/ip4/0.0.0.0/tcp/16591")}

	// The consensus port is whatever ListenAddress resolves to; the external
	// address must reuse that exact port but with the configured external host.
	listenPort := strings.TrimPrefix(listenUrl(c.Listen, defaultHost, useTCP{}, portCmtP2P), "tcp://0.0.0.0:")

	t.Run("ip4 external", func(t *testing.T) {
		inst := &Instance{config: &Config{P2P: &P2P{
			External: multiaddr.StringCast("/ip4/203.0.113.7/tcp/16593"),
		}}}
		got := c.cometExternalAddress(inst)
		want := "203.0.113.7:" + listenPort
		if got != want {
			t.Errorf("cometExternalAddress = %q, want %q", got, want)
		}
	})

	t.Run("dns external", func(t *testing.T) {
		inst := &Instance{config: &Config{P2P: &P2P{
			External: multiaddr.StringCast("/dns/node.example.com/tcp/16593"),
		}}}
		got := c.cometExternalAddress(inst)
		want := "node.example.com:" + listenPort
		if got != want {
			t.Errorf("cometExternalAddress = %q, want %q", got, want)
		}
	})

	t.Run("no external configured returns empty", func(t *testing.T) {
		if got := c.cometExternalAddress(&Instance{config: &Config{P2P: &P2P{}}}); got != "" {
			t.Errorf("cometExternalAddress = %q, want empty", got)
		}
		if got := c.cometExternalAddress(&Instance{config: &Config{}}); got != "" {
			t.Errorf("cometExternalAddress (nil P2P) = %q, want empty", got)
		}
	})
}
