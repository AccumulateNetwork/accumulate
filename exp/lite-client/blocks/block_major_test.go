package blocks

import (
	"context"
	"os"
	"testing"
	"time"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// TestQueryMajorBlocks_Kermit connects to the Kermit testnet and retrieves major blocks.
func TestQueryMajorBlocks_Kermit(t *testing.T) {
	// Optionally allow overriding the Kermit endpoint via env var
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

	recordRange, err := QueryMajorBlocks(ctx, cl, startIndex, count)
	if err != nil {
		t.Fatalf("QueryMajorBlocks failed: %v", err)
	}
	if recordRange == nil || len(recordRange) == 0 {
		t.Fatalf("No major blocks returned from Kermit testnet")
	}

	t.Logf("Retrieved %d major blocks from Kermit", len(recordRange))
	for i, block := range recordRange {
		if block == nil {
			t.Errorf("Block %d is nil", i)
			continue
		}
		idx, hasIndex := block["index"]
		if !hasIndex || idx == nil {
			t.Errorf("Block %d missing 'index' field", i)
		}
		timeVal, hasTime := block["time"]
		if !hasTime || timeVal == nil {
			t.Errorf("Block %d missing 'time' field", i)
		}
		t.Logf("Block %d: index=%v time=%v", i, block["index"], block["time"])
	}
}
