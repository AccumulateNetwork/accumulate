// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"testing"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/stretchr/testify/assert"
)

func TestDiscoveryConfig_ApplyDefaults(t *testing.T) {
	config := DiscoveryConfig{}
	config.ApplyDefaults()

	assert.Equal(t, dht.ModeAutoServer, config.DHTMode)
	assert.Equal(t, 30*time.Second, config.AdvertiseInterval)
	assert.Equal(t, 10*time.Second, config.DiscoveryInterval)
}

func TestDiscoveryConfig_ApplyDefaults_CustomValues(t *testing.T) {
	config := DiscoveryConfig{
		DHTMode:           dht.ModeClient,
		AdvertiseInterval: 60 * time.Second,
		DiscoveryInterval: 20 * time.Second,
	}
	config.ApplyDefaults()

	// Custom values should not be overwritten
	assert.Equal(t, dht.ModeClient, config.DHTMode)
	assert.Equal(t, 60*time.Second, config.AdvertiseInterval)
	assert.Equal(t, 20*time.Second, config.DiscoveryInterval)
}

func TestPartitionRendezvous(t *testing.T) {
	// Test rendezvous string generation
	testCases := []struct {
		partition string
		expected  string
	}{
		{"bvn0", "/acc/consensus/bvn0"},
		{"directory", "/acc/consensus/directory"},
		{"bvn-123", "/acc/consensus/bvn-123"},
	}

	for _, tc := range testCases {
		t.Run(tc.partition, func(t *testing.T) {
			result := PartitionRendezvous(tc.partition)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParseBootstrapPeers(t *testing.T) {
	// Valid bootstrap peer addresses
	validAddrs := []string{
		"/ip4/127.0.0.1/tcp/9000/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN",
	}
	peers, err := ParseBootstrapPeers(validAddrs)
	assert.NoError(t, err)
	assert.Len(t, peers, 1)
	assert.NotEmpty(t, peers[0].ID)

	// Invalid address (not a multiaddr)
	invalidAddrs := []string{"invalid-address"}
	_, err = ParseBootstrapPeers(invalidAddrs)
	assert.Error(t, err)

	// Valid multiaddr but no p2p component
	noP2PAddrs := []string{"/ip4/127.0.0.1/tcp/9000"}
	_, err = ParseBootstrapPeers(noP2PAddrs)
	assert.Error(t, err)

	// Empty list
	emptyAddrs := []string{}
	peers, err = ParseBootstrapPeers(emptyAddrs)
	assert.NoError(t, err)
	assert.Len(t, peers, 0)
}

func TestParseBootstrapPeers_MultiplePeers(t *testing.T) {
	addrs := []string{
		"/ip4/192.168.1.1/tcp/9000/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN",
		"/ip4/192.168.1.2/tcp/9001/p2p/12D3KooWQYV9dGMFoRzNStwpXztXaBzGPQwVTBPExLcpGCCBrC3o",
	}

	peers, err := ParseBootstrapPeers(addrs)
	assert.NoError(t, err)
	assert.Len(t, peers, 2)

	// Each peer should have distinct IDs
	assert.NotEqual(t, peers[0].ID, peers[1].ID)
}

func TestDiscoveryConfig_EmptyBootstrapPeers(t *testing.T) {
	config := DiscoveryConfig{}
	config.ApplyDefaults()

	assert.Nil(t, config.BootstrapPeers)
}
