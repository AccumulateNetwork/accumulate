package blocks

import (
	"context"
	"os"
	"testing"
	"time"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// TestQueryMajorBlocks_Kermit connects to the Kermit testnet and retrieves major blocks using v3 (default).
func TestQueryMajorBlocks_Kermit(t *testing.T) {
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

	startIndex := uint64(0)
	count := uint64(2)
	partition := "acc://dn"

	recordRange, err := QueryMajorBlocks(ctx, cl, partition, startIndex, count, "v3")
	if err != nil {
		t.Fatalf("QueryMajorBlocks (v3) failed: %v", err)
	}
	if len(recordRange) == 0 {
		t.Fatalf("No major blocks returned from Kermit testnet (v3)")
	}

	t.Logf("[v3] Retrieved %d major blocks from Kermit", len(recordRange))
	for i, block := range recordRange {
		if block == nil {
			t.Errorf("[v3] Block %d is nil", i)
			continue
		}
		idx, hasIndex := block["index"]
		if !hasIndex || idx == nil {
			t.Errorf("[v3] Block %d missing 'index' field", i)
		}
		timeVal, hasTime := block["time"]
		if !hasTime || timeVal == nil {
			t.Errorf("[v3] Block %d missing 'time' field", i)
		}
		t.Logf("[v3] Block %d: index=%v time=%v", i, block["index"], block["time"])
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

	startIndex := uint64(0)
	count := uint64(2)
	partition := "acc://dn"

	recordRange, err := QueryMajorBlocks(ctx, cl, partition, startIndex, count, "v2")
	if err != nil {
		t.Fatalf("QueryMajorBlocks (v2) failed: %v", err)
	}
	if len(recordRange) == 0 {
		t.Fatalf("No major blocks returned from Kermit testnet (v2)")
	}

	t.Logf("[v2] Retrieved %d major blocks from Kermit", len(recordRange))
	for i, block := range recordRange {
		if block == nil {
			t.Errorf("[v2] Block %d is nil", i)
			continue
		}
		idx, hasIndex := block["majorBlockIndex"]
		if !hasIndex || idx == nil {
			t.Errorf("[v2] Block %d missing 'majorBlockIndex' field", i)
		}
		timeVal, hasTime := block["majorBlockTime"]
		if !hasTime || timeVal == nil {
			t.Errorf("[v2] Block %d missing 'majorBlockTime' field", i)
		}
		t.Logf("[v2] Block %d: majorBlockIndex=%v majorBlockTime=%v", i, block["majorBlockIndex"], block["majorBlockTime"])
	}
}
