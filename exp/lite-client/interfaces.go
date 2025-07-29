package liteclient

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NetworkClientInterface defines the interface for all network operations
type NetworkClientInterface interface {
	// V2 API methods
	QueryV2(ctx context.Context, query v2.Query) (v2.QueryResponse, error)
	
	// V3 API methods
	QueryV3(ctx context.Context, scope *v3.MessageRecord, query v3.Query) (*v3.QueryResponse, error)
	
	// Convenience methods for common operations
	GetAccount(ctx context.Context, accountURL string) (*v2.AccountQueryResponse, error)
	GetTransactionHistory(ctx context.Context, accountURL string, start uint64, count uint64) (*v2.TransactionQueryResponse, error)
	GetChainEntry(ctx context.Context, chainID string, entryHash []byte) (*v3.QueryResponse, error)
}

// AccountServiceInterface defines the interface for account data operations
type AccountServiceInterface interface {
	GetAccountData(ctx context.Context, accountURL string) (*AccountData, error)
	GetAccountType(accountData interface{}) (AccountType, string)
	CategorizeAccount(accountType AccountType) AccountCategory
}

// ProofServiceInterface defines the interface for proof operations
type ProofServiceInterface interface {
	FetchProof(ctx context.Context, accountURL string) (*merkle.Receipt, error)
	ValidateProof(ctx context.Context, proof *merkle.Receipt, accountURL string) error
	GetBPTRootHash(ctx context.Context) ([]byte, error)
}

// CacheServiceInterface defines the interface for caching operations
type CacheServiceInterface interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl time.Duration)
	Delete(key string)
	Clear()
	IsStale(key string, maxAge time.Duration) bool
}

// OrchestrationServiceInterface defines the interface for orchestration operations
type OrchestrationServiceInterface interface {
	ProcessTargetADIs(ctx context.Context, adis []string) (*ADIProcessingReport, error)
	DiscoverAccounts(ctx context.Context, adiURL string) ([]string, error)
	ProcessAccount(ctx context.Context, accountURL string) (*VerifiedAccount, error)
}
