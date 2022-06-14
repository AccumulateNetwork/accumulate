package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/v1"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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
	SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) (fallback bool, err error)

	// TransactionIsReady checks if the transaction is ready to be executed.
	TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error)
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

type AuthDelegate interface {
	GetAccountAuthoritySet(*database.Batch, protocol.Account) (*protocol.AccountAuth, error)
	SignerIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) error
	AuthorityIsSatisfied(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus, authUrl *url.URL) (bool, error)
	SignerIsSatisfied(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus, signer protocol.Signer) (bool, error)
}
