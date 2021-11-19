package chain

import (
	"crypto/ed25519"
	"math/big"

	accapi "github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func NewBlockValidator(query *accapi.Query, db *state.StateDB, key ed25519.PrivateKey) (*Executor, error) {
	return NewExecutor(query, db, key,
		CreateIdentity{},
		WithdrawTokens{},
		CreateTokenAccount{},
		AddCredits{},
		CreateKeyPage{},
		CreateKeyBook{},
		UpdateKeyPage{},
		SyntheticGenesis{},
		SyntheticCreateChain{},
		SyntheticTokenDeposit{},
		SyntheticDepositCredits{},

		// TODO Only for TestNet
		AcmeFaucet{},
	)
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
