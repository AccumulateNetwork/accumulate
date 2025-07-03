package liteclient

import (
	"context"
	"os"
	"strings"
	"testing"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// TestFetchBPTRootHash verifies fetching the BPT root hash from a node status endpoint.
// This test requires access to a local node or a Kermit API that exposes the status endpoint.
// On public endpoints (e.g., https://kermit.accumulatenetwork.io), the status endpoint is disabled
// and the test will be skipped. See docs/authority_validation.md for partition mapping details.
func TestFetchBPTRootHash(t *testing.T) {
	t.Parallel()

	kermitUrl := os.Getenv("KERMIT_API")
	if kermitUrl == "" {
		kermitUrl = "https://kermit.accumulatenetwork.io"
	}

	cl, err := client.New(kermitUrl)

	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	ctx := context.Background()

	rootHash, err := FetchBPTRootHash(ctx, cl, "dn")
	if err != nil {
		if strings.Contains(err.Error(), "node ID is missing") ||
			strings.Contains(err.Error(), "method not found") ||
			strings.Contains(err.Error(), "not implemented") {
			t.Skip("Status endpoint not available on public Kermit API; run against a local node for full test")
		}
		t.Fatalf("failed to fetch DN BPT root hash: %v", err)
	}
	if len(rootHash) == 0 {
		t.Fatal("DN BPT root hash is empty")
	}
	t.Logf("DN BPT root hash: %x", rootHash)

	rootHash, err = FetchBPTRootHash(ctx, cl, "bvn0.acme")
	if err != nil {
		t.Fatalf("failed to fetch BVN BPT root hash: %v", err)
	}
	if len(rootHash) == 0 {
		t.Fatal("BVN BPT root hash is empty")
	}
	t.Logf("BVN BPT root hash: %x", rootHash)
}
