// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// remoteServiceFallback returns a p2p service-discovery fallback that asks a set
// of registry nodes (their info-server /peers/{partition} endpoint) for the
// peers serving a partition. It is the delivery-discovery backstop for a node
// that does NOT run the embedded registry locally (#4047 §10): when the DHT is
// slow/stale under churn, cross-partition delivery still resolves live peers by
// querying a registry node. Private peers are already excluded server-side.
func remoteServiceFallback(bases []string) func(context.Context, multiaddr.Multiaddr) []peer.AddrInfo {
	return func(ctx context.Context, sa multiaddr.Multiaddr) []peer.AddrInfo {
		_, _, svc, _, err := api.UnpackAddress(sa)
		if err != nil || svc == nil || svc.Argument == "" {
			return nil
		}
		for _, base := range bases {
			if peers := httpServicePeers(ctx, base, svc.Argument); len(peers) > 0 {
				return peers
			}
		}
		return nil
	}
}

// peersResponse mirrors the bootstrap info server's /peers/{partition} JSON.
type peersResponse struct {
	Peers []struct {
		PeerID    string   `json:"peer_id"`
		Addresses []string `json:"addresses"`
	} `json:"peers"`
}

// httpServicePeers GETs {base}/peers/{partition} and returns dialable AddrInfos.
func httpServicePeers(ctx context.Context, base, partition string) []peer.AddrInfo {
	client := &http.Client{Timeout: 10 * time.Second}
	url := strings.TrimRight(base, "/") + "/peers/" + partition
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var body peersResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}

	out := make([]peer.AddrInfo, 0, len(body.Peers))
	for _, p := range body.Peers {
		id, err := peer.Decode(p.PeerID)
		if err != nil {
			continue
		}
		var addrs []multiaddr.Multiaddr
		for _, a := range p.Addresses {
			ma, err := multiaddr.NewMultiaddr(a)
			if err != nil {
				continue
			}
			// Addresses include /p2p/<id>; AddrInfoFromP2pAddr strips it.
			if info, err := peer.AddrInfoFromP2pAddr(ma); err == nil {
				addrs = append(addrs, info.Addrs...)
			}
		}
		out = append(out, peer.AddrInfo{ID: id, Addrs: addrs})
	}
	return out
}
