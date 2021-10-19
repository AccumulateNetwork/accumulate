package chain

import (
	"crypto/ed25519"
	"fmt"
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
	CheckTx(*state.StateEntry, *transactions.GenTransaction) error

	// DeliverTx fully validates and executes the transaction.
	DeliverTx(*state.StateEntry, *transactions.GenTransaction) (*DeliverTxResult, error)
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

type DeliverTxResult struct {
	SyntheticTransactions []*transactions.GenTransaction //this is a list of synthetic transactions
	Chains                map[[32]byte]state.Chain
}

func (r *DeliverTxResult) AddSyntheticTransaction(tx *transactions.GenTransaction) {
	r.SyntheticTransactions = append(r.SyntheticTransactions, tx)
}

func (r *DeliverTxResult) AddChain(ch state.Chain) {
	u, err := url.Parse(ch.GetChainUrl())
	if err != nil {
		// The caller must ensure the chain URL is correct
		panic(fmt.Errorf("attempted to add an invalid chain: %v", err))
	}
	if r.Chains == nil {
		r.Chains = map[[32]byte]state.Chain{}
	}
	var chainId [32]byte
	copy(chainId[:], u.ResourceChain())
	r.Chains[chainId] = ch
}
