package block

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
)

// NewNodeExecutor creates a new Executor for a node.
func NewNodeExecutor(opts ExecutorOptions, db *database.Database) (*Executor, error) {
	switch opts.Network.Type {
	case config.Directory:
		return newExecutor(opts, db,
			SyntheticAnchor{Network: &opts.Network},
			SyntheticMirror{},
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
			UpdateKeyPage{},
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
			SyntheticAnchor{Network: &opts.Network},
			SyntheticBurnTokens{},
			SyntheticCreateIdentity{},
			SyntheticDepositCredits{},
			SyntheticDepositTokens{},
			SyntheticMirror{},
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
