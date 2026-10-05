// Copyright 2026 The Accumulate Authors
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
		// Rotated 2026-08-17. The previous identities had their private
		// keys disclosed and are retired; nothing answers on them
		// (12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg, and
		// 12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx, which this
		// line carried until 2026-10-05).
		//
		// Every bootstrap server, each by its own name: two hosts run the
		// same identity, and a node that cannot reach one reaches the
		// other. As of 2026-10-05, from each server's :8080/info:
		//   bootstrap.accumulatenetwork.io   -> 3.138.61.111
		//   bootstrap.accumulate.defidevs.io -> 77.42.40.245
		// libp2p authenticates the peer ID, not the hostname, so moving a
		// host is a DNS change and rotating the key is a release.
		"/dns/bootstrap.accumulatenetwork.io/tcp/16593/p2p/12D3KooWQaWn1L63nJUxfidDomh6W6o1jXJ1VHykzEEdKASSbURr",
		"/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWQaWn1L63nJUxfidDomh6W6o1jXJ1VHykzEEdKASSbURr",
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
