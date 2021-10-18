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
	StateData     map[types.Bytes32]types.Bytes  //acctypes.StateObject
	MainChainData map[types.Bytes32]types.Bytes  //stuff to store on pending chain.
	PendingData   map[types.Bytes32]types.Bytes  //stuff to store on pending chain.
	EventData     []byte                         //this should be events that need to get published
	Submissions   []*transactions.GenTransaction //this is a list of synthetic transactions
}

func (r *DeliverTxResult) AddMainChainData(chainid *types.Bytes32, data []byte) {
	if r.MainChainData == nil {
		r.MainChainData = make(map[types.Bytes32]types.Bytes)
	}
	r.MainChainData[*chainid] = data
}

func (r *DeliverTxResult) AddStateData(chainid *types.Bytes32, data []byte) {
	if r.StateData == nil {
		r.StateData = make(map[types.Bytes32]types.Bytes)
	}
	r.StateData[*chainid] = data
}

func (r *DeliverTxResult) AddPendingData(txId *types.Bytes32, data []byte) {
	if r.PendingData == nil {
		r.PendingData = make(map[types.Bytes32]types.Bytes)
	}
	r.PendingData[*txId] = data
}

func (r *DeliverTxResult) AddSyntheticTransaction(tx *transactions.GenTransaction) {
	r.Submissions = append(r.Submissions, tx)
}
