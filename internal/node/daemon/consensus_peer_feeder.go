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

// DefaultFeedInterval is how often the feeder refreshes by default.
const DefaultFeedInterval = time.Minute

// Run pulls and dials on Interval until ctx is cancelled. It performs an
// immediate first refresh so a freshly started node converges without
// waiting a full interval.
func (f *ConsensusPeerFeeder) Run(ctx context.Context) {
	interval := f.Interval
	if interval <= 0 {
		interval = DefaultFeedInterval
	}

	if err := f.refreshOnce(ctx); err != nil {
		slog.DebugContext(ctx, "Consensus peer feed: initial refresh failed", "partition", f.Partition, "error", err)
	}

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := f.refreshOnce(ctx); err != nil {
				slog.DebugContext(ctx, "Consensus peer feed: refresh failed", "partition", f.Partition, "error", err)
			}
		}
	}
}

// refreshOnce fetches the current peer list and dials it. It returns an
// error only when the source fails; an empty list or a dial error is
// logged and swallowed so the loop keeps running.
func (f *ConsensusPeerFeeder) refreshOnce(ctx context.Context) error {
	peers, err := f.Source.ConsensusPeers(ctx, f.Partition)
	if err != nil {
		return err
	}
	if len(peers) == 0 {
		return nil
	}
	if f.Dialer == nil {
		return nil
	}
	if err := f.Dialer.DialPeersAsync(peers); err != nil {
		slog.WarnContext(ctx, "Consensus peer feed: dial failed", "partition", f.Partition, "count", len(peers), "error", err)
		return nil
	}
	slog.DebugContext(ctx, "Consensus peer feed: dialed peers", "partition", f.Partition, "count", len(peers))
	return nil
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
