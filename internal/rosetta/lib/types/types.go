package types

import (
	"context"

	"github.com/coinbase/rosetta-sdk-go/server"
	"github.com/coinbase/rosetta-sdk-go/types"
)

// SpecVersion defines the specification of rosetta
const SpecVersion = ""

// NetworkInformationProvider defines the interface used to provide information regarding
// the network and the version of the Accumulate used
type NetworkInformationProvider interface {
	// SupportedOperations lists the operations supported by the implementation
	SupportedOperations() []string
	// OperationStatuses returns the list of statuses supported by the implementation
	OperationStatuses() []*types.OperationStatus
	// Version returns the version of the node
	Version() string
}

// Client defines the API the client implementation should provide
type Client interface {
	// Bootstrap needed if the client needs to perform some action before connecting
	Bootstrap() error
	// Ready checks if the servicer constraints for queries are satisfied
	Ready() error

	// Data API

	// Balances fetches the balance of the given address
	// if height is not nil,then the balance will be displayed
	// at the provided height, otherwise last block balance will be returned
	Balances(ctx context.Context, addr string, height *int64) ([]*types.Amount, error)
	// BlockByHash gets a block and its transaction at the provided height
	BlockByHash(ctx context.Context, hash string) (BlockResponse, error)
	// BlockByHeight gets a block given its height, if height is nil then last block is returned
	BlockByHeight(ctx context.Context, height *int64) (BlockResponse, error)
	// BlockTransactionsByHash gets the block, parent block and transactions
	// given the block hash.
	BlockTransactionsByHash(ctx context.Context, hash string) (BlockTransactionsResponse, error)
	// BlockTransactionsByHeight gets the block, parent block and transactions
	// given the block hash.
	BlockTransactionsByHeight(ctx context.Context, height *int64) (BlockTransactionsResponse, error)
	// GetTx gets a transaction given its hash
	GetTx(ctx context.Context, hash string) (*types.Transaction, error)
	// GetUnconfirmedTx gets an unconfirmed Tx given its hash

	// Note:- Not sure about this functionality
	GetUnconfirmedTx(ctx context.Context, hash string) (*types.Transaction, error)
	// Mempool returns the list of the current non confirmed transactions
	Mempool(ctx context.Context) ([]*types.TransactionIdentifier, error)
	// Peers gets the peers currently connected to the node
	Peers(ctx context.Context) ([]*types.Peer, error)
	// Status returns the node status, such as sync data, version etc
	Status(ctx context.Context) (*types.SyncStatus, error)

	OfflineClient
}

// OfflineClient defines the functionalities supported without having access to the node
type OfflineClient interface {
	NetworkInformationProvider
}

type BlockTransactionsResponse struct {
	BlockResponse
	Transactions []*types.Transaction
}

type BlockResponse struct {
	Block                *types.BlockIdentifier
	ParentBlock          *types.BlockIdentifier
	MillisecondTimestamp int64
	TxCount              int64
}

// API defines the exposed API's
// if the service is online
type API interface {
	DataAPI
}

// DataAPI defines the full data API implementation
type DataAPI interface {
	server.NetworkAPIServicer
	server.AccountAPIServicer
	server.BlockAPIServicer
	server.MempoolAPIServicer
}
