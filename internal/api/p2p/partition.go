package p2p

import (
	"strings"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

// A MessageStreamHandler handles an incoming [message.Stream].
type MessageStreamHandler func(message.Stream)

// idRpc constructs a [protocol.ID] for the given partition.
func idRpc(part string) protocol.ID {
	part = strings.ToLower(part)
	return "/acc/rpc/" + protocol.ID(part) + "/1.0.0"
}

// partition manages a [Node]'s participation in a partition.
type partition struct {
	id      string
	moniker string
	rpc     MessageStreamHandler
}

// info returns the PartitionInfo for this partition.
func (p *partition) info() *PartitionInfo {
	return &PartitionInfo{
		ID:      p.id,
		Moniker: p.moniker,
	}
}

// SetRpcHandler sets the RPC call handler for the partition. SetRpcHandler
// panics if the node does not participate in the partition.
func (n *Node) SetRpcHandler(partition string, handler MessageStreamHandler) {
	partition = strings.ToLower(partition)
	n.partitions[partition].rpc = handler
	n.host.SetStreamHandler(idRpc(partition), func(s network.Stream) {
		defer s.Close()
		handler(message.NewStream(s))
	})
}
