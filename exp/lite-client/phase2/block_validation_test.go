package liteclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const testEndpoint = "https://kermit.accumulatenetwork.io" // or your preferred testnet

func createTestClient(t *testing.T) *client.Client {
	cl, err := client.New(testEndpoint)
	require.NoError(t, err, "failed to create client")
	cl.Timeout = 20 * time.Second
	return cl
}

func TestRetrieveGenesisBlock(t *testing.T) {
	cl := createTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	block, err := RetrieveGenesisBlock(ctx, cl)
	require.NoError(t, err, "error retrieving genesis block")
	require.NotNil(t, block, "block should not be nil")
	require.NotEmpty(t, block["majorBlockIndex"], "block index should not be empty")
	require.NotEmpty(t, block["majorBlockTime"], "block time should not be empty")
	require.NotEmpty(t, block["minorBlocks"], "minor blocks should not be empty")
}

func TestRetrieveGenesisAuthority(t *testing.T) {
	cl := createTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	book, page, err := RetrieveGenesisAuthority(ctx, cl)
	require.NoError(t, err, "error retrieving genesis authority")
	require.NotNil(t, book, "key book should not be nil")
	require.NotNil(t, page, "key page should not be nil")
	require.IsType(t, &protocol.KeyBook{}, book)
	require.IsType(t, &protocol.KeyPage{}, page)

	require.NotNil(t, book.Url, "key book should have a URL")
	require.GreaterOrEqual(t, book.PageCount, uint64(1), "key book should have at least one page")
	require.NotNil(t, page.Url, "key page should have a URL")
}

func TestCheckPartitionExists(t *testing.T) {
	cl := createTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := &client.GeneralQuery{UrlQuery: client.UrlQuery{
		Url: mustParseUrl(t, "acc://bvn0/partition"),
	}}

	var raw map[string]any
	_, err := cl.QueryAccountAs(ctx, query, &raw)
	require.NoError(t, err, "partition account should exist")
	t.Logf("Partition raw: %+v", raw)
}

func mustParseUrl(t *testing.T, s string) *url.URL {
	u, err := url.Parse(s)
	require.NoError(t, err)
	return u
}
