package chain

import (
	"crypto/ed25519"
	"math/big"

	accapi "github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

func NewBlockValidator(query *accapi.Query, db *state.StateDB, key ed25519.PrivateKey) (*Executor, error) {
	return NewExecutor(query, db, key,
		IdentityCreate{},
		TokenTx{},
		TokenAccountCreate{},
		AddCredits{},
		CreateSigSpec{},
		CreateSigSpecGroup{},
		UpdateKeyPage{},
		SyntheticGenesis{},
		SyntheticCreateChain{},
		SyntheticTokenDeposit{},
		SyntheticDepositCredits{},
	)
}

// TxExecutor executes a specific type of transaction.
type TxExecutor interface {
	// Type is the transaction type the executor can execute.
	Type() types.TxType

	// CheckTx partially validates the transaction.
	CheckTx(*StateManager, *transactions.GenTransaction) error

	// DeliverTx fully validates and executes the transaction.
	DeliverTx(*StateManager, *transactions.GenTransaction) error
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
