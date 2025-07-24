package liteclient

import (
	"context"
	"fmt"
	"testing"
	"time"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestAccountsWithReceipts(t *testing.T) {
	// Test different account URLs to find ones that return receipts
	// These accounts are from various parts of the codebase
	testAccounts := []string{
		// Known working test account (from account_data_test.go)
		"acc://c7b2d77d5beadeb7774ca04106f2f68a9317b75c2f96efee/ACME",

		// Factoid-derived accounts (from factoid_test.go)
		"acc://08115f96ebb5e35a9c806de9cffe4c99455a0c5a60942d53/ACME",
		"acc://e4571e13d3af400ad41a7e70134387d0f9b0bd5a94f4347f/ACME",
		"acc://3752fc879ff3538e4e436512191aec2b61f8a9374c38f723/ACME",
		"acc://9fe752486d3f03a607b465c0766947f86a8242de54e0c0c4/ACME",
		"acc://fa639612f2567f4ceb92a516c3dacc715d349575dbdc81d8/ACME",

		// Test database accounts (from database-v1.0.0.json)
		"acc://7117c50f04f1254d56b704dc05298912deeb25dbc1d26ef6/ACME",
		"acc://alice",
		"acc://alice/book",
		"acc://alice/book/1",

		// Launch.json accounts (from .vscode/launch.json)
		"acc://7a6f9db5789710a6b27e0c5965e337d8fc7431075290434d/ACME",
		"acc://20021b633ee9b168259af8fd6a903022724f09466ec1684e/ACME",

		// System accounts
		"acc://dn.acme",
		"acc://dn.acme/tokens",
		"acc://dn.acme/acme",

		// Directory accounts
		"acc://accumulate.acme/ACME",
		"acc://accumulate.acme/tokens",
	}

	apis := []string{
		"https://testnet.accumulatenetwork.io",
		"https://mainnet.accumulatenetwork.io",
		"https://kermit.accumulatenetwork.io",
	}

	for _, apiURL := range apis {
		fmt.Printf("\n=== Testing API: %s ===\n", apiURL)

		// Create client
		c, err := client.New(apiURL)
		if err != nil {
			t.Logf("Failed to create client for %s: %v", apiURL, err)
			continue
		}

		for _, account := range testAccounts {
			fmt.Printf("Testing account: %s\n", account)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

			// Parse account URL
			u, err := url.Parse(account)
			if err != nil {
				t.Logf("  ❌ Invalid URL: %v", err)
				cancel()
				continue
			}

			req := &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: u}}
			var resp client.ChainQueryResponse

			err = c.RequestAPIv2(ctx, "query", req, &resp)
			if err != nil {
				t.Logf("  ❌ Query failed: %v", err)
				cancel()
				continue
			}

			if resp.Receipt == nil {
				t.Logf("  ⚠️  No receipt returned")
			} else {
				t.Logf("  ✅ Receipt found! MajorBlock: %d", resp.Receipt.MajorBlock)
				t.Logf("     Account: %s", account)
				t.Logf("     API: %s", apiURL)
				// This account works - we can use it in tests
			}

			cancel()
			time.Sleep(100 * time.Millisecond) // Be nice to the API
		}
	}
}
