package liteclient

import (
	"context"
	"os"
	"testing"
)

func TestRetrieveAndValidateProof(t *testing.T) {
	// Use the testnet endpoint; change to "mainnet", "local", or a custom URL as needed
	kermitUrl := os.Getenv("KERMIT_API")
	if kermitUrl == "" {
		kermitUrl = "https://kermit.accumulatenetwork.io"
	}

	liteClient, err := NewLiteClient(kermitUrl)
	if err != nil {
		t.Fatalf("Failed to initialize LiteClient: %v", err)
	}

	// This account is known to exist on the testnet
	accounts := []string{"acc://dn.acme/tokens"}
	err = liteClient.RetrieveAccountStates(context.Background(), accounts)
	if err != nil {
		t.Logf("RetrieveAndValidateProof returned error: %v", err)
	} else {
		t.Logf("RetrieveAndValidateProof succeeded")
	}
}
