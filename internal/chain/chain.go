package chain

import (
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/tendermint/tendermint/libs/log"
)

// NewNodeExecutor creates a new Executor for a node.
func NewNodeExecutor(opts ExecutorOptions) (*Executor, error) {
	switch opts.Network.Type {
	case config.Directory:
		return newExecutor(opts,
			SyntheticAnchor{Network: &opts.Network},
			SyntheticMirror{},

			InternalSendTransactions{},
			InternalTransactionsSigned{},
			InternalTransactionsSent{},
		)

	case config.BlockValidator:
		return newExecutor(opts,
			AddCredits{},
			CreateDataAccount{},
			CreateIdentity{},
			CreateKeyBook{},
			CreateKeyPage{},
			CreateTokenAccount{},
			SendTokens{},
			UpdateKeyPage{},
			WriteData{},

			SyntheticAnchor{Network: &opts.Network},
			SyntheticCreateChain{},
			SyntheticDepositCredits{},
			SyntheticDepositTokens{},
			SyntheticMirror{},

			InternalSendTransactions{},
			InternalTransactionsSigned{},
			InternalTransactionsSent{},

			// TODO Only for TestNet
			AcmeFaucet{},
		)

	default:
		return nil, fmt.Errorf("invalid subnet type %v", opts.Network.Type)
	}
}

// NewGenesisExecutor creates a transaction executor that can be used to set up
// the genesis state.
func NewGenesisExecutor(db *database.Database, logger log.Logger, network config.Network) (*Executor, error) {
	return newExecutor(ExecutorOptions{
		DB:        db,
		Network:   network,
		Logger:    logger,
		isGenesis: true,
	})
}

// TxExecutor executes a specific type of transaction.
type TxExecutor interface {
	// Type is the transaction type the executor can execute.
	Type() types.TransactionType

	// Validate fully validates and executes the transaction.
	Validate(*StateManager, *transactions.GenTransaction) error
}

type creditChain interface {
	state.Chain
	CreditCredits(amount uint64)
	DebitCredits(amount uint64) bool
}

type tokenChain interface {
	state.Chain
	NextTx() uint64
	ParseTokenUrl() (*url.URL, error)
	CreditTokens(amount *big.Int) bool
	CanDebitTokens(amount *big.Int) bool
	DebitTokens(amount *big.Int) bool
}
