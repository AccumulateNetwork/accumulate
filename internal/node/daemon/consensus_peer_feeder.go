// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/node/consensuspeer"
)

// ConsensusPeerSource supplies the current set of CometBFT consensus
// peer dial strings (`NodeID@host:port`) for a partition. The bootstrap
// server is the canonical implementation (HTTPConsensusPeerSource); it
// derives the list from the libp2p peers it tracks (#4043).
type ConsensusPeerSource interface {
	ConsensusPeers(ctx context.Context, partition string) ([]string, error)
}

// ConsensusPeerFeeder periodically pulls the consensus peer list for a
// partition from a source and dials those peers through the CometBFT
// switch. This is the consume side of #4043: it lets a node's consensus
// peer set track the bootstrap server instead of a frozen, hand-edited
// persistent_peers list.
//
// It is additive and fail-soft: the statically configured
// persistent_peers remains the cold-start seed, and a source error just
// skips a round. Dialing a peer already connected is a no-op in
// CometBFT, so re-dialing the same list is harmless.
type ConsensusPeerFeeder struct {
	Source    ConsensusPeerSource
	Dialer    PeerDialer
	Partition string
	Interval  time.Duration
}

// Feed intervals. The feeder polls quickly at cold start — when it is
// the only source of consensus peers, the network cannot form a block
// until peers arrive — then settles to a steady refresh once it has
// dialed at least one peer.
const (
	StartupFeedInterval = 5 * time.Second
	DefaultFeedInterval = time.Minute
)

// Run pulls and dials until ctx is cancelled. It polls every
// StartupFeedInterval until the peer count is non-zero and stable across
// two consecutive polls, then switches to the steady interval
// (f.Interval, or DefaultFeedInterval). Waiting for a stable count keeps
// a cold start from locking onto a partial peer set when nodes connect
// to the bootstrap at slightly different times — important because the
// broker is the only source of consensus peers.
func (f *ConsensusPeerFeeder) Run(ctx context.Context) {
	steady := f.Interval
	if steady <= 0 {
		steady = DefaultFeedInterval
	}
	interval := StartupFeedInterval
	if steady < interval {
		interval = steady
	}
	converged := false
	lastN := -1

	for {
		n, err := f.refreshOnce(ctx)
		switch {
		case err != nil:
			slog.DebugContext(ctx, "Consensus peer feed: refresh failed", "partition", f.Partition, "error", err)
		case !converged && n > 0 && n == lastN:
			converged = true
			interval = steady
			slog.InfoContext(ctx, "Consensus peer feed: converged", "partition", f.Partition, "count", n, "steady_interval", steady)
		}
		if !converged {
			lastN = n
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// refreshOnce fetches the current peer list and dials it. It returns the
// number of peers dialed and an error only when the source fails; an
// empty list or a dial error yields (0, nil) so the loop keeps running.
func (f *ConsensusPeerFeeder) refreshOnce(ctx context.Context) (int, error) {
	peers, err := f.Source.ConsensusPeers(ctx, f.Partition)
	if err != nil {
		return 0, err
	}
	if len(peers) == 0 || f.Dialer == nil {
		return 0, nil
	}
	if err := f.Dialer.DialPeersAsync(peers); err != nil {
		slog.WarnContext(ctx, "Consensus peer feed: dial failed", "partition", f.Partition, "count", len(peers), "error", err)
		return 0, nil
	}
	slog.DebugContext(ctx, "Consensus peer feed: dialed peers", "partition", f.Partition, "count", len(peers))
	return len(peers), nil
}

// ConsensusPeerLister is the in-process peer registry interface a node
// satisfies when it runs the embedded registry (peerregistry.PartitionTracker).
// Kept as an interface so this package does not import peerregistry.
type ConsensusPeerLister interface {
	GetConsensusPeers(partition string) []consensuspeer.Peer
}

// LocalConsensusPeerSource serves consensus peers from an in-process peer
// registry — a node that runs the embedded registry reads its own tracker with
// no network round-trip. This is the provider half of "no single bootstrap":
// every required-number registry node can answer locally (#4047).
type LocalConsensusPeerSource struct {
	Registry ConsensusPeerLister
}

// ConsensusPeers implements ConsensusPeerSource.
func (s *LocalConsensusPeerSource) ConsensusPeers(_ context.Context, partition string) ([]string, error) {
	peers := s.Registry.GetConsensusPeers(partition)
	out := make([]string, 0, len(peers))
	for _, p := range peers {
		if d := p.DialString(); d != "" {
			out = append(out, d)
		}
	}
	return out, nil
}

// MultiConsensusPeerSource queries several sources and returns the union of
// their peers, de-duplicated. It is fail-soft: a source that errors is skipped,
// and an error is returned only when EVERY source fails — so a single registry
// node going down cannot starve the consumer. This is the consume-side
// no-SPOF: feed from several registry nodes with failover (#4047, spec §4/§8).
type MultiConsensusPeerSource struct {
	Sources []ConsensusPeerSource
}

// ConsensusPeers implements ConsensusPeerSource.
func (m *MultiConsensusPeerSource) ConsensusPeers(ctx context.Context, partition string) ([]string, error) {
	seen := make(map[string]bool)
	out := make([]string, 0)
	failed := 0
	var lastErr error
	for _, s := range m.Sources {
		peers, err := s.ConsensusPeers(ctx, partition)
		if err != nil {
			failed++
			lastErr = err
			continue
		}
		for _, p := range peers {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	if len(m.Sources) > 0 && failed == len(m.Sources) {
		return nil, fmt.Errorf("all %d consensus-peer sources failed: %w", failed, lastErr)
	}
	return out, nil
}

// HTTPConsensusPeerSource fetches consensus peers from a bootstrap
// server's /consensus-peers/{partition} endpoint.
type HTTPConsensusPeerSource struct {
	// BaseURL is the bootstrap info server root, e.g.
	// "http://bootstrap.example:8080" (no trailing slash required).
	BaseURL string
	// Client is the HTTP client to use; if nil a default client with a
	// short timeout is used.
	Client *http.Client
}

// consensusPeersResponse mirrors the JSON served by the bootstrap
// server's handleConsensusPeers.
type consensusPeersResponse struct {
	Partition       string `json:"partition"`
	Count           int    `json:"count"`
	PersistentPeers string `json:"persistent_peers"`
	Peers           []struct {
		Dial string `json:"dial"`
	} `json:"peers"`
}

// ConsensusPeers implements ConsensusPeerSource.
func (s *HTTPConsensusPeerSource) ConsensusPeers(ctx context.Context, partition string) ([]string, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	url := strings.TrimRight(s.BaseURL, "/") + "/consensus-peers/" + partition
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("consensus-peers: unexpected status %d", resp.StatusCode)
	}

	var body consensusPeersResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("consensus-peers: decode: %w", err)
	}

	// Prefer the per-peer dial strings; fall back to splitting the
	// comma-joined persistent_peers form.
	if len(body.Peers) > 0 {
		out := make([]string, 0, len(body.Peers))
		for _, p := range body.Peers {
			if p.Dial != "" {
				out = append(out, p.Dial)
			}
		}
		return out, nil
	}
	if body.PersistentPeers == "" {
		return nil, nil
	}
	parts := strings.Split(body.PersistentPeers, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}
