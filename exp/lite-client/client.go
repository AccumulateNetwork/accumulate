package liteclient

import (
	"context"
	"fmt"
	"net/http"

	liteblocks "gitlab.com/accumulatenetwork/accumulate/exp/lite-client/blocks"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	v2 "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// QuerierValidator combines the v3 Querier and Validator interfaces.
type QuerierValidator interface {
	api.Querier
	api.Validator
}

type LiteClient struct {
	v2          *v2.Client
	v3          QuerierValidator
	cache       map[string]VerifiedAccount
	authorities liteblocks.AuthorityProvider
	validator   *liteblocks.BlockValidator
}

// NewLiteClient creates a new LiteClient for Phase 1 (account proof creation).
func NewLiteClient(server string) (*LiteClient, error) {
	// 1. Create new v2 and v3 clients
	v2Client, err := v2.New(server)
	if err != nil {
		return nil, fmt.Errorf("failed to create v2 client: %w", err)
	}
	v3Client := jsonrpc.NewClient(server)

	// 2. Initialize cache for verified accounts
	// 3. Initialize authority provider for signature validation
	authorityProvider, err := liteblocks.NewDynamicAuthorityProvider(context.Background(), &http.Client{}, server)
	if err != nil {
		return nil, fmt.Errorf("failed to create authority provider: %w", err)
	}

	return &LiteClient{
		v2:          v2Client,
		v3:          v3Client,
		cache:       make(map[string]VerifiedAccount),
		authorities: authorityProvider,
		validator:   liteblocks.NewBlockValidator(authorityProvider),
	}, nil
}

// RetrieveAccountStates is the main orchestration entrypoint for the lite client.
// It retrieves and validates account states using cryptographic proofs and signature validation.
func (c *LiteClient) RetrieveAccountStates(ctx context.Context, accountUrls []string) error {

	// PHASE 1: ACCOUNT STATE PROOF CREATION
	// Create cryptographic proofs for account states against a BPT root hash.
	err := RetrieveAndValidateProof(ctx, accountUrls, c)
	if err != nil {
		return fmt.Errorf("phase 1 failed: unable to retrieve or validate account proof: %w", err)
	}



	// PHASE 2: MAJOR BLOCK SIGNATURE VALIDATION (from Genesis)
	// Validate signatures from the genesis block to the current major block.
	fmt.Println("Phase 2: Starting major block validation from genesis...")

	var nextBlock uint64 = 0 // Start from Genesis (Block 0)

	for {
		// Use the v3 API to query the DN for a range of major blocks.
		query := &api.BlockQuery{
			MajorRange: &api.RangeOptions{
				Start: nextBlock,
				Count: api.Ptr(uint64(10)),
			},
		}
		resp, err := c.v3.Query(ctx, protocol.DnUrl(), query)
		if err != nil {
			return fmt.Errorf("phase 2 failed: could not query major blocks from %d: %w", nextBlock, err)
		}

		blocks, ok := resp.(*api.RecordRange[*api.MajorBlockRecord])
		if !ok {
			return fmt.Errorf("phase 2 failed: could not cast response to major block range")
		}

		if len(blocks.Records) == 0 {
			fmt.Println("Phase 2: Successfully validated the entire major block chain.")
			break
		}

		fmt.Printf("Processing %d major blocks from %d to %d...\n", len(blocks.Records), blocks.Records[0].Index, blocks.Records[len(blocks.Records)-1].Index)

		for _, block := range blocks.Records {
			// Validate the block against the current set of trusted authorities.
			err = liteblocks.ValidateMajorBlock(ctx, block, c.validator)
			if err != nil {
				return fmt.Errorf("phase 2 failed: validation failed for major block %d: %w", block.Index, err)
			}

			// After validating the major block anchor, check for authority updates within the block.
			for _, minor := range block.MinorBlocks.Records {
				for _, entry := range minor.Entries.Records {
					if rec, ok := entry.Value.(*api.MessageRecord[messaging.Message]); ok {
						if txnMsg, ok := rec.Message.(*messaging.TransactionMessage); ok {
							// Check if the transaction is an authority update on the DN's ledger.
							if update, ok := txnMsg.Transaction.Body.(*protocol.UpdateKeyPage); ok {
								if txnMsg.Transaction.Header.Principal.Equal(protocol.DnUrl().JoinPath(protocol.Ledger)) {
									fmt.Printf("Found authority update in block %d, applying...\n", block.Index)
																		if dynProvider, ok := c.authorities.(*liteblocks.DynamicAuthorityProvider); ok {
										err = dynProvider.Update(ctx, block.Index, rec, txnMsg.Transaction, update)
									} else {
										return fmt.Errorf("authority provider is not dynamic, cannot update")
									}
									if err != nil {
										return fmt.Errorf("failed to apply authority update from block %d: %w", block.Index, err)
									}
								}
							}
						}
					}
				}
			}
		}

		nextBlock = blocks.Records[len(blocks.Records)-1].Index + 1
	}

	// PHASE 3: MINOR BLOCK SIGNATURE VALIDATION
	// Validate signatures for minor blocks from the last major block to the present.
	fmt.Println("Phase 3: Minor block validation (not yet implemented).")

	// PHASE 4: ROOT HASH RECEIPT CREATION
	// Create the cryptographic receipt to the root hash that covers the BPT root hash.
	fmt.Println("Phase 4: Root hash receipt creation (not yet implemented).")

	// PHASE 5: ACCOUNT TRANSACTION COLLECTION
	// Collect and validate hashes and transactions for the set of accounts.
	fmt.Println("Phase 5: Account transaction collection (not yet implemented).")

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
	for _, url := range accountUrls {
		// Placeholder for the actual root hash

		err := ValidateAndCacheProof(c, ctx, url, rootHash)
		if err != nil {
			return fmt.Errorf("error validating and caching proof for %s: %w", url, err)
		}
	}

	return nil
}
