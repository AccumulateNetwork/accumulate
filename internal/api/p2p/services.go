package p2p

import (
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

// A MessageStreamHandler handles an incoming [message.Stream].
type MessageStreamHandler func(message.Stream)

// idRpc constructs a [protocol.ID] for the given partition.
func idRpc(sa *api.ServiceAddress) protocol.ID {
	return "/acc/rpc/" + protocol.ID(sa.String()) + "/1.0.0"
}

// RegisterService registers a service handler.
func (n *Node) RegisterService(sa *api.ServiceAddress, handler MessageStreamHandler) bool {
	ptr, ok := sortutil.BinaryInsert(&n.services, func(s *service) int { return s.address.Compare(sa) })
	if !ok {
		return false
	}
	*ptr = &service{sa, handler}

	n.host.SetStreamHandler(idRpc(sa), func(s network.Stream) {
		defer s.Close()
		handler(message.NewStream(s))
	})

	err := n.peermgr.advertizeNewService()
	if err != nil {
		n.logger.Error("Advertizing failed", "error", err)
	}
	return true
}

// service manages a [Node]'s participation in a service.
type service struct {
	address *api.ServiceAddress
	handler MessageStreamHandler
}

// info returns the PartitionInfo for this partition.
func (s *service) info() *ServiceInfo {
	return &ServiceInfo{
		Address: s.address,
	}
}

// WaitForService blocks until the given service is available. WaitForService
// will return once the service is registered on the current node or until the
// node is informed of a peer with the given service. WaitForService will return
// immediately if the service is already registered or known.
func (s *Node) WaitForService(sa *api.ServiceAddress) {
	s.peermgr.waitFor(sa)
}
