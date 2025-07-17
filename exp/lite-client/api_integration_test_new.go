package liteclient

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

const (
	KermitTestAccountURL     = "acc://c7b2d77d5beadeb7774ca04106f2f68a9317b75c2f96efee/ACME"
	KermitTestAccountBalance = "10.00000000"
	KermitTestAccountToken   = "acc://ACME"
)

func getKermitClient(t *testing.T) *LiteClient {
	kermit := os.Getenv("KERMIT_API")
	if kermit == "" {
		kermit = "https://kermit.accumulatenetwork.io"
	}
	client, err := NewLiteClient(kermit)
	if err != nil {
		t.Fatalf("Failed to create Kermit client: %v", err)
	}
	return client
}

func TestKermit_GetBalance(t *testing.T) {
	client := getKermitClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bal, err := client.GetBalance(ctx, KermitTestAccountURL)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if bal.Balance == "" {
		t.Fatal("Balance is empty")
	}
	if bal.Token == "" {
		t.Fatal("Token is empty")
	}
	if bal.Height <= 0 {
		t.Fatalf("Invalid height: %d", bal.Height)
	}

	// Validate balance is numeric
	if _, err := strconv.ParseFloat(bal.Balance, 64); err != nil {
		t.Fatalf("Balance is not numeric: %s", bal.Balance)
	}

	t.Logf("Balance: %s %s (height %d)", bal.Balance, bal.Token, bal.Height)
}

func TestKermit_GetTransactions(t *testing.T) {
	client := getKermitClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	txs, err := client.GetTransactions(ctx, KermitTestAccountURL, 5)
	if err != nil {
		t.Fatalf("GetTransactions failed: %v", err)
	}

	t.Logf("Found %d transactions", len(txs))

	for i, tx := range txs {
		if tx.TxID == "" {
			t.Fatalf("Transaction %d has empty TxID", i)
		}
		if tx.Type == "" {
			t.Fatalf("Transaction %d has empty Type", i)
		}
		if tx.Status == "" {
			t.Fatalf("Transaction %d has empty Status", i)
		}
		if tx.Timestamp <= 0 {
			t.Fatalf("Transaction %d has invalid timestamp: %d", i, tx.Timestamp)
		}
	}
}

func TestKermit_GetBalanceAndTransactions(t *testing.T) {
	client := getKermitClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bal, txs, err := client.GetBalanceAndTransactions(ctx, KermitTestAccountURL, 5)
	if err != nil {
		t.Fatalf("GetBalanceAndTransactions failed: %v", err)
	}

	if bal.Balance == "" {
		t.Fatal("Balance is empty")
	}
	if bal.Token == "" {
		t.Fatal("Token is empty")
	}

	t.Logf("Balance: %s %s, Transactions: %d", bal.Balance, bal.Token, len(txs))

	// Test consistency with separate GetBalance call
	bal2, err := client.GetBalance(ctx, KermitTestAccountURL)
	if err != nil {
		t.Fatalf("GetBalance consistency check failed: %v", err)
	}
	if bal.Balance != bal2.Balance {
		t.Fatalf("Balance inconsistency: combined=%s, separate=%s", bal.Balance, bal2.Balance)
	}
}

func TestKermit_V3ApiConnection(t *testing.T) {
	client := getKermitClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, err := accurl.Parse(KermitTestAccountURL)
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}

	q := api.Querier2{Querier: client.v3}
	resp, err := q.QueryAccount(ctx, u, nil)
	if err != nil {
		t.Fatalf("v3 API call failed: %v", err)
	}

	if resp == nil {
		t.Fatal("v3 API returned nil response")
	}
	if resp.Account == nil {
		t.Fatal("v3 API response has nil Account")
	}

	t.Logf("v3 API connection successful")
}
