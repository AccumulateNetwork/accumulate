// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"encoding/json"
	"log/slog"
	"net"

	"github.com/libp2p/go-libp2p/core/network"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/consensuspeer"
)

// consensusAdvertiser serves a node's CometBFT endpoints (one per
// partition it runs) over consensuspeer.ProtocolID. The bootstrap server
// reads this to broker consensus reachability (#4043). It is populated
// by each ConsensusService as it starts and shares the instance's single
// libp2p host, so a dual node advertises both its DN and BVN endpoints
// on one stream.
type consensusAdvertiser struct {
	byPartition map[string]consensuspeer.Advertised
	registered  bool
}

// advertiseConsensusEndpoint records this partition's CometBFT endpoint
// and, on first call, registers the advertise stream handler on the
// shared libp2p host. host/port are the externally reachable CometBFT
// P2P address; an empty host is ignored (nothing to advertise).
func (i *Instance) advertiseConsensusEndpoint(partition, host string, port int) {
	if host == "" || port == 0 {
		return
	}

	// A loopback or unspecified host is not externally reachable; the
	// bootstrap server drops such endpoints downstream (isUnroutableHost).
	// Warn so the misconfiguration is visible rather than silently
	// advertising an unusable endpoint.
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		i.logger.Warn("Advertising a non-routable consensus endpoint; bootstrap will drop it",
			"partition", partition, "host", host, "port", port)
	}

	i.consensusAdvMu.Lock()
	defer i.consensusAdvMu.Unlock()

	if i.consensusAdv == nil {
		i.consensusAdv = &consensusAdvertiser{byPartition: map[string]consensuspeer.Advertised{}}
	}
	i.consensusAdv.byPartition[partition] = consensuspeer.Advertised{
		Partition: partition,
		Host:      host,
		Port:      port,
	}

	if !i.consensusAdv.registered && i.p2p != nil {
		i.consensusAdv.registered = true
		i.p2p.Host().SetStreamHandler(consensuspeer.ProtocolID, i.handleConsensusAdvertise)
		i.logger.Info("Serving consensus-peer advertisements", "protocol", consensuspeer.ProtocolID)
	}
}

// handleConsensusAdvertise writes the node's current advertisement as
// JSON and closes the stream.
func (i *Instance) handleConsensusAdvertise(s network.Stream) {
	defer func() { _ = s.Close() }()

	i.consensusAdvMu.Lock()
	var adv consensuspeer.Advertisement
	if i.consensusAdv != nil {
		for _, a := range i.consensusAdv.byPartition {
			adv.Peers = append(adv.Peers, a)
		}
	}
	i.consensusAdvMu.Unlock()

	if err := json.NewEncoder(s).Encode(adv); err != nil {
		slog.Debug("Failed to write consensus-peer advertisement", "peer", s.Conn().RemotePeer(), "error", err)
	}
}
