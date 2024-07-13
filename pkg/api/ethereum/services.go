package ethrpc

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Service interface {
	EthChainId(ctx context.Context) (*Number, error)
	EthBlockNumber(ctx context.Context) (*Number, error)
	EthGasPrice(ctx context.Context) (*Number, error)
	EthGetBalance(ctx context.Context, addr Address, block string) (*Number, error)
	EthGetBlockByNumber(ctx context.Context, block string, expand bool) (*BlockData, error)

	AccTypedData(context.Context, *protocol.Transaction, protocol.Signature) (*encoding.EIP712Call, error)
}
