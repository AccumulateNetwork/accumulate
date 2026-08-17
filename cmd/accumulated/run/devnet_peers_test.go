// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"strconv"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// peerID is a real libp2p identity; cmtPeerAddress has to decode it to derive
// the CometBFT node ID, so a placeholder will not do.
const peerID = "12D3KooWQaWn1L63nJUxfidDomh6W6o1jXJ1VHykzEEdKASSbURr"

func portOf(t *testing.T, cmtAddr string) int {
	t.Helper()
	// tmp2p.IDAddressString gives "<id>@<host>:<port>"
	i := strings.LastIndex(cmtAddr, ":")
	require.Greater(t, i, 0, "no port in %q", cmtAddr)
	p, err := strconv.Atoi(cmtAddr[i+1:])
	require.NoError(t, err)
	return p
}

// cmtPeerAddress converts an Accumulate-P2P address (base+2) to a CometBFT
// P2P address (base+0). Handing it an address that is already at the CometBFT
// port produces base-2, which nothing listens on — that was #4081.
func TestCmtPeerAddress_ConvertsFromAccumulateP2PPort(t *testing.T) {
	const base = 26656 // devnet default: CometBFT P2P +0, libp2p +2

	got, err := cmtPeerAddress(multiaddr.StringCast(
		"/ip4/127.0.1.2/tcp/" + strconv.Itoa(base+2) + "/p2p/" + peerID))
	require.NoError(t, err)
	require.Equal(t, base, portOf(t, got),
		"an Accumulate-P2P address must convert to the CometBFT P2P port")
}

// The devnet builds the peer list that becomes ConsensusService.BootstrapPeers.
// This walks the same path those addresses take — construction, then
// conversion — and asserts they land where a node actually listens.
//
// Before the fix the devnet built these at portCmtP2P (base+0), so conversion
// produced base-2: every node dialled a port nothing bound, every connection
// was refused, and multi-node devnets sat at height 0 indefinitely. A
// single-node devnet was unaffected only because it has no peers to dial.
func TestDevnetPeerAddresses_LandOnTheCometBFTPort(t *testing.T) {
	const base = 26656
	listenAddr := multiaddr.StringCast("/tcp/" + strconv.Itoa(base))

	for _, c := range []struct {
		name      string
		partition portOffset
		wantPort  int
	}{
		{"directory", portDir, base},
		{"block validator", portBVN, base + 100},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Exactly how devnet.go builds a peer entry, for node IP .2
			id, err := peer.Decode(peerID)
			require.NoError(t, err)
			addr := addrForPeer(
				listen(listenAddr, devNetDefaultHost, ipOffset(1), useTCP{}, c.partition, portAccP2P),
				id)

			// The address handed to CometBFT
			cmt, err := cmtPeerAddress(addr)
			require.NoError(t, err)

			require.Equal(t, c.wantPort, portOf(t, cmt),
				"nodes must dial the port their peers listen on, not base-2")
			require.NotEqual(t, base-2, portOf(t, cmt), "regression: #4081")
		})
	}
}
