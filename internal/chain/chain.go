package chain

import (
	"math/big"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

// NewBlockValidatorExecutor creates a new Executor for a block validator node.
func NewBlockValidatorExecutor(opts ExecutorOptions) (*Executor, error) {
	return NewExecutor(opts, false,
		CreateIdentity{},
		WithdrawTokens{},
		CreateTokenAccount{},
		AddCredits{},
		CreateKeyPage{},
		CreateKeyBook{},
		UpdateKeyPage{},
		SyntheticCreateChain{},
		SyntheticTokenDeposit{},
		SyntheticDepositCredits{},
		SyntheticSignTransactions{},

		// TODO Only for TestNet
		AcmeFaucet{},
	)
}

func NewDirectoryExecutor(opts ExecutorOptions) (*Executor, error) {
	return NewExecutor(opts, true) // TODO Add DN validators
}

// TxExecutor executes a specific type of transaction.
type TxExecutor interface {
	// Type is the transaction type the executor can execute.
	Type() types.TxType

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
