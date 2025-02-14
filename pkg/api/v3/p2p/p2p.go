// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"crypto/ed25519"
	"net"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
	ma "github.com/multiformats/go-multiaddr"
	mafmt "github.com/multiformats/go-multiaddr-fmt"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/dial"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Node implements peer-to-peer routing of API v3 messages over via binary
// message transport.
type Node struct {
	context  context.Context
	cancel   context.CancelFunc
	peermgr  *peerManager
	host     host.Host
	tracker  dial.Tracker
	services []*serviceHandler
}

// Options are options for creating a [Node].
type Options struct {
	// Network is the network the node is a part of. An empty Network indicates
	// the node is not part of any network.
	Network string

	// Listen is an array of addresses to listen on.
	Listen []multiaddr.Multiaddr

	// BootstrapPeers is an array of addresses of the bootstrap peers to connect
	// to on bootup.
	BootstrapPeers []multiaddr.Multiaddr

	// Key is the node's private key. If Key is omitted, the node will
	// generate a new key.
	Key ed25519.PrivateKey

	// DiscoveryMode determines how the node responds to peer discovery
	// requests.
	DiscoveryMode dht.ModeOpt

	// External is the node's external address
	External multiaddr.Multiaddr

	// EnablePeerTracker enables the peer tracker to reduce the impact of
	// mis-configured peers. This is currently experimental.
	EnablePeerTracker bool

	PeerDatabase         string
	PeerScanFrequency    time.Duration
	PeerPersistFrequency time.Duration
}

// New creates a node with the given [Options].
func New(opts Options) (_ *Node, err error) {
	// Initialize basic fields
	n := new(Node)
	n.context, n.cancel = context.WithCancel(context.Background())

	// Cancel on fail
	defer func() {
		if err != nil {
			n.cancel()
		}
	}()

	// Configure libp2p host options
	options := []config.Option{
		libp2p.ListenAddrs(opts.Listen...),
		libp2p.EnableNATService(),
		libp2p.EnableRelay(),
		libp2p.EnableHolePunching(),
	}

	// If an external address is specified, replace external IPs with that address
	if opts.External != nil {
		options = append(options, libp2p.AddrsFactory(func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
			for i, addr := range addrs {
				first, rest := multiaddr.SplitFirst(addr)
				switch first.Protocol().Code {
				case multiaddr.P_IP4:
					ip := net.ParseIP(first.Value())
					if !ip.IsLoopback() {
						addrs[i] = opts.External.Encapsulate(rest)
					}
				}
			}
			return addrs
		}))
	}

	// Use the given key if specified
	if opts.Key != nil {
		key, _, err := crypto.KeyPairFromStdKey(&opts.Key)
		if err != nil {
			return nil, err
		}
		options = append(options, libp2p.Identity(key))
	}

	// Create the libp2p host
	n.host, err = libp2p.New(options...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			n.host.Close()
		}
	}()

	// Create a peer manager
	n.peermgr, err = newPeerManager(n.context, n.host, func() []*serviceHandler { return n.services }, opts)
	if err != nil {
		return nil, err
	}

	// Register the node service
	mh, err := message.NewHandler(&message.NodeService{NodeService: (*nodeService)(n)})
	if err != nil {
		return nil, err
	}
	n.RegisterService(api.ServiceTypeNode.Address(), mh.Handle)

	// List the node as part of the network
	if opts.Network != "" {
		c, err := multiaddr.NewComponent(api.N_ACC, opts.Network)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("create network multiaddr: %w", err)
		}
		util.Advertise(n.context, n.peermgr.routing, c.String())
	}

	// Set up the peer tracker
	if opts.PeerDatabase != "" {
		n.tracker, err = dial.NewPersistentTracker(n.context, dial.PersistentTrackerOptions{
			Network:          opts.Network,
			Filename:         opts.PeerDatabase,
			SelfID:           n.host.ID(),
			Host:             (*connector)(n),
			Peers:            (*dhtDiscoverer)(n),
			ScanFrequency:    opts.PeerScanFrequency,
			PersistFrequency: opts.PeerPersistFrequency,
		})
		if err != nil {
			return nil, err
		}
	} else if opts.EnablePeerTracker {
		n.tracker = new(dial.SimpleTracker)
	} else {
		n.tracker = dial.FakeTracker
	}

	return n, nil
}

func (n *Node) ID() peer.ID { return n.host.ID() }

func (n *Node) Services() *nodeService { return (*nodeService)(n) }

// Addresses lists the node's addresses.
func (n *Node) Addresses() []multiaddr.Multiaddr {
	// Wrap the TCP/IP address with /p2p/{id}
	id, err := multiaddr.NewComponent("p2p", n.host.ID().String())
	if err != nil {
		return nil
	}

	var addrs []multiaddr.Multiaddr
	for _, a := range n.host.Addrs() {
		addrs = append(addrs, a.Encapsulate(id))
	}
	return addrs
}

// ConnectDirectly connects this node directly to another node.
func (n *Node) ConnectDirectly(m *Node) error {
	if n.ID() == m.ID() {
		return nil
	}

	// TODO Keep the [Node] around so we can create direct connections that
	// avoid the TCP/IP overhead
	return n.host.Connect(context.Background(), peer.AddrInfo{
		ID:    m.ID(),
		Addrs: m.Addresses(),
	})
}

// Close shuts down the host and topics.
func (n *Node) Close() error {
	n.cancel()
	return n.host.Close()
}

// getPeerService returns a new stream for the given peer and service.
func (n *Node) getPeerService(ctx context.Context, peerID peer.ID, service *api.ServiceAddress, ip multiaddr.Multiaddr) (message.Stream, error) {
	if ip != nil {
		ip = oldQuicCompat(ip)

		// libp2p requires that the address include the peer ID
		c, err := multiaddr.NewComponent("p2p", peerID.String())
		if err != nil {
			return nil, errors.InternalError.With(err)
		}
		ip = ip.Encapsulate(c)

		// Tell the host to connect to the specified address
		err = n.host.Connect(ctx, peer.AddrInfo{
			ID:    peerID,
			Addrs: []multiaddr.Multiaddr{ip},
		})
		if err != nil {
			return nil, err
		}
	}

	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	s, err := n.host.NewStream(connCtx, peerID, idRpc(service))
	if err != nil {
		return nil, err
	}

	// Close the stream when the context is canceled
	go func() { <-ctx.Done(); _ = s.Close() }()

	return message.NewStream(s), nil
}

// getOwnService returns a service of this node.
func (n *Node) getOwnService(network string, sa *api.ServiceAddress) (*serviceHandler, bool) {
	if network != "" && !strings.EqualFold(network, n.peermgr.network) {
		return nil, false
	}
	i, ok := sortutil.Search(n.services, func(s *serviceHandler) int { return s.address.Compare(sa) })
	if !ok {
		return nil, false
	}
	return n.services[i], true
}

var quicDraft29DialMatcher = mafmt.And(mafmt.IP, mafmt.Base(ma.P_UDP), mafmt.Base(ma.P_QUIC))

func oldQuicCompat(a multiaddr.Multiaddr) multiaddr.Multiaddr {
	if !quicDraft29DialMatcher.Matches(a) {
		return a
	}

	var b multiaddr.Multiaddr
	multiaddr.ForEach(a, func(c multiaddr.Component) bool {
		if c.Protocol().Code == ma.P_QUIC {
			d, err := ma.NewComponent("quic-v1", "")
			if err != nil {
				panic(err)
			}
			c = *d
		}
		if b == nil {
			b = &c
		} else {
			b = b.Encapsulate(&c)
		}
		return true
	})
	return b
}
