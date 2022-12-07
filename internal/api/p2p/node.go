package p2p

import (
	"context"
	"crypto/ed25519"
	"io"
	"strings"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	errors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Node implements peer-to-peer routing of API v3 messages over via binary
// message transport.
type Node struct {
	logger     logging.OptionalLogger
	context    context.Context
	cancel     context.CancelFunc
	peermgr    *peerManager
	host       host.Host
	partitions map[string]*partition
	// valInfo    *protocol.ValidatorInfo
}

// Options are options for creating a [Node].
type Options struct {
	Logger log.Logger

	// Listen is an array of addresses to listen on.
	Listen []multiaddr.Multiaddr

	// BootstrapPeers is an array of addresses of the bootstrap peers to connect
	// to on bootup.
	BootstrapPeers []multiaddr.Multiaddr

	// Key is the node's private key. If Key is omitted, the node will
	// generate a new key.
	Key ed25519.PrivateKey

	// Partitions are the partitions the node participates in.
	Partitions []PartitionOptions

	// Validator      *protocol.ValidatorInfo
}

// PartitionOptions defines a [Node]'s involvement in a partition.
type PartitionOptions struct {
	// ID is the ID of the partition.
	ID string

	// Moniker is the Tendermint node's moniker.
	Moniker string
}

// New creates a node with the given [Options].
func New(opts Options) (_ *Node, err error) {
	if len(opts.Partitions) == 0 {
		return nil, errors.BadRequest.With("cannot start node without any partitions")
	}

	// Initialize basic fields
	n := new(Node)
	n.logger.Set(opts.Logger, "module", "acc-p2p")
	n.context, n.cancel = context.WithCancel(context.Background())
	n.partitions = make(map[string]*partition, len(opts.Partitions))

	for _, opts := range opts.Partitions {
		part := new(partition)
		part.id = opts.ID
		part.moniker = opts.Moniker
		n.partitions[strings.ToLower(opts.ID)] = part
	}

	// Cancel on fail
	defer func() {
		if err != nil {
			n.cancel()
		}
	}()

	// Configure libp2p host options
	options := []config.Option{
		libp2p.ListenAddrs(opts.Listen...),
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
	n.peermgr, err = newPeerManager(n.host, opts)
	if err != nil {
		return nil, err
	}

	return n, nil
}

// Addrs lists the node's addresses.
func (n *Node) Addrs() []multiaddr.Multiaddr {
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

// Peers lists the node's known peers.
func (n *Node) Peers() []*Info {
	n.peermgr.mu.RLock()
	defer n.peermgr.mu.RUnlock()
	peers := make([]*Info, 0, len(n.peermgr.peers))
	for _, p := range n.peermgr.peers {
		peers = append(peers, p.info)
	}
	return peers
}

// ConnectDirectly connects this node directly to another node.
func (n *Node) ConnectDirectly(m *Node) error {
	// TODO Keep the Node around so we can create direct connections that avoid
	// the TCP/IP overhead
	n.peermgr.addKnown(m.info())
	return n.peermgr.connectTo(&peer.AddrInfo{
		ID:    m.host.ID(),
		Addrs: m.host.Addrs(),
	})
}

// Close shuts down the host and topics.
func (n *Node) Close() error {
	n.cancel()
	return n.host.Close()
}

// info returns the Info for this node.
func (n *Node) info() *Info {
	info := &Info{ID: n.host.ID()}
	for _, p := range n.partitions {
		info.Partitions = append(info.Partitions, p.info())
	}
	return info
}

// selfID returns the node's ID.
func (n *Node) selfID() peer.ID {
	return n.host.ID()
}

// newRpcStream returns a new stream for the given peer and partition.
func (n *Node) newRpcStream(ctx context.Context, peer peer.ID, partition string) (io.ReadWriteCloser, error) {
	return n.host.NewStream(ctx, peer, idRpc(partition))
}

// getSelf returns a partition of this node.
func (n *Node) getSelf(part string) *partition {
	return n.partitions[strings.ToLower(part)]
}
