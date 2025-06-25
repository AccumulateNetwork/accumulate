package liteclient

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// =====================
// === Core Methods ====
// =====================

// QueryMajorBlocks retrieves a paginated slice of major blocks from the given partition.
func QueryMajorBlocks(ctx context.Context, cl *client.Client, startIndex uint64, count uint64) ([]map[string]interface{}, error) {
	partitionUrl, err := parseUrl("acc://bvn0.acme")
	if err != nil {
		return nil, fmt.Errorf("failed to parse partition URL: %v", err)
	}

	query := createQueryMajorBlock(startIndex, count, partitionUrl)
	fmt.Printf("Querying for major blocks starting at %d (count: %d)...\n", startIndex, count)

	resp, err := executeQueryMajorBlock(ctx, cl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query major blocks: %v", err)
	}

	blocks, err := processMajorBlock(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to process major blocks: %v", err)
	}

	fmt.Printf("Retrieved %d major blocks\n", len(blocks))
	return blocks, nil
}

// Get a KeyBook given its URL.
func GetKeyBook(ctx context.Context, cl *client.Client, urlStr string) (*protocol.KeyBook, error) {
	var book protocol.KeyBook
	if err := queryAccountAs(ctx, cl, urlStr, &book); err != nil {
		return nil, fmt.Errorf("failed to query key book: %v", err)
	}
	return &book, nil
}

// Get a KeyPage from a KeyBook and page index.
func GetKeyPage(ctx context.Context, cl *client.Client, bookUrl string, pageIndex int) (*protocol.KeyPage, error) {
	pageUrl := fmt.Sprintf("%s/page/%d", bookUrl, pageIndex)
	var page protocol.KeyPage
	if err := queryAccountAs(ctx, cl, pageUrl, &page); err != nil {
		return nil, fmt.Errorf("failed to query key page: %v", err)
	}
	return &page, nil
}

// Validate all minor blocks from the past 24h and their signatures.
func ValidateRecentMinorBlocks(ctx context.Context, cl *client.Client, authorities *AuthoritySet) error {
	oneDayAgo := time.Now().Add(-24 * time.Hour)

	startBlock, err := findMajorBlockByTime(ctx, cl, oneDayAgo)
	if err != nil {
		return fmt.Errorf("failed to find starting block: %v", err)
	}

	query := &client.MajorBlocksQuery{
		QueryPagination: client.QueryPagination{
			Start: uint64(startBlock),
		},
	}
	query.Url, err = parseUrl("acc://dn.acme")
	if err != nil {
		return fmt.Errorf("failed to parse DN URL: %v", err)
	}

	resp, err := cl.QueryMajorBlocks(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query major blocks: %v", err)
	}
	if resp == nil || len(resp.Items) == 0 {
		return fmt.Errorf("no major blocks found")
	}

	for _, item := range resp.Items {
		majorBlock := convertToMajorBlockRecord(item)

		valid, err := validateMajorBlockSignatures(ctx, cl, int(majorBlock.Index), authorities)
		if err != nil {
			return fmt.Errorf("failed to validate signatures for major block %d: %v", majorBlock.Index, err)
		}
		if !valid {
			return fmt.Errorf("invalid signatures for major block %d", majorBlock.Index)
		}

		minorBlocks, err := getMinorBlocksForMajorBlock(ctx, cl, majorBlock)
		if err != nil {
			return fmt.Errorf("failed to get minor blocks for major block %d: %v", majorBlock.Index, err)
		}
		for _, minorBlock := range minorBlocks {
			valid, err := validateMinorBlockSignatures(ctx, cl, minorBlock, authorities)
			if err != nil {
				return fmt.Errorf("failed to validate signatures for minor block %d: %v", minorBlock.Index, err)
			}
			if !valid {
				return fmt.Errorf("invalid signatures for minor block %d", minorBlock.Index)
			}
			if !verifyMinorBlockInMajor(minorBlock, majorBlock) {
				return fmt.Errorf("minor block %d not correctly referenced in major block %d", minorBlock.Index, majorBlock.Index)
			}
		}
	}
	return nil
}

// =========================
// === Internal Helpers ====
// =========================

func parseUrl(urlStr string) (*accurl.URL, error) {
	return accurl.Parse(urlStr)
}

func createQueryMajorBlock(startIndex uint64, count uint64, partitionUrl *accurl.URL) *client.MajorBlocksQuery {
	return &client.MajorBlocksQuery{
		UrlQuery: client.UrlQuery{Url: partitionUrl},
		QueryPagination: client.QueryPagination{
			Start: startIndex,
			Count: count,
		},
	}
}

func executeQueryMajorBlock(ctx context.Context, cl *client.Client, query *client.MajorBlocksQuery) (*api.MultiResponse, error) {
	resp, err := cl.QueryMajorBlocks(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query major blocks: %v", err)
	}
	if resp == nil || len(resp.Items) == 0 {
		return nil, fmt.Errorf("no major blocks found")
	}
	return resp, nil
}

func processMajorBlock(resp *api.MultiResponse) ([]map[string]interface{}, error) {
	var blocks []map[string]interface{}
	for i, item := range resp.Items {
		raw := make(map[string]interface{})
		bz, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal block %d: %v", i, err)
		}
		if err := json.Unmarshal(bz, &raw); err != nil {
			return nil, fmt.Errorf("failed to unmarshal block %d: %v", i, err)
		}
		if _, ok := raw["majorBlockIndex"]; !ok {
			return nil, fmt.Errorf("block %d missing majorBlockIndex field", i)
		}
		fmt.Printf("Retrieved major block %v\n", raw["majorBlockIndex"])
		blocks = append(blocks, raw)
	}
	return blocks, nil
}

func queryAccountAs(ctx context.Context, cl *client.Client, urlStr string, out interface{}) error {
	parsedUrl, err := parseUrl(urlStr)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %v", err)
	}
	query := &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: parsedUrl}}
	_, err = cl.QueryAccountAs(ctx, query, out)
	if err != nil {
		return fmt.Errorf("failed to query account %s: %v", urlStr, err)
	}
	return nil
}

func getFirstAuthorityUrl(auth *protocol.AccountAuth) (string, error) {
	if len(auth.Authorities) == 0 {
		return "", fmt.Errorf("no authorities found")
	}
	return auth.Authorities[0].Url.String(), nil
}

// ==============================
// === TODO: Implementations ====
// ==============================

type MajorBlockRecord struct {
	Index         uint64              // The block number/height
	Time          *time.Time          // When the block was created
	MinorBlocks   []*MinorBlockRecord // Minor blocks contained in this major block
	LastBlockTime *time.Time          // Timestamp of the last block
	// TODO: Add more fields if needed
}

type MinorBlockRecord struct {
	Index  uint64      // The block number
	Time   *time.Time  // When the block was created
	Source *accurl.URL // URL of the partition that produced this block
	// TODO: Add Entries, Anchored, LastBlockTime, etc. as needed
}

// getGenesisAuthorities retrieves the genesis authority set
func getGenesisAuthorities(ctx context.Context, cl *client.Client) (*AuthoritySet, error) {
	// TODO: Implement retrieval of genesis authority set
	return &AuthoritySet{}, nil
}

// trackAuthorityChanges tracks authority set changes from genesis to present
func trackAuthorityChanges(ctx context.Context, cl *client.Client) ([]*AuthoritySet, error) {
	// TODO: Implement logic to track authority set changes
	return []*AuthoritySet{}, nil
}

// validateAuthorityChange validates authority change transactions and signatures
func validateAuthorityChange(ctx context.Context, cl *client.Client, prevSet *AuthoritySet, changeTx interface{}) (bool, error) {
	// TODO: Implement validation of authority change signatures (2/3 threshold, etc.)
	return true, nil
}

func findMajorBlockByTime(ctx context.Context, cl *client.Client, t time.Time) (int, error) {
	// TODO: Implement logic to find the major block index closest to a timestamp
	return 0, nil
}

func convertToMajorBlockRecord(item interface{}) MajorBlockRecord {
	// TODO: Convert item to strongly typed struct
	return MajorBlockRecord{}
}

// validateAuthorityChanges validates authority set changes from genesis to present
func validateAuthorityChanges(ctx context.Context, cl *client.Client) error {
	// TODO: Implement authority change validation loop
	// 1. Start with genesis
	// 2. Query major blocks in batches
	// 3. For each block, check for authority change txs and validate signatures
	return nil
}

func validateMajorBlockSignatures(ctx context.Context, cl *client.Client, majorBlockIndex int, authorities *AuthoritySet) (bool, error) {
	// TODO: Implement signature validation for major blocks
	return true, nil
}

func getMinorBlocksForMajorBlock(ctx context.Context, cl *client.Client, majorBlock MajorBlockRecord) ([]MinorBlockRecord, error) {
	// TODO: Fetch minor blocks for a given major block
	return nil, nil
}

func validateMinorBlockSignatures(ctx context.Context, cl *client.Client, minorBlock MinorBlockRecord, authorities *AuthoritySet) (bool, error) {
	// TODO: Validate minor block signatures
	return true, nil
}

func verifyMinorBlockInMajor(minorBlock MinorBlockRecord, majorBlock MajorBlockRecord) bool {
	// TODO: Check reference consistency
	return true
}
