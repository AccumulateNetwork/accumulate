package blocks

import (
	"context"
	"os"
	"testing"
	"time"

	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// TestQueryMajorBlocksV3_Kermit connects to the Kermit testnet and retrieves major blocks using v3 API.
func TestQueryMajorBlocksV3_Kermit(t *testing.T) {
	kermitUrl := os.Getenv("KERMIT_API")
	if kermitUrl == "" {
		kermitUrl = "https://kermit.accumulatenetwork.io"
	}

	v3Url := strings.TrimSuffix(kermitUrl, "/") + "/v3"
	querier := jsonrpc.NewClient(v3Url)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	startIndex := uint64(1) // Start from block 1, as 0 is genesis
	count := uint64(2)
	partition := "acc://dn"

	records, err := QueryMajorBlocksV3(ctx, querier, partition, startIndex, count)
	if err != nil {
		t.Fatalf("QueryMajorBlocksV3 failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("No major blocks returned from Kermit testnet (v3)")
	}

	t.Logf("[v3] Retrieved %d major blocks from Kermit", len(records))
	for i, block := range records {
		if block == nil {
			t.Errorf("[v3] Block %d is nil", i)
			continue
		}
		if block.Index == 0 {
			t.Errorf("[v3] Block %d missing or zero 'Index' field", i)
		}
		if block.Time.IsZero() {
			t.Errorf("[v3] Block %d missing 'Time' field", i)
		}
		t.Logf("[v3] Block %d: Index=%v Time=%v", i, block.Index, block.Time)
	}
}

// TestQueryMajorBlocksV2_Kermit connects to the Kermit testnet and retrieves major blocks using v2 API.
func TestQueryMajorBlocksV2_Kermit(t *testing.T) {
	kermitUrl := os.Getenv("KERMIT_API")
	if kermitUrl == "" {
		kermitUrl = "https://kermit.accumulatenetwork.io"
	}

	cl, err := client.New(kermitUrl)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	startIndex := uint64(1) // Start from block 1, as 0 is genesis
	count := uint64(2)
	partition := "acc://dn"

	records, err := QueryMajorBlocksV2(ctx, cl, partition, startIndex, count)
	if err != nil {
		t.Fatalf("QueryMajorBlocks (v2) failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("No major blocks returned from Kermit testnet (v2)")
	}

	t.Logf("[v2] Retrieved %d major blocks from Kermit", len(records))
	for i, block := range records {
		if block == nil {
			t.Errorf("[v2] Block %d is nil", i)
			continue
		}
		if block.MajorBlockIndex == 0 {
			t.Errorf("[v2] Block %d missing or zero 'MajorBlockIndex' field", i)
		}
		if block.MajorBlockTime == nil {
			t.Errorf("[v2] Block %d missing 'MajorBlockTime' field", i)
		}
		t.Logf("[v2] Block %d: majorBlockIndex=%v majorBlockTime=%v", i, block.MajorBlockIndex, block.MajorBlockTime)
	}
}
