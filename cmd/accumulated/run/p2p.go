// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"strconv"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/peerregistry"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func (p *P2P) start(inst *Instance) error {
	// Two-plane concealment guardrail (#4047 §2): refuse to start a private node
	// with a publicly-reachable listen, and force DHT client mode so it never
	// advertises itself. Must run before the P2P host binds.
	if err := p.enforcePrivateConcealment(inst); err != nil {
		return err
	}

	sk, err := getPrivateKey(p.Key, inst)
	if err != nil {
		return err
	}

	setDefaultPtr(&p.PeerDB, "")
	node, err := p2p.New(p2p.Options{
		Key:               sk,
		Network:           inst.config.Network,
		Listen:            p.Listen,
		BootstrapPeers:    p.BootstrapPeers,
		PeerDatabase:      *p.PeerDB,
		EnablePeerTracker: p.EnablePeerTracking,
	})
	if err != nil {
		return err
	}
	inst.p2p = node

	// Run the embedded peer registry on this node's own libp2p host when
	// enabled — the no-SPOF model: a required number of nodes each hold the
	// peer map and answer locally, instead of one standalone bootstrap (#4047).
	if p.Registry != nil && *p.Registry {
		var seeds []peer.AddrInfo
		for _, addr := range p.BootstrapPeers {
			if ai, err := peer.AddrInfoFromP2pAddr(addr); err == nil {
				seeds = append(seeds, *ai)
			}
		}
		inst.consensusRegistry = peerregistry.StartEmbedded(node.Host(), node.DHT(), seeds)
		slog.InfoContext(inst.context, "Started embedded peer registry", "module", "run")
		inst.cleanup("peer registry", func(context.Context) error {
			inst.consensusRegistry.Stop()
			return nil
		})
	}

	slog.InfoContext(inst.context, "We are", "node-id", node.ID(), "instance-id", inst.id, "module", "run")

	inst.cleanup("p2p node", func(context.Context) error {
		err := node.Close()
		if err != nil {
			return err
		}
		slog.InfoContext(inst.context, "Stopped", "id", node.ID(), "module", "run")
		return nil
	})
	return nil
}

type DhtMode dht.ModeOpt

func (d DhtMode) String() string {
	switch dht.ModeOpt(d) {
	case dht.ModeAuto:
		return "auto"
	case dht.ModeClient:
		return "client"
	case dht.ModeServer:
		return "server"
	case dht.ModeAutoServer:
		return "auto-server"
	}
	return strconv.FormatInt(int64(d), 10)
}

func (d DhtMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *DhtMode) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	switch s {
	case "auto":
		*d = DhtMode(dht.ModeAuto)
		return nil
	case "client":
		*d = DhtMode(dht.ModeClient)
		return nil
	case "server":
		*d = DhtMode(dht.ModeServer)
		return nil
	case "auto-server":
		*d = DhtMode(dht.ModeAutoServer)
		return nil
	}

	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*d = DhtMode(dht.ModeOpt(i))
	return nil
}

// enforcePrivateConcealment applies the two-plane concealment guardrail for a
// private (guarded) node (#4047 §2). A private node must be reachable only by
// its guards: it refuses to start with a publicly-routable libp2p listen, and
// is forced into DHT client mode so it never advertises itself. Leaking the
// libp2p address would also leak the CometBFT node ID via key derivation, so
// concealment must hold on both planes. No-op for a public node.
func (p *P2P) enforcePrivateConcealment(inst *Instance) error {
	if p.Private == nil || !*p.Private {
		return nil
	}
	for _, addr := range p.Listen {
		_, host, _, _, err := decomposeListen(addr)
		if err == nil && isPublicHost(host) {
			return errors.BadRequest.WithFormat(
				"private node must not listen on public address %q: a guarded validator binds guard-facing only (#4047 §2)", host)
		}
	}
	clientMode := DhtMode(dht.ModeClient)
	p.DiscoveryMode = &clientMode
	if inst != nil && inst.logger != nil {
		inst.logger.Info("Private node: forcing DHT client mode so it never advertises itself (#4047 §2)")
	}
	return nil
}

// isPublicHost reports whether a listen host is publicly reachable. Loopback,
// RFC1918/ULA private, and link-local addresses are guard-facing and allowed;
// the unspecified address (0.0.0.0/::) and globally-routable IPs are public. A
// non-IP host (DNS name) is treated as public, conservatively.
func isPublicHost(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return true
	}
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}
