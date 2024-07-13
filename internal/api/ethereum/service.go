package ethimpl

import (
	"context"
	"math/big"

	ethrpc "gitlab.com/accumulatenetwork/accumulate/pkg/api/ethereum"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Service struct {
	Network api.NetworkService
	Query   api.Querier
}

var _ ethrpc.Service = (*Service)(nil)

func (s *Service) EthChainId(ctx context.Context) (*ethrpc.Number, error) {
	ns, err := s.Network.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		return nil, err
	}
	cid := protocol.EthChainID(ns.Network.NetworkName)
	return (*ethrpc.Number)(cid), nil
}

func (s *Service) EthBlockNumber(ctx context.Context) (*ethrpc.Number, error) {
	ns, err := s.Network.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		return nil, err
	}
	return ethrpc.NewNumber(int64(ns.DirectoryHeight)), nil
}

func (s *Service) EthGasPrice(ctx context.Context) (*ethrpc.Number, error) {
	ns, err := s.Network.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		return nil, err
	}
	// Instead of the ACME precision, we have to use 18, because Ethereum
	// wallets require native tokens to have a precision of 18
	v := big.NewInt(1e18 / protocol.CreditPrecision)
	v.Div(v, big.NewInt(int64(ns.Oracle.Price)))
	return (*ethrpc.Number)(v), nil
}

func (s *Service) EthGetBalance(ctx context.Context, addr ethrpc.Address, block string) (*ethrpc.Number, error) {
	u, err := protocol.LiteTokenAddressFromHash(addr[:], "ACME")
	if err != nil {
		return nil, err
	}

	var lite *protocol.LiteTokenAccount
	_, err = api.Querier2{Querier: s.Query}.QueryAccountAs(ctx, u, nil, &lite)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return ethrpc.NewNumber(0), nil
		}
		return nil, err
	}

	value := new(big.Int)
	value.Set(&lite.Balance)

	// Adjust for precision
	value.Mul(value, big.NewInt(1e18/protocol.AcmePrecision))

	return (*ethrpc.Number)(value), nil
}

func (s *Service) EthGetBlockByNumber(ctx context.Context, block string, expand bool) (*ethrpc.BlockData, error) {
	// TODO
	return &ethrpc.BlockData{}, nil
}

func (s *Service) AccTypedData(_ context.Context, txn *protocol.Transaction, sig protocol.Signature) (*encoding.EIP712Call, error) {
	return protocol.NewEIP712Call(txn, sig)
}
