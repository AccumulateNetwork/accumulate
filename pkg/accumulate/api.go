// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulate

import (
	stdurl "net/url"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

var BootstrapServers = func() []multiaddr.Multiaddr {
	s := []string{
		// Rotated 2026-08-17. The previous identity
		// (12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg) had its
		// private key disclosed and is retired; nothing answers on it.
		//
		// Named under the network's own domain rather than the operator's
		// (was bootstrap.accumulate.defidevs.io). Peer discovery is the
		// network's entry point; it should not depend on a name belonging
		// to whoever happens to host it. Both names currently resolve to
		// the same address, so moving the host is now a DNS change rather
		// than a release — though rotating the KEY still is not, because
		// libp2p authenticates the peer ID below, not the hostname.
		"/dns/bootstrap.accumulatenetwork.io/tcp/16593/p2p/12D3KooWQaWn1L63nJUxfidDomh6W6o1jXJ1VHykzEEdKASSbURr",
	}
	addrs := make([]multiaddr.Multiaddr, len(s))
	for i, s := range s {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			panic(err)
		}
		addrs[i] = addr
	}
	return addrs
}()

const MainNetEndpoint = "https://mainnet.accumulatenetwork.io"
const KermitEndpoint = "https://kermit.accumulatenetwork.io"
const FozzieEndpoint = "https://fozzie.accumulatenetwork.io"

var WellKnownNetworks = map[string]string{
	"mainnet": MainNetEndpoint,
	"kermit":  KermitEndpoint,
	"fozzie":  FozzieEndpoint,

	"testnet": "https://kermit.accumulatenetwork.io",
	"local":   "http://127.0.1.1:26660",
}

func ResolveWellKnownEndpoint(name string, version string) string {
	addr, ok := WellKnownNetworks[strings.ToLower(name)]
	if !ok {
		addr = name
	}

	u, err := stdurl.Parse(addr)
	if err != nil {
		return addr
	}
	if u.Path == "" {
		addr += "/"
	}
	version = strings.TrimPrefix(version, "/")
	return addr + version
}
