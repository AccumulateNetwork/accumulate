package liteclient

import (
	"context"
	"testing"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

func TestRetrieveGenesisBlockAndAuthority(t *testing.T) {
	cl := &client.Client{}
	block, book, page, err := RetrieveGenesisBlockAndAuthority(context.Background(), cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block == nil {
		t.Error("expected non-nil block result")
	}
	if book == nil {
		t.Error("expected non-nil key book result")
	}
	if page == nil {
		t.Error("expected non-nil key page result")
	}
}
