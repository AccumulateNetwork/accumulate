package liteclient

import (
	"context"
	"os"
	"testing"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

func TestBuildAuthoritySets(t *testing.T) {
	kermitUrl := os.Getenv("KERMIT_API")
	if kermitUrl == "" {
		kermitUrl = "https://kermit.accumulatenetwork.io"
	}

	cl, err := client.New(kermitUrl)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	liteClient := &LiteClient{
		v2:    cl,
		cache: make(map[string]VerifiedAccount),
	}

	authSets, err := buildAuthoritySets(context.Background(), liteClient)
	if err != nil {
		t.Errorf("buildAuthoritySets returned error: %v", err)
	}
	if len(authSets) == 0 {
		t.Errorf("buildAuthoritySets returned no authority sets")
	} else {
		t.Logf("buildAuthoritySets returned %d authority sets", len(authSets))
	}
}

func TestRetrieveAndValidateProof(t *testing.T) {
	// Use the testnet endpoint; change to "mainnet", "local", or a custom URL as needed
	kermitUrl := os.Getenv("KERMIT_API")
	if kermitUrl == "" {
		kermitUrl = "https://kermit.accumulatenetwork.io"
	}

	cl, err := client.New(kermitUrl)
	if err != nil {
		t.Fatalf("Failed to create client: %v %v", err, cl)
	}

	if err != nil {
		t.Fatalf("Failed to initialize LiteClient: %v", err)
	}
	liteClient := &LiteClient{
		v2:    cl,
		cache: make(map[string]VerifiedAccount),
	}
	accounts := []string{"example-account"}
	err = RetrieveAndValidateProof(context.Background(), accounts, liteClient)
	if err != nil {
		t.Logf("RetrieveAndValidateProof returned error: %v", err)
	} else {
		t.Logf("RetrieveAndValidateProof succeeded")
	}
}
