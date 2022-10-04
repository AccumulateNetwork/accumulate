package net

import (
	"bytes"
	"context"
	"encoding/hex"
	stderr "errors"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

type Client interface {
	api.NodeService
	api.QueryService
	api.EventService
	api.SubmitService
}

type node struct {
	id      [32]byte
	clients map[string]Client
}

type PeerClientFunc func(*api.PeerInfo) (Client, error)

type ConnMgr struct {
	logger    logging.OptionalLogger
	nodes     []*node
	newClient PeerClientFunc
}

func NewConnMgr(logger log.Logger, newClient PeerClientFunc) *ConnMgr {
	c := new(ConnMgr)
	c.logger.Set(logger, "module", "nodenet")
	c.newClient = newClient
	return c
}

func (c *ConnMgr) Register(ctx context.Context, n Client) error {
	status, err := n.Status(ctx)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "get status: %w", err)
	}

	c.register(ctx, n, status, false)
	return nil
}

func (c *ConnMgr) register(ctx context.Context, n Client, status *api.NodeStatus, isPeer bool) {
	// Add an entry for this node
	ptr, create := sortutil.BinaryInsert(&c.nodes, func(n *node) int {
		return bytes.Compare(n.id[:], status.NodeKeyHash[:])
	})
	var nn *node
	if create {
		nn = new(node)
		nn.id = status.NodeKeyHash
		nn.clients = make(map[string]Client)
		*ptr = nn
	}

	// If we're registering a peer (vs a direct call to Register) and a
	// client already exists, leave the existing client
	part := strings.ToLower(status.Partition)
	if isPeer && nn.clients[part] != nil {
		return
	}

	nn.clients[part] = n

	// Register all of the node's peers
	for _, peer := range status.Peers {
		id, err := hex.DecodeString(peer.NodeID)
		if err == nil && len(id) < 20 {
			err = stderr.New("too short")
		}
		if err != nil {
			c.logger.Error("Invalid node ID for peer", "source", nn.id, "peer", peer, "error", err)
			continue
		}

		// Create a client
		cl, err := c.newClient(peer)
		if err != nil {
			c.logger.Error("Unable to create client for peer", "source", nn.id, "peer", peer, "error", err)
			continue
		}

		// Get the node's info
		status, err := n.Status(ctx)
		if err != nil {
			err = errors.Format(errors.StatusUnknownError, "get status: %w", err)
			c.logger.Error("Unable to register peer", "source", nn.id, "peer", peer, "error", err)
			continue
		}

		// Verify the peer ID
		if !bytes.HasPrefix(status.NodeKeyHash[:], id) {
			c.logger.Error("Invalid peer, id does not match node key hash", "source", nn.id, "peer", peer, "nodeKeyHash", logging.AsHex(status.NodeKeyHash))
			continue
		}

		c.register(ctx, cl, status, true)
	}
}
