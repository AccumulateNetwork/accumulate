// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package bootstrap resolves a network's bootstrap peers from its well-known
// HTTP endpoint, so that peer identity is not compiled into binaries.
//
// # Why
//
// [accumulate.BootstrapServers] hardcodes a multiaddr, which couples key
// rotation to a software release. The mainnet bootstrap key was rotated on
// 2026-08-17 after its private key was disclosed; the rotation took about
// forty seconds, and making the network aware of it took a merge request, an
// approval, a release and an upgrade of every operator. Until that completed,
// every cold-starting client dialled an identity whose private key no longer
// existed. Nodes with live connections were unaffected, so the network looked
// healthy while cold-start discovery was broken.
//
// A key rotation is a security operation, sometimes an urgent one. It should
// not require shipping software.
//
// # Trust
//
// This moves the trust anchor from a peer ID that cannot be forged without
// the private key to the TLS certificate of the well-known endpoint. That is
// a real widening of the trust base and is worth stating plainly.
//
// It is acceptable because the client already trusts that exact endpoint
// unreservedly — it submits transactions through it and reads query results
// from it. An attacker holding that endpoint has already won, with or without
// this package. The blast radius is bounded: a hostile peer set can eclipse a
// client (stale views, censorship, refusal to relay) but cannot forge state,
// because transactions are signed and consensus validates independently of
// how a peer was discovered.
//
// Compare #4032, which argues the general form: discovered peer data is a
// cache, never a trust store, and must never override an authenticated
// source. Accordingly [Resolve] never overrides an explicit configuration —
// callers pass their configured peers as the fallback, and a caller that has
// been given peers deliberately should not call this at all.
package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
)

// DefaultTimeout bounds the whole resolution attempt. A node must start even
// when the endpoint is unreachable, so this is short and failure is silent:
// the caller gets its fallback and boots as it does today.
const DefaultTimeout = 5 * time.Second

// MaxPeers caps how many peers are returned. Bootstrapping needs a handful of
// reachable entry points, not the whole network, and an unbounded list is an
// amplification vector if the endpoint is ever hostile.
const MaxPeers = 16

// client is dedicated rather than http.DefaultClient. A bootstrap probe runs
// at startup against an endpoint that may be slow or unreachable, and it must
// not tie up connections in a pool shared with the rest of the process.
var client = &http.Client{Timeout: DefaultTimeout}

// probeService is the service asked about. Every Accumulate network has a
// Directory, and every node serving it answers queries — unlike the "node"
// service type, which mainnet returns zero peers for (see #4065).
var probeService = struct{ Type, Argument string }{"query", "Directory"}

// Resolve returns bootstrap peers for a network by asking its well-known
// endpoint, falling back to the supplied peers.
//
// It never returns an error and never returns an empty slice when fallback is
// non-empty: every failure path — unknown network, unreachable endpoint,
// malformed response, no usable peers — yields the fallback. Callers treat
// this as "better peers if available", not as an operation that can fail.
func Resolve(ctx context.Context, network string, fallback []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	endpoint, ok := endpointFor(network)
	if !ok {
		// Not a well-known network — a devnet, a simulator, or a private
		// deployment. There is nothing to ask, and no HTTP is attempted, so
		// this is free for every caller that is not on a public network.
		return fallback
	}

	peers := cached(ctx, network, endpoint)
	if len(peers) == 0 {
		return fallback
	}
	return peers
}

// CacheTTL is how long a resolution is reused.
//
// Several nodes are routinely created in one process — a devnet, the test
// suite, a dual-partition validator — and each was otherwise paying its own
// round trip. Measured on the cmd/accumulated/run tests, resolving per node
// cost 19 seconds of wall clock; caching returns that to the baseline.
//
// A FAILED resolution is cached too, and that is deliberate: without it, a
// process that cannot reach the endpoint pays the full timeout again for
// every node it starts, which is precisely the case where startup is already
// under stress.
const CacheTTL = time.Minute

var cache struct {
	sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	peers []multiaddr.Multiaddr
	at    time.Time
}

func cached(ctx context.Context, network, endpoint string) []multiaddr.Multiaddr {
	cache.Lock()
	defer cache.Unlock()

	if e, ok := cache.entries[network]; ok && time.Since(e.at) < CacheTTL {
		return e.peers
	}

	peers, err := query(ctx, endpoint, network)
	if err != nil {
		peers = nil
	}
	if cache.entries == nil {
		cache.entries = map[string]cacheEntry{}
	}
	cache.entries[network] = cacheEntry{peers: peers, at: time.Now()}
	return peers
}

// ResetCache discards cached resolutions. For tests.
func ResetCache() {
	cache.Lock()
	defer cache.Unlock()
	cache.entries = nil
}

// Augment returns the configured peers followed by any additional peers the
// network's well-known endpoint reports.
//
// This is what a node should call, and the ordering is the point. By the time
// configuration reaches the P2P layer there is no way to tell a peer an
// operator pinned deliberately from one a default filled in — both arrive as
// a populated slice. Replacing the list would therefore silently override an
// explicit choice, which is exactly what #4032 warns against.
//
// Augmenting sidesteps that: configured peers are kept and tried first, and
// discovered peers are added behind them. A stale compiled-in entry stops
// being fatal, because live peers accompany it rather than replace it, and an
// operator's pin is still honoured.
func Augment(ctx context.Context, network string, configured []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	discovered := Resolve(ctx, network, nil)
	if len(discovered) == 0 {
		return configured
	}

	out := make([]multiaddr.Multiaddr, 0, len(configured)+len(discovered))
	seen := make(map[string]bool, len(configured)+len(discovered))
	for _, list := range [][]multiaddr.Multiaddr{configured, discovered} {
		for _, a := range list {
			if s := a.String(); !seen[s] {
				seen[s] = true
				out = append(out, a)
			}
		}
	}
	return out
}

// endpointFor looks the network up WITHOUT the fallback-to-literal behaviour
// of [accumulate.ResolveWellKnownEndpoint], which returns the input unchanged
// for an unknown name. Here an unknown name must mean "do not attempt HTTP",
// not "treat this string as a URL".
func endpointFor(network string) (string, bool) {
	addr, ok := accumulate.WellKnownNetworks[strings.ToLower(network)]
	if !ok {
		return "", false
	}
	return strings.TrimSuffix(addr, "/") + "/v3", true
}

type findServiceResult struct {
	PeerID    string   `json:"peerID"`
	Addresses []string `json:"addresses"`
}

func query(ctx context.Context, endpoint, network string) ([]multiaddr.Multiaddr, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "find-service",
		"params": map[string]any{
			"network": network,
			"service": map[string]any{
				"type":     probeService.Type,
				"argument": probeService.Argument,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errStatus(resp.StatusCode)
	}

	var out struct {
		Result []findServiceResult `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, errRPC(out.Error.Message)
	}

	return collect(out.Result), nil
}

// collect turns the response into dialable multiaddrs.
//
// The API returns a peer ID and bare transport addresses as separate fields;
// a bootstrap peer needs them joined, because libp2p authenticates the peer
// ID and will not dial without it.
func collect(results []findServiceResult) []multiaddr.Multiaddr {
	var out []multiaddr.Multiaddr
	seen := map[string]bool{}

	for _, r := range results {
		if r.PeerID == "" {
			continue
		}
		p2p, err := multiaddr.NewComponent("p2p", r.PeerID)
		if err != nil {
			continue // not a peer ID we understand; skip rather than fail
		}

		for _, a := range r.Addresses {
			addr, err := multiaddr.NewMultiaddr(a)
			if err != nil {
				continue
			}

			// Filter here as well as server-side. #4091 fixed the server, but
			// a client cannot assume the node it is asking has been upgraded,
			// and a loopback address from a remote peer is guaranteed to be
			// undialable. Measured on mainnet before that fix, 4 of 10 peers
			// advertised only loopback.
			if !manet.IsPublicAddr(addr) {
				continue
			}

			full := addr.Encapsulate(p2p)
			if s := full.String(); !seen[s] {
				seen[s] = true
				out = append(out, full)
				if len(out) >= MaxPeers {
					return out
				}
			}
		}
	}
	return out
}

type errStatus int

func (e errStatus) Error() string { return "unexpected status " + http.StatusText(int(e)) }

type errRPC string

func (e errRPC) Error() string { return "rpc error: " + string(e) }
