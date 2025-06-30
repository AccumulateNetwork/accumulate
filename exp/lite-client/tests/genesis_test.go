package liteclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func createTestClient(t *testing.T) *client.Client {
	cl, err := client.New("https://kermit.accumulatenetwork.io")
	require.NoError(t, err, "failed to create client")
	cl.Timeout = 20 * time.Second
	return cl
}

func TestRetrieveGenesisBlock(t *testing.T) {
	t.Log("[GENESIS TEST] Starting RetrieveGenesisBlock test...")

	cl := createTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	block, err := RetrieveGenesisBlock(ctx, cl)
	require.NoError(t, err, "should retrieve genesis block without error")
	require.NotNil(t, block, "genesis block should not be nil")

	// Check expected fields
	index, ok := block["mQueryMajorBlocksIndex"]
	require.True(t, ok, "missing mQueryMajorBlocksIndex")
	t.Logf("✔ Found mQueryMajorBlocksIndex: %v", index)

	timestamp, ok := block["mQueryMajorBlocksTime"]
	require.True(t, ok, "missing mQueryMajorBlocksTime")
	t.Logf("✔ Found mQueryMajorBlocksTime: %v", timestamp)

	minors, ok := block["minorBlocks"]
	require.True(t, ok, "missing minorBlocks field")
	require.IsType(t, []interface{}{}, minors, "minorBlocks should be a slice")
	t.Logf("✔ Found %d minor blocks", len(minors.([]interface{})))

	t.Log("[GENESIS TEST] RetrieveGenesisBlock test passed.")
}

func TestRetrieveGenesisAuthority(t *testing.T) {
	t.Log("[AUTHORITY TEST] Starting RetrieveGenesisAuthority test...")

	cl := createTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	book, page, err := RetrieveGenesisAuthority(ctx, cl)
	require.NoError(t, err, "should retrieve authority without error")
	require.NotNil(t, book, "key book should not be nil")
	require.NotNil(t, page, "key page should not be nil")

	// Validate book structure
	require.IsType(t, &protocol.KeyBook{}, book)
	require.NotNil(t, book.Url, "key book URL should not be nil")
	require.GreaterOrEqual(t, book.PageCount, uint64(1), "book should have at least one page")
	t.Logf("✔ KeyBook URL: %s", book.Url)

	// Validate page structure
	require.IsType(t, &protocol.KeyPage{}, page)
	require.NotNil(t, page.Url, "key page URL should not be nil")
	require.GreaterOrEqual(t, uint64(len(page.Keys)), uint64(1), "page should have at least one key")
	t.Logf("✔ KeyPage URL: %s", page.Url)

	t.Log("[AUTHORITY TEST] RetrieveGenesisAuthority test passed.")
}
