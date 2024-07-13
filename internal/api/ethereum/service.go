package ethimpl

import (
	"context"

	ethrpc "gitlab.com/accumulatenetwork/accumulate/pkg/api/ethereum"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Service struct {
	Network api.NetworkService
}

var _ ethrpc.Service = (*Service)(nil)

func (s *Service) EthChainId(ctx context.Context) (ethrpc.Bytes, error) {
	ns, err := s.Network.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		return nil, err
	}
	cid := protocol.EthChainID(ns.Network.NetworkName)
	return cid.Bytes(), nil
}

func (s *Service) EthBlockNumber(ctx context.Context) (uint64, error) {
	ns, err := s.Network.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		return 0, err
	}
	return ns.DirectoryHeight, nil
}

func (s *Service) AccTypedData(_ context.Context, txn *protocol.Transaction, sig protocol.Signature) (*encoding.EIP712Call, error) {
	return protocol.NewEIP712Call(txn, sig)
}
