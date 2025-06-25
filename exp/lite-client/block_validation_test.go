package liteclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestQueryMajorBlocksAndSequence(t *testing.T) {
	cl := createTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	blocks, err := QueryMajorBlocks(ctx, cl, 0, 5)
	require.NoError(t, err, "error fetching major blocks")
	require.NotNil(t, blocks)
	require.GreaterOrEqual(t, len(blocks), 2, "should fetch at least 2 blocks to test sequence")

	// Convert to []*client.MajorQueryResponse for validateBlockSequence
	var blockObjs []*client.MajorQueryResponse
	for _, b := range blocks {
		blockObjs = append(blockObjs, &client.MajorQueryResponse{
			MajorBlockIndex: toUint64(t, b["majorBlockIndex"]),
			MajorBlockTime:  toTimePtr(t, b["majorBlockTime"]),
		})
	}
	// Use any known valid partition just for display (not needed for globals in fix)
	partitionUrl, _ := accurl.Parse("acc://bvn0.acme")
	require.NoError(t, validateBlockSequence(ctx, *cl, blockObjs, partitionUrl))
}

func TestQueryNetworkGlobals(t *testing.T) {
	cl := createTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use global constant URL instead of partition path
	globalsUrl, err := accurl.Parse("acc://NetworkGlobals")
	require.NoError(t, err)
	globals, err := QueryNetworkGlobals(ctx, *cl, globalsUrl)
	require.NoError(t, err)
	require.NotNil(t, globals)
	require.NotEmpty(t, globals.MajorBlockSchedule)
}

type dummyKeySig struct {
	protocol.KeySignature
}

func (d dummyKeySig) GetSignature() []byte {
	return []byte("invalid-signature")
}

func TestTrackAuthorityChanges_Stub(t *testing.T) {
	cl := createTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := TrackAuthorityChanges(ctx, *cl, []byte("dummyTxId"))
	require.Error(t, err)
}

func toUint64(t *testing.T, v interface{}) uint64 {
	switch x := v.(type) {
	case float64:
		return uint64(x)
	case int:
		return uint64(x)
	case uint64:
		return x
	default:
		t.Fatalf("unexpected type for uint64: %T", v)
		return 0
	}
}

func toTimePtr(t *testing.T, v interface{}) *time.Time {
	str, ok := v.(string)
	require.True(t, ok, "majorBlockTime should be string")
	tm, err := time.Parse(time.RFC3339, str)
	require.NoError(t, err)
	return &tm
}
