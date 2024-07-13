package ethrpc

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Service interface {
	EthChainId(context.Context) (Bytes, error)
	EthBlockNumber(context.Context) (uint64, error)

	AccTypedData(context.Context, *protocol.Transaction, protocol.Signature) (*encoding.EIP712Call, error)
}
