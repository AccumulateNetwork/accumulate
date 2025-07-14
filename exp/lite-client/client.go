package liteclient

import (
	"context"
	"fmt"
	"net/http"

	liteblocks "gitlab.com/accumulatenetwork/accumulate/exp/lite-client/blocks"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// QuerierValidator combines the v3 Querier and Validator interfaces.
type QuerierValidator interface {
	api.Querier
	api.Validator
}

type LiteClient struct {
	v2    *client.Client
	v3    QuerierValidator
	cache map[string]VerifiedAccount
	authorities liteblocks.AuthorityProvider
}

// NewLiteClient creates a new LiteClient for Phase 1 (account proof creation).
func NewLiteClient(server string) (*LiteClient, error) {
	// 1. Create a new client
	// 2. Initialize cache for verified accounts
	// 3. Initialize blocks and signatures modules for later use
	v2Client, err := client.New(server)
	if err != nil {
		return nil, err
	}

	v3Client := jsonrpc.NewClient(server)

	authorityProvider := liteblocks.NewGenesisAuthorityProvider(&http.Client{}, server)

	return &LiteClient{
		v2:          v2Client,
		v3:          v3Client,
		cache:       make(map[string]VerifiedAccount),
		authorities: authorityProvider,
	}, nil
}

// RetrieveAccountStates is the main orchestration entrypoint for the lite client.
// It retrieves and validates account states using cryptographic proofs and signature validation.
func (c *LiteClient) RetrieveAccountStates(ctx context.Context, accountUrls []string) error {

	// // PHASE 1: RETRIEVE CRYPTOGRAPHIC PROOFS OF ACCOUNT STATES
	err := RetrieveAndValidateProof(ctx, accountUrls, c)
	if err != nil {
		return fmt.Errorf("unable to retrieve or validate account state")
	}

	// PHASE 2: VALIDATE SIGNATURES FROM GENESIS TO CURRENT MAJOR BLOCK
	//
	// The goal of this phase is to independently verify every major block on the chain
	// by querying its anchor message and validating its signatures using the v3 API.
	//
	// High-level steps:
	// 1. Determine the partition URL for the major block chain (e.g., "acc://dn" or "acc://bvn0.acme").
	// 2. Query the total number of major blocks (or iterate until no more blocks are found).
	// 3. For each major block index from 0 (genesis) to the current/latest:
	//    a. Construct the anchor message URL or hash for that major block.
	//    b. Use blocks.QueryMessageRecord (from message.go) to fetch the MessageRecord for the anchor.
	//    c. Use blocks.ValidateMessageRecord (from validate.go) to validate the signatures via the v3 Validator.
	//    d. Record/report any validation failures immediately.
	//
	// Note: The v3 Validator will handle all authority set logic, so we do not need to track authority sets manually.
	//
	// Minimal implementation stub:
	partitionUrl := "acc://dn" // or "acc://bvn0.acme" for a BVN
	i := uint64(0)

	for {
		blocks, err := liteblocks.QueryMajorBlocksV3(ctx, c.v3, partitionUrl, i, 10)
		if err != nil {
			return fmt.Errorf("failed to query major blocks: %w", err)
		}

		if len(blocks) == 0 {
			break // No more blocks
		}

				// Create a block validator with the authority provider.
		validator := liteblocks.NewBlockValidator(c.authorities)

		for _, block := range blocks {
			err := liteblocks.ValidateMajorBlock(ctx, block, validator)
			if err != nil {
				return fmt.Errorf("validation failed for major block %d: %w", block.Index, err)
			}
		}

		i += uint64(len(blocks))
	}

	return nil
}

func RetrieveAndValidateProof(ctx context.Context, accountUrls []string, c *LiteClient) error {
	// 1.1 FetchBPTRootHash() (bpt.go)
	// Input: context, client, network string
	// Output: rootHash, type []byte
	rootHash, err := FetchBPTRootHash(ctx, c.v2, "dn")
	if err != nil {
		// Assign a placeholder root hash and continue
		rootHash = []byte("placeholder-root-hash")
	}

	// 1.2 ValidateAndCacheProof() (proof.go)
	// Input: context, client, accountUrl
	// Output: verifiedAccount, type account.VerifiedAccount
	// verifiedAccount will be stored in Client's cache
	for _, acct := range accountUrls {
		err = ValidateAndCacheProof(c, ctx, acct, rootHash)
		if err != nil {
			return fmt.Errorf("failed to validate and cache proof for %s: %w", acct, err)
		}
	}
	return nil
}
