package liteclient

import (
	"context"
	"fmt"

	account "gitlab.com/accumulatenetwork/accumulate/exp/lite-client/account"
	"gitlab.com/accumulatenetwork/accumulate/exp/lite-client/blocks"
	"gitlab.com/accumulatenetwork/accumulate/exp/lite-client/signatures"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

type LiteClient struct {
	v2    *client.Client
	cache map[string]account.VerifiedAccount
}

// NewLiteClient creates a new LiteClient for Phase 1 (account proof creation).
func NewLiteClient(server string) (*LiteClient, error) {
	// 1. Create a new client
	// 2. Initialize cache for verified accounts
	// 3. Initialize blocks and signatures modules for later use
	cli, err := client.New(server)
	if err != nil {
		return nil, err
	}
	return &LiteClient{
		v2:    cli,
		cache: make(map[string]account.VerifiedAccount),
	}, nil
}

// QueryAllMajorBlocks fetches all major blocks from the node.
// For now, this fetches up to 10,000 blocks starting from index 0.
func (c *LiteClient) QueryAllMajorBlocks(ctx context.Context) ([]map[string]interface{}, error) {
	const maxBlocks = 10000
	return blocks.QueryMajorBlocks(ctx, c.v2, 0, maxBlocks)
}

// RetrieveAccountStates is the main orchestration entrypoint for the lite client.
// It retrieves and validates account states using cryptographic proofs and signature validation.
func (c *LiteClient) RetrieveAccountStates(ctx context.Context, accountUrls []string) error {

	// PHASE 1: RETRIEVE CRYPTOGRAPHIC PROOFS OF ACCOUNT STATES

	// 1.1 FetchLatestMajorBlockRootHash() (blocks/block_major.go)
	// Input: context, client, timestamp
	// Output: rootHash, type []byte
	rootHash, err := blocks.FetchLatestMajorBlockRootHash(ctx, c.v2)
	if err != nil {
		return fmt.Errorf("failed to fetch latest major block root hash: %w", err)
	}

	// 1.2 ValidateAndCacheProof() (account/proof.go)
	// Input: context, client, accountUrl
	// Output: verifiedAccount, type account.VerifiedAccount
	for _, acct := range accountUrls {
		err = account.ValidateAndCacheProof(c, ctx, acct, rootHash)
		if err != nil {
			return fmt.Errorf("failed to validate and cache proof for %s: %w", acct, err)
		}
	}

	// PHASE 2: VALIDATE SIGNATURES FROM GENESIS TO CURRENT MAJOR BLOCK
	// 2.1 RetrieveGenesisBlockAndAuthority() (genesis.go)
	// Input: context, client
	// Output: block, keybook, keypage
	genesisBlock, keyBook, keyPage, err := RetrieveGenesisBlockAndAuthority(ctx, c.v2)
	if err != nil {
		return fmt.Errorf("failed to retrieve genesis block and authority: %w", err)
	}

	// 2.2 QueryMajorBlocks()
	// This step consists of extracting signatures and thresholds for each block
	// Input: context, client
	majorBlocks, err := c.QueryAllMajorBlocks(ctx)
	if err != nil {
		return fmt.Errorf("failed to query all major blocks: %w", err)
	}

	// 2.2.1 QueryMajorBlock()
	// Input: context, client, index
	// Output: AuthoritySet for 1 block (Sigs + Threshold)
	var AuthoritySets []signatures.AuthoritySet
	var sampleAuthoritySet *signatures.AuthoritySet
	if len(majorBlocks) > 0 {
		_, sampleSet, err := blocks.QueryMajorBlock(ctx, c.v2, 0)
		if err != nil {
			return fmt.Errorf("failed to extract AuthoritySet for block 0: %w", err)
		}
		sampleAuthoritySet = sampleSet
	}

	// Output: AuthorityTracker for all blocks (map(index, AuthoritySet))
	authorityTracker, err := blocks.BuildAuthorityTracker(majorBlocks)
	if err != nil {
		return fmt.Errorf("failed to build authority tracker: %w", err)
	}

	// 2.3 Fetch Authorities
	// This step consists of determining what are the correct authorities
	// at each block (height, index, timestamp?)
	// Output: AuthorityTracker for the valid authorities of the major blockchain
	// 2.4 ValidateFromGenesisToCurrent()
	// This step consists of checking, index by index both Authority Trackers
	// to see if they match. If they do, that means we can trust information given
	// by the node?

	return nil
}
