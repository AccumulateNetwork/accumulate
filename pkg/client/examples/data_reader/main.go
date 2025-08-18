// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// data_reader is a tool to read and display data from Accumulate data accounts
// with support for different data formats and entry navigation.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	var (
		network = flag.String("network", "mainnet", "Network to connect to (mainnet, testnet, local)")
		url     = flag.String("url", "", "Data account URL to read")
		index   = flag.Int("index", -1, "Specific entry index to read (-1 for all)")
		format  = flag.String("format", "auto", "Output format: auto, hex, text, json")
		latest  = flag.Bool("latest", false, "Show only the latest entry")
		limit   = flag.Int("limit", 10, "Maximum entries to display")
	)
	flag.Parse()

	if *url == "" {
		log.Fatal("Please provide a data account URL with -url flag")
	}

	// Create client
	c, err := createClient(*network)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	reader := &DataReader{
		client: c,
		format: *format,
		limit:  *limit,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if *latest {
		// Show only latest entry
		if err := reader.ReadLatest(ctx, *url); err != nil {
			log.Fatalf("Failed to read latest entry: %v", err)
		}
	} else if *index >= 0 {
		// Read specific entry
		if err := reader.ReadEntry(ctx, *url, *index); err != nil {
			log.Fatalf("Failed to read entry %d: %v", *index, err)
		}
	} else {
		// Read all entries (up to limit)
		if err := reader.ReadAll(ctx, *url); err != nil {
			log.Fatalf("Failed to read data account: %v", err)
		}
	}
}

func createClient(network string) (*client.Client, error) {
	switch network {
	case "mainnet":
		return client.NewMainnet()
	case "testnet":
		return client.NewTestnet()
	case "local":
		endpoint := os.Getenv("ACCUMULATE_ENDPOINT")
		if endpoint == "" {
			endpoint = "http://localhost:8080/v3"
		}
		return client.NewLocal(endpoint)
	default:
		return nil, fmt.Errorf("unknown network: %s", network)
	}
}

type DataReader struct {
	client *client.Client
	format string
	limit  int
}

func (r *DataReader) ReadAll(ctx context.Context, accountURL string) error {
	// Get account info first
	account, err := r.client.GetAccount(ctx, accountURL)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	if account == nil || account.Account == nil {
		return fmt.Errorf("account not found")
	}

	// Check if it's a data account
	switch acc := account.Account.(type) {
	case *protocol.DataAccount:
		fmt.Printf("📄 Data Account: %s\n", accountURL)
		fmt.Printf("Type: Full Data Account\n")
		if acc.Entry != nil {
			fmt.Printf("Entry Type: %s\n", acc.Entry.Type())
		}
	case *protocol.LiteDataAccount:
		fmt.Printf("📄 Lite Data Account: %s\n", accountURL)
		fmt.Printf("Type: Lite Data Account\n")
	default:
		return fmt.Errorf("not a data account (type: %s)", account.Account.Type())
	}

	if account.LastBlockTime != nil {
		fmt.Printf("Last Update: %s\n", account.LastBlockTime.Format(time.RFC3339))
	}
	fmt.Println(strings.Repeat("-", 80))

	// For data accounts, the current entry is in the account itself
	if acc, ok := account.Account.(*protocol.DataAccount); ok && acc.Entry != nil {
		r.displayDataEntry(0, acc.Entry)
		fmt.Println()
	} else {
		fmt.Println("No data entries found")
	}


	return nil
}

func (r *DataReader) ReadEntry(ctx context.Context, accountURL string, index int) error {
	fmt.Printf("📄 Reading entry %d from: %s\n", index, accountURL)
	fmt.Println(strings.Repeat("-", 80))

	// Note: Currently the API returns the account with data, not individual entries
	// This is a limitation of the current API
	account, err := r.client.GetAccount(ctx, accountURL)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	if account == nil || account.Account == nil {
		return fmt.Errorf("account not found")
	}

	switch acc := account.Account.(type) {
	case *protocol.DataAccount:
		if acc.Entry != nil {
			r.displayDataEntry(index, acc.Entry)
		} else {
			fmt.Println("No data entries found")
		}
	case *protocol.LiteDataAccount:
		fmt.Println("Lite data accounts don't have indexed entries")
	default:
		return fmt.Errorf("not a data account (type: %s)", account.Account.Type())
	}

	return nil
}

func (r *DataReader) ReadLatest(ctx context.Context, accountURL string) error {
	// Get account to find the latest entry
	account, err := r.client.GetAccount(ctx, accountURL)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	if account == nil || account.Account == nil {
		return fmt.Errorf("account not found")
	}

	switch acc := account.Account.(type) {
	case *protocol.DataAccount:
		if acc.Entry == nil {
			return fmt.Errorf("no entries found")
		}
	case *protocol.LiteDataAccount:
		return fmt.Errorf("lite data accounts don't have indexed entries")
	default:
		return fmt.Errorf("not a data account (type: %s)", account.Account.Type())
	}

	fmt.Printf("📄 Latest entry from: %s\n", accountURL)
	if account.LastBlockTime != nil {
		fmt.Printf("Last Update: %s\n", account.LastBlockTime.Format(time.RFC3339))
	}
	fmt.Println(strings.Repeat("-", 80))

	// Display the current entry (which is the latest)
	if acc, ok := account.Account.(*protocol.DataAccount); ok && acc.Entry != nil {
		r.displayDataEntry(0, acc.Entry)
	}
	return nil
}

func (r *DataReader) displayDataEntry(index int, entry protocol.DataEntry) {
	if entry == nil {
		fmt.Printf("Entry #%d: [Empty]\n", index)
		return
	}

	fmt.Printf("Entry #%d\n", index)
	fmt.Printf("  Type: %s\n", entry.Type())

	// Get data from the entry
	data := entry.GetData()
	if len(data) == 0 {
		fmt.Println("  [No data]")
		return
	}

	// Display each data item
	for i, item := range data {
		if len(data) > 1 {
			fmt.Printf("  Data Item %d:\n", i)
		}
		r.formatData(item, entry.Type().String())
	}
}

func (r *DataReader) formatData(data []byte, entryType string) {
	if len(data) == 0 {
		fmt.Println("  [Empty]")
		return
	}

	format := r.format
	if format == "auto" {
		// Auto-detect format based on content
		if isJSON(data) {
			format = "json"
		} else if isPrintableText(data) {
			format = "text"
		} else {
			format = "hex"
		}
	}

	switch format {
	case "json":
		var v interface{}
		if err := json.Unmarshal(data, &v); err != nil {
			// Not valid JSON, show as hex
			r.formatData(data, "hex")
			return
		}
		formatted, _ := json.MarshalIndent(v, "  ", "  ")
		fmt.Println(string(formatted))

	case "text":
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			fmt.Printf("  %s\n", line)
		}

	case "hex":
		hexStr := hex.EncodeToString(data)
		// Format in chunks for readability
		for i := 0; i < len(hexStr); i += 64 {
			end := i + 64
			if end > len(hexStr) {
				end = len(hexStr)
			}
			fmt.Printf("  %s\n", hexStr[i:end])
		}

	default:
		// Raw bytes with size info
		fmt.Printf("  [%d bytes]\n", len(data))
		if len(data) <= 256 {
			fmt.Printf("  %x\n", data)
		} else {
			fmt.Printf("  %x... (truncated)\n", data[:256])
		}
	}
}

func isJSON(data []byte) bool {
	var v interface{}
	return json.Unmarshal(data, &v) == nil
}

func isPrintableText(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	
	printableCount := 0
	for _, b := range data {
		// Check for printable ASCII and common whitespace
		if (b >= 32 && b <= 126) || b == '\n' || b == '\r' || b == '\t' {
			printableCount++
		}
	}
	
	// Consider it text if >90% printable
	return float64(printableCount)/float64(len(data)) > 0.9
}