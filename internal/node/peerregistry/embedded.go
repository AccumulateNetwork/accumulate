// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerregistry

import (
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/consensuspeer"
)

// Embedded runs the peer registry in-process on a node's own libp2p host. This
// is the no-SPOF model from #4047: instead of one standalone bootstrap server
// holding the peer map, a required number of nodes each run the registry and
// answer locally. It bundles the four tracking components (partition tracker,
// connection manager, version tracker, active discovery) on the host the node
// already has — no second libp2p instance.
type Embedded struct {
	tracker *PartitionTracker
	conn    *ConnectionManager
	version *VersionTracker
	disc    *ActiveDiscovery
}

// StartEmbedded wires and starts the registry cluster on the given host and
// DHT, seeding discovery from bootstrapPeers.
func StartEmbedded(h host.Host, d *dht.IpfsDHT, bootstrapPeers []peer.AddrInfo) *Embedded {
	m := NewMetricsCollector(h)
	pt := NewPartitionTracker(h, m) // self-starts its probe/prune loops
	cm := NewConnectionManager(h, pt, m, DefaultConnectionConfig())
	cm.Start()
	vt := NewVersionTracker(pt)
	vt.Start()
	ad := NewActiveDiscovery(h, d, pt, m, DefaultDiscoveryConfig())
	ad.SetBootstrapPeers(bootstrapPeers)
	ad.Start()
	return &Embedded{tracker: pt, conn: cm, version: vt, disc: ad}
}

// GetConsensusPeers returns the tracked consensus peers for a partition. This
// satisfies the in-process source interface the feeder consumes
// (daemon.ConsensusPeerLister).
func (e *Embedded) GetConsensusPeers(partition string) []consensuspeer.Peer {
	return e.tracker.GetConsensusPeers(partition)
}

// GetServicePeers returns the libp2p peers known to serve a partition, as
// AddrInfos to dial. This is the delivery-discovery backstop (#4047 §10): when
// DHT service discovery is slow/stale under churn, the dialer can fall back to
// the partition's known peers from the directory the node already holds.
// Private peers are already excluded by GetPeersByPartition.
func (e *Embedded) GetServicePeers(partition string) []peer.AddrInfo {
	infos := e.tracker.GetPeersByPartition(partition)
	out := make([]peer.AddrInfo, 0, len(infos))
	for _, in := range infos {
		out = append(out, peer.AddrInfo{ID: in.PeerID, Addrs: in.Addresses})
	}
	return out
}

// Stop shuts the registry down.
func (e *Embedded) Stop() {
	if e == nil {
		return
	}
	e.disc.Stop()
	e.version.Stop()
	e.conn.Stop()
	e.tracker.Stop()
}
