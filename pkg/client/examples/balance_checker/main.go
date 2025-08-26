// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// balance_checker is a simple utility to check ACME token balances
// for multiple accounts and display them in a formatted table.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/big"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	var (
		network  = flag.String("network", "mainnet", "Network to connect to (mainnet, testnet, local)")
		accounts = flag.String("accounts", "", "Comma-separated list of account URLs to check")
		watch    = flag.Bool("watch", false, "Watch mode - refresh every 10 seconds")
		csv      = flag.Bool("csv", false, "Output as CSV")
		web      = flag.Bool("web", false, "Launch web UI")
		port     = flag.Int("port", 8080, "Web server port")
	)
	flag.Parse()

	// Parse account list
	var accountList []string
	if *accounts != "" {
		accountList = strings.Split(*accounts, ",")
		for i := range accountList {
			accountList[i] = strings.TrimSpace(accountList[i])
		}
	} else {
		// Default accounts to check
		accountList = []string{
			"acc://ACME",
		}
	}

	// Create client
	c, err := createClient(*network)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	checker := &BalanceChecker{
		client: c,
		csv:    *csv,
	}

	if *web {
		// Launch web server
		server := NewWebServer(checker, *port)
		url := fmt.Sprintf("http://localhost:%d", *port)
		fmt.Printf("💰 Starting web UI at %s\n", url)
		fmt.Println("Press Ctrl+C to stop")
		
		// Try to open browser automatically
		go func() {
			time.Sleep(1 * time.Second) // Give server time to start
			openBrowser(url)
		}()
		
		if err := server.Start(); err != nil {
			log.Fatalf("Failed to start web server: %v", err)
		}
	} else if *watch {
		// Watch mode
		for {
			clearScreen()
			checker.CheckBalances(accountList)
			time.Sleep(10 * time.Second)
		}
	} else {
		// Run once
		checker.CheckBalances(accountList)
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

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

type BalanceChecker struct {
	client *client.Client
	csv    bool
}

type AccountBalance struct {
	URL           string
	Type          string
	Symbol        string
	Balance       *big.Int
	BalanceFloat  float64
	Credits       uint64
	Issued        *big.Int
	SupplyLimit   *big.Int
	Error         error
}

func (b *BalanceChecker) CheckBalances(accounts []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Collect all balances
	balances := make([]AccountBalance, 0, len(accounts))
	
	for _, accountURL := range accounts {
		balance := b.getAccountBalance(ctx, accountURL)
		balances = append(balances, balance)
	}

	// Display results
	if b.csv {
		b.displayCSV(balances)
	} else {
		b.displayTable(balances)
	}
}

func (b *BalanceChecker) getAccountBalance(ctx context.Context, accountURL string) AccountBalance {
	result := AccountBalance{
		URL: accountURL,
	}

	// Get account info
	account, err := b.client.GetAccount(ctx, accountURL)
	if err != nil {
		result.Error = err
		return result
	}

	if account == nil || account.Account == nil {
		result.Error = fmt.Errorf("account not found")
		return result
	}

	result.Type = account.Account.Type().String()

	// Extract balance based on account type
	switch acc := account.Account.(type) {
	case *protocol.TokenIssuer:
		result.Symbol = acc.Symbol
		result.Issued = &acc.Issued
		result.SupplyLimit = acc.SupplyLimit
		// For token issuers, the "balance" is the issued amount
		result.Balance = &acc.Issued
		result.BalanceFloat = b.toTokenAmount(result.Balance, acc.Precision)
		
	case *protocol.LiteTokenAccount:
		result.Balance = &acc.Balance
		// Note: LiteTokenAccount doesn't have CreditBalance in current protocol
		// result.Credits = acc.CreditBalance
		// Get token info
		if acc.TokenUrl != nil {
			tokenInfo, err := b.client.GetAccount(ctx, acc.TokenUrl.String())
			if err == nil && tokenInfo != nil {
				if issuer, ok := tokenInfo.Account.(*protocol.TokenIssuer); ok {
					result.Symbol = issuer.Symbol
					result.BalanceFloat = b.toTokenAmount(result.Balance, issuer.Precision)
				}
			}
		}
		
	case *protocol.TokenAccount:
		result.Balance = &acc.Balance
		// Get token info
		if acc.TokenUrl != nil {
			tokenInfo, err := b.client.GetAccount(ctx, acc.TokenUrl.String())
			if err == nil && tokenInfo != nil {
				if issuer, ok := tokenInfo.Account.(*protocol.TokenIssuer); ok {
					result.Symbol = issuer.Symbol
					result.BalanceFloat = b.toTokenAmount(result.Balance, issuer.Precision)
				}
			}
		}
		
	case *protocol.KeyPage:
		result.Credits = acc.CreditBalance
		
	default:
		// Account type doesn't have a balance
		result.Balance = big.NewInt(0)
	}

	return result
}

func (b *BalanceChecker) toTokenAmount(balance *big.Int, precision uint64) float64 {
	if balance == nil {
		return 0
	}
	divisor := math.Pow(10, float64(precision))
	balanceFloat := new(big.Float).SetInt(balance)
	result, _ := new(big.Float).Quo(balanceFloat, big.NewFloat(divisor)).Float64()
	return result
}

func (b *BalanceChecker) displayTable(balances []AccountBalance) {
	fmt.Printf("\n💰 ACCUMULATE ACCOUNT BALANCES\n")
	fmt.Printf("Time: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ACCOUNT\tTYPE\tSYMBOL\tBALANCE\tCREDITS\tSTATUS\n")
	fmt.Fprintf(w, "-------\t----\t------\t-------\t-------\t------\n")

	for _, bal := range balances {
		status := "✅"
		balanceStr := "-"
		creditsStr := "-"
		symbolStr := bal.Symbol
		
		if bal.Error != nil {
			status = "❌"
			balanceStr = "Error"
			symbolStr = "-"
		} else {
			if bal.Balance != nil && bal.Balance.Sign() > 0 {
				if bal.BalanceFloat > 0 {
					balanceStr = fmt.Sprintf("%.8f", bal.BalanceFloat)
				} else {
					balanceStr = bal.Balance.String()
				}
			}
			
			if bal.Credits > 0 {
				creditsStr = fmt.Sprintf("%d", bal.Credits)
			}
			
			if symbolStr == "" {
				symbolStr = "-"
			}
		}

		// Shorten long URLs for display
		displayURL := bal.URL
		if len(displayURL) > 30 {
			displayURL = displayURL[:27] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			displayURL,
			bal.Type,
			symbolStr,
			balanceStr,
			creditsStr,
			status,
		)
	}

	w.Flush()
	fmt.Println()

	// Display any errors
	hasErrors := false
	for _, bal := range balances {
		if bal.Error != nil {
			if !hasErrors {
				fmt.Println("Errors:")
				hasErrors = true
			}
			fmt.Printf("  • %s: %v\n", bal.URL, bal.Error)
		}
	}

	// Display summary for token issuers
	fmt.Println("\n📊 TOKEN ISSUER SUMMARY")
	for _, bal := range balances {
		if bal.Type == "tokenIssuer" && bal.Error == nil {
			fmt.Printf("  %s (%s):\n", bal.URL, bal.Symbol)
			if bal.Issued != nil {
				fmt.Printf("    • Issued: %.8f %s\n", bal.BalanceFloat, bal.Symbol)
			}
			if bal.SupplyLimit != nil {
				limitFloat := b.toTokenAmount(bal.SupplyLimit, 8) // Assuming precision 8
				fmt.Printf("    • Supply Limit: %.8f %s\n", limitFloat, bal.Symbol)
				
				if bal.Issued != nil && bal.SupplyLimit.Sign() > 0 {
					percentIssued := (bal.BalanceFloat / limitFloat) * 100
					fmt.Printf("    • Utilization: %.2f%%\n", percentIssued)
				}
			}
		}
	}
}

func (b *BalanceChecker) displayCSV(balances []AccountBalance) {
	// CSV header
	fmt.Println("Account,Type,Symbol,Balance,Credits,Error")
	
	for _, bal := range balances {
		errorStr := ""
		if bal.Error != nil {
			errorStr = bal.Error.Error()
		}
		
		balanceStr := "0"
		if bal.Balance != nil {
			if bal.BalanceFloat > 0 {
				balanceStr = fmt.Sprintf("%.8f", bal.BalanceFloat)
			} else {
				balanceStr = bal.Balance.String()
			}
		}
		
		fmt.Printf("%s,%s,%s,%s,%d,%s\n",
			bal.URL,
			bal.Type,
			bal.Symbol,
			balanceStr,
			bal.Credits,
			errorStr,
		)
	}
}

// openBrowser tries to open the URL in the default browser
func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
		args = []string{url}
	}

	err := exec.Command(cmd, args...).Start()
	if err != nil {
		fmt.Printf("Could not open browser automatically. Please visit %s manually.\n", url)
	} else {
		fmt.Printf("Browser opened to %s\n", url)
	}
}