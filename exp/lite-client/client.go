package liteclient

import (
	"context"
	"fmt"

	blocks "gitlab.com/accumulatenetwork/accumulate/exp/lite-client/blocks"
	"gitlab.com/accumulatenetwork/accumulate/exp/lite-client/signatures"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

type LiteClient struct {
	v2    *client.Client
	v3    api.Querier
	cache map[string]VerifiedAccount
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

	return &LiteClient{
		v2:    v2Client,
		v3:    v3Client,
		cache: make(map[string]VerifiedAccount),
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

	// // PHASE 2: VALIDATE SIGNATURES FROM GENESIS TO CURRENT MAJOR BLOCK
	// // 2.1 RetrieveGenesisBlockAndAuthority() (genesis.go)
	// // Input: context, client
	// // Output: block, keybook, keypage
	genesisBlock, keyBook, keyPage, err := RetrieveGenesisBlockAndAuthority(ctx, c.v2)
	if err != nil {
		return fmt.Errorf("failed to retrieve genesis block and authority: %w for %v, %v, %v", err, genesisBlock, keyBook, keyPage)
	}

	// // 2.2 QueryMajorBlocks()
	// // This step consists of extracting signatures and thresholds for each block
	// // Input: context, client
	authSets, err := buildAuthoritySets(ctx, c)
	if err != nil {
		return fmt.Errorf("failed to build authority sets for %v", authSets)
	}

	// // Output: AuthorityTracker for all blocks (map(index, AuthoritySet))
	// authorityTracker, err := blocks.BuildAuthorityTracker(authoritySets)
	// if err != nil {
	// 	return fmt.Errorf("failed to build authority tracker: %w", err)
	// }

	// // 2.3 Fetch Authorities
	// // This step consists of determining what are the correct authorities
	// // at each block (height, index, timestamp?)
	// // Output: AuthorityTracker for the valid authorities of the major blockchain
	// // 2.4 ValidateFromGenesisToCurrent()
	// // This step consists of checking, index by index both Authority Trackers
	// // to see if they match. If they do, that means we can trust information given
	// // by the node?

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

func buildAuthoritySets(ctx context.Context, c *LiteClient) ([]*signatures.AuthoritySet, error) {
	// 2.2.1 QueryMajorBlock()
	// Input: context, client, index
	// Output: AuthoritySet for 1 block (Sigs + Threshold)
	var authoritySets []*signatures.AuthoritySet
	for i := uint64(0); ; i++ {
		majorBlocks, err := blocks.QueryMajorBlocks(ctx, c.v2, "acc://bvn0.acme", i, 1, "v2")
		if err != nil {
			return nil, fmt.Errorf("failed to query major block %d: %w", i, err)
		}
		if len(majorBlocks) == 0 {
			// No more blocks
			break
		}
		authSet, err := blocks.ExtractAuthoritySet(majorBlocks[0])
		if err != nil {
			return nil, fmt.Errorf("failed to extract AuthoritySet for block %d: %w", i, err)
		}
		authoritySets = append(authoritySets, authSet)
	}
	return authoritySets, nil
}
