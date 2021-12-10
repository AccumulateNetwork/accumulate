package chain

import (
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

// NewNodeExecutor creates a new Executor for a node.
func NewNodeExecutor(opts ExecutorOptions) (*Executor, error) {
	switch opts.SubnetType {
	case config.Directory:
		return newExecutor(opts,
			SyntheticAnchor{SubnetType: opts.SubnetType},
			SyntheticMirror{},
		)

	case config.BlockValidator:
		return newExecutor(opts,
			CreateIdentity{},
			WithdrawTokens{},
			CreateTokenAccount{},
			CreateDataAccount{},
			AddCredits{},
			CreateKeyPage{},
			CreateKeyBook{},
			UpdateKeyPage{},
			WriteData{},
			SyntheticCreateChain{},
			SyntheticTokenDeposit{},
			SyntheticDepositCredits{},
			SyntheticSignTransactions{},
			SyntheticAnchor{SubnetType: opts.SubnetType},
			SyntheticMirror{},

			// TODO Only for TestNet
			AcmeFaucet{},
		)

	default:
		return nil, fmt.Errorf("invalid subnet type %v", opts.SubnetType)
	}
}

// NewGenesisExecutor creates a transaction executor that can be used to set up
// the genesis state.
func NewGenesisExecutor(db *state.StateDB, typ config.NetworkType) (*Executor, error) {
	m, err := newExecutor(ExecutorOptions{
		DB:         db,
		SubnetType: typ,
	})
	if err != nil {
		return nil, err
	}

	m.isGenesis = true
	return m, nil
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
