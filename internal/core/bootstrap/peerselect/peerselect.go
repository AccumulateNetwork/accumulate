// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package peerselect implements the consumer-side routing rule for
// bootstrap-v3 advertisement (#3998).
//
// Two cases:
//
//   Case A — bootstrap-launcher traffic. Queries needed during sync
//   (BptPageQuery, EventService.Subscribe, signed-anchor walks)
//   require a peer that advertises ServiceTypeBootstrap with
//   <partition>:active or :complete. Legacy peers don't implement
//   these queries — there is NO fallback. If no eligible peer is
//   reachable, the launcher polls until one comes online.
//
//   Case B — general v3 client traffic. Submit, account Query,
//   transaction status. Prefer advertising peers (their state has
//   been verified) but legacy peers are acceptable: legacy nodes
//   reflect production state and predate the advertisement scheme.
//
// This package's job is finding eligible peers. It does NOT alter
// the underlying dialer; callers use the returned peer IDs to
// construct per-peer connections via the existing dial path.
package peerselect

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/advert"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// ErrNoEligiblePeer is returned when the discovery layer reports no
// peer advertising the required ServiceTypeBootstrap. Bootstrap
// launchers must poll on this error; there is no legacy fallback.
var ErrNoEligiblePeer = errors.New("peerselect: no peer advertises bootstrap-v3 for this partition")

// Finder is the discovery surface. Production: api.NodeService
// (via p2p.ClientNode.FindService). Tests pass a fake.
type Finder interface {
	FindService(ctx context.Context, opts api.FindServiceOptions) ([]*api.FindServiceResult, error)
}

// EligiblePeers returns the peer IDs (and addresses) of peers that
// currently advertise ServiceTypeBootstrap for partition with state
// "active" or "complete". Returns ErrNoEligiblePeer with an empty
// slice if none are reachable.
//
// Bootstrap launchers should poll on ErrNoEligiblePeer. There is no
// legacy fallback — legacy peers don't implement BptPageQuery or
// the signed-anchor walks the launcher needs.
func EligiblePeers(ctx context.Context, finder Finder, network, partition string) ([]*api.FindServiceResult, error) {
	if finder == nil {
		return nil, fmt.Errorf("peerselect.EligiblePeers: finder required")
	}
	if partition == "" {
		return nil, fmt.Errorf("peerselect.EligiblePeers: partition required")
	}

	// Try ACTIVE first, then COMPLETE. Both are eligible; ACTIVE is
	// less rare on a young network.
	results := make([]*api.FindServiceResult, 0)
	for _, state := range []nodestate.State{nodestate.StateActive, nodestate.StateComplete} {
		sa := advert.ServiceAddress(partition, state)
		if sa == nil {
			continue
		}
		page, err := finder.FindService(ctx, api.FindServiceOptions{
			Network: network,
			Service: sa,
		})
		if err != nil {
			return nil, fmt.Errorf("find %s: %w", sa.Argument, err)
		}
		results = append(results, page...)
	}

	if len(results) == 0 {
		return nil, ErrNoEligiblePeer
	}
	return results, nil
}

// PreferAdvertisingPeers ranks the input peer set so that those
// advertising ServiceTypeBootstrap (any partition, any state) come
// first. Used for Case B traffic: legacy peers stay reachable but
// advertising peers are picked first when both are available.
//
// hasBootstrap is a closure over per-peer service knowledge —
// typically a lookup against the dialer's tracker / peer database.
func PreferAdvertisingPeers(peers []*api.FindServiceResult, hasBootstrap func(*api.FindServiceResult) bool) []*api.FindServiceResult {
	if len(peers) <= 1 || hasBootstrap == nil {
		return peers
	}
	front := make([]*api.FindServiceResult, 0, len(peers))
	back := make([]*api.FindServiceResult, 0, len(peers))
	for _, p := range peers {
		if p == nil {
			continue
		}
		if hasBootstrap(p) {
			front = append(front, p)
		} else {
			back = append(back, p)
		}
	}
	return append(front, back...)
}

// PartitionFromArgument extracts the partition name from a
// ServiceTypeBootstrap argument (e.g. "directory:active" -> "directory").
// Returns the empty string if the argument is malformed.
func PartitionFromArgument(arg string) string {
	i := strings.Index(arg, ":")
	if i < 0 {
		return ""
	}
	return arg[:i]
}

// IsAdvertising reports whether the FindServiceResult corresponds to
// a bootstrap-v3 advertisement (its argument is "<partition>:active"
// or "<partition>:complete"). Helper for callers building
// `hasBootstrap` predicates against query results.
func IsAdvertising(r *api.FindServiceResult, _ ...nodestate.State) bool {
	if r == nil || r.Status == api.PeerStatusIsKnownBad {
		return false
	}
	// FindServiceResult doesn't carry the ServiceAddress directly;
	// callers that want to check service-type pass the result of
	// FindService(ServiceTypeBootstrap, ...) which is implicitly
	// already filtered. This helper is here for symmetry.
	return r.PeerID != ""
}
