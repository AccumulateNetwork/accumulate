package block

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var _ SignerValidator = (*CreateIdentity)(nil)
var _ SignerValidator = (*UpdateKeyPage)(nil)
var _ SignerValidator = (*WriteData)(nil)
var _ SignerValidator = (*AddValidator)(nil)
var _ SignerValidator = (*RemoveValidator)(nil)
var _ SignerValidator = (*UpdateValidatorKey)(nil)

var _ PrincipalValidator = (*CreateIdentity)(nil)
var _ PrincipalValidator = (*SyntheticDepositCredits)(nil)

var _ TransactionExecutorCleanup = (*SyntheticDepositTokens)(nil)

// NewNodeExecutor creates a new Executor for a node.
func NewNodeExecutor(opts ExecutorOptions, db *database.Database) (*Executor, error) {
	switch opts.Network.Type {
	case config.Directory:
		return newExecutor(opts, db,
			PartitionAnchor{Network: &opts.Network},
			MirrorSystemRecords{},
			SyntheticForwardTransaction{},

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
			UpdateAccountAuth{},
			UpdateKey{},

			// BVN validator management
			AddValidator{},
			RemoveValidator{},
			UpdateValidatorKey{},

			// Synthetics...
			DirectoryAnchor{Network: &opts.Network},
			MirrorSystemRecords{},
			SyntheticBurnTokens{},
			SyntheticCreateIdentity{},
			SyntheticDepositCredits{},
			SyntheticDepositTokens{},
			SyntheticWriteData{},
			SyntheticForwardTransaction{},

			// TODO Only for TestNet
			AcmeFaucet{},
		)

	default:
		return nil, fmt.Errorf("invalid subnet type %v", opts.Network.Type)
	}
}

// NewGenesisExecutor creates a transaction executor that can be used to set up
// the genesis state.
func NewGenesisExecutor(db *database.Database, logger log.Logger, network config.Network, router routing.Router) (*Executor, error) {
	return newExecutor(ExecutorOptions{
		Network:   network,
		Logger:    logger,
		Router:    router,
		isGenesis: true,
	}, db)
}

// TransactionExecutor executes a specific type of transaction.
type TransactionExecutor interface {
	// Type is the transaction type the executor can execute.
	Type() protocol.TransactionType

	// Validate validates the transaction for acceptance.
	Validate(*StateManager, *Delivery) (protocol.TransactionResult, error)

	// Execute fully validates and executes the transaction.
	Execute(*StateManager, *Delivery) (protocol.TransactionResult, error)
}

// SignerValidator validates signatures for a specific type of transaction.
type SignerValidator interface {
	TransactionExecutor

	// SignerIsAuthorized checks if the signature is authorized for the
	// transaction.
	SignerIsAuthorized(*database.Batch, *protocol.Transaction, protocol.Signer) (fallback bool, err error)

	// TransactionIsReady checks if the transaction is ready to be executed.
	TransactionIsReady(*database.Batch, *protocol.Transaction, *protocol.TransactionStatus) (ready, fallback bool, err error)
}

// PrincipalValidator validates the principal for a specific type of transaction.
type PrincipalValidator interface {
	TransactionExecutor

	AllowMissingPrincipal(*protocol.Transaction) (allow, fallback bool)
}

// TransactionExecutorCleanup cleans up after a failed transaction.
type TransactionExecutorCleanup interface {
	// DidFail is called if the transaction failed.
	DidFail(*ProcessTransactionState, *protocol.Transaction) error
}
