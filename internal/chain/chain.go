package chain

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NewNodeExecutor creates a new Executor for a node.
func NewNodeExecutor(opts ExecutorOptions, db *database.Database) (*Executor, error) {
	switch opts.Network.Type {
	case config.Directory:
		return newExecutor(opts, db,
			SyntheticAnchor{Network: &opts.Network},
			SyntheticMirror{},

			InternalSendTransactions{},
			InternalTransactionsSigned{},
			InternalTransactionsSent{},

			// for data accounts
			WriteData{},

			// for ACME
			IssueTokens{},
			SyntheticBurnTokens{},

			// DN validator set management
			AddValidator{},
			RemoveValidator{},
			UpdateValidatorKey{},
		)

	case config.BlockValidator:
		return newExecutor(opts, db,
			AddCredits{},
			BurnTokens{},
			CreateDataAccount{},
			CreateIdentity{},
			CreateKeyBook{},
			CreateKeyPage{},
			CreateToken{},
			CreateTokenAccount{},
			IssueTokens{},
			SendTokens{},
			UpdateKeyPage{},
			WriteData{},
			WriteDataTo{},
			UpdateManager{},
			RemoveManager{},

			// BVN validator management
			AddValidator{},
			RemoveValidator{},
			UpdateValidatorKey{},

			// Synthetics...
			SyntheticAnchor{Network: &opts.Network},
			SyntheticBurnTokens{},
			SyntheticCreateChain{},
			SyntheticDepositCredits{},
			SyntheticDepositTokens{},
			SyntheticMirror{},
			SyntheticWriteData{},

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
		Network:   network,
		Logger:    logger,
		isGenesis: true,
	}, db)
}

// TxExecutor executes a specific type of transaction.
type TxExecutor interface {
	// Type is the transaction type the executor can execute.
	Type() protocol.TransactionType

	// Validate fully validates and executes the transaction.
	Validate(*StateManager, *protocol.Envelope) (protocol.TransactionResult, error)
}

// creditChain = protocol.SignerAccount
//
// Deprecated: use protocol.SignerAccount
type creditChain = protocol.SignerAccount

// tokenChain = protocol.TokenHolderAccount
//
// Deprecated: use protocol.TokenHolderAccount
type tokenChain = protocol.TokenHolderAccount
