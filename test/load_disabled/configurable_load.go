package main

import (
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	v3api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Configuration flags
var (
	mode           = flag.String("mode", "fixed", "Mode: fixed, continuous, or burst")
	txCount        = flag.Int("txcount", 100, "Number of transactions (for fixed mode)")
	duration       = flag.Duration("duration", 60*time.Second, "Duration for continuous mode")
	tps            = flag.Int("tps", 10, "Target TPS for continuous mode")
	accounts       = flag.Int("accounts", 10, "Number of test accounts to create")
	txAmount       = flag.Int("amount", 1000, "Amount of ACME per transaction")
	reportInterval = flag.Duration("report", 10*time.Second, "Reporting interval for metrics")
	serverURL      = flag.String("server", "http://127.0.0.1:26660/v3", "API server URL")
	verbose        = flag.Bool("verbose", false, "Verbose output")
	dropRate       = flag.Float64("droprate", 0.0, "Simulate transaction drop rate (0.0-1.0)")
)

// Metrics structure for tracking
type Metrics struct {
	// Transaction counts
	TotalTransactions int64
	SuccessfulTx      int64
	FailedTx          int64
	DroppedTx         int64
	RetriedTx         int64

	// Token metrics
	TokensSent   int64
	TokensMinted int64
	CreditsUsed  int64

	// Performance metrics
	StartTime      time.Time
	LastReportTime time.Time
	LastTxCount    int64
	CurrentTPS     float64
	PeakTPS        float64
	AverageTPS     float64

	// Partition routing
	CrossPartitionTx int64
	SamePartitionTx  int64
	PartitionRoutes  map[string]int64

	mu sync.RWMutex
}

// Account structure
type Account struct {
	PrivateKey  ed25519.PrivateKey
	TokenURL    *url.URL
	IdentityURL *url.URL
	Partition   string
	Balance     int64
	Credits     int64
}

// Global metrics instance
var metrics = &Metrics{
	PartitionRoutes: make(map[string]int64),
	StartTime:       time.Now(),
	LastReportTime:  time.Now(),
}

func main() {
	flag.Parse()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create API client
	client := jsonrpc.NewClient(*serverURL)

	fmt.Println("🚀 Configurable Load Test Runner")
	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Printf("Mode: %s\n", *mode)

	switch *mode {
	case "fixed":
		fmt.Printf("Transactions: %d\n", *txCount)
	case "continuous":
		fmt.Printf("Duration: %v\n", *duration)
		fmt.Printf("Target TPS: %d\n", *tps)
	case "burst":
		fmt.Printf("Burst size: %d\n", *txCount)
	default:
		log.Fatal("Invalid mode. Use: fixed, continuous, or burst")
	}

	fmt.Printf("Accounts: %d\n", *accounts)
	fmt.Printf("Amount per TX: %d ACME\n", *txAmount)
	fmt.Printf("Report interval: %v\n", *reportInterval)
	if *dropRate > 0 {
		fmt.Printf("Drop rate: %.1f%%\n", *dropRate*100)
	}
	fmt.Println()

	// Create and fund accounts
	testAccounts := setupAccounts(client, *accounts)
	if len(testAccounts) < 2 {
		log.Fatal("Need at least 2 accounts for testing")
	}

	// Start metrics reporter
	stopReporter := make(chan bool)
	go metricsReporter(stopReporter)

	// Run the selected mode
	stopTest := make(chan bool)
	var wg sync.WaitGroup
	done := make(chan bool)

	go func() {
		switch *mode {
		case "fixed":
			runFixedMode(client, testAccounts, *txCount, &wg)
			wg.Wait()
		case "continuous":
			runContinuousMode(client, testAccounts, *duration, *tps, stopTest, &wg)
			wg.Wait()
		case "burst":
			runBurstMode(client, testAccounts, *txCount, &wg)
			wg.Wait()
		}
		close(done)
	}()

	// Wait for interrupt or completion
	select {
	case <-sigChan:
		fmt.Println("\n⚠️ Interrupt received, shutting down...")
		close(stopTest)
	case <-done:
		// Test completed normally
	case <-time.After(1 * time.Hour): // Safety timeout
		if *mode == "continuous" {
			close(stopTest)
		}
	}

	// Wait for all transactions to complete
	wg.Wait()
	close(stopReporter)

	// Print final report
	printFinalReport()
}

// setupAccounts creates and funds test accounts
func setupAccounts(client *jsonrpc.Client, count int) []*Account {
	fmt.Println("📝 Creating test accounts...")

	accounts := make([]*Account, 0, count)
	partitionCounts := make(map[string]int)

	for i := 0; i < count*3 && len(accounts) < count; i++ {
		acc, err := createAccount()
		if err != nil {
			log.Printf("Failed to create account: %v", err)
			continue
		}

		// Distribute accounts across partitions
		if partitionCounts[acc.Partition] < (count/3)+1 {
			accounts = append(accounts, acc)
			partitionCounts[acc.Partition]++
			if *verbose {
				fmt.Printf("  Account %d: %s (%s)\n", len(accounts), acc.TokenURL.ShortString(), acc.Partition)
			}
		}
	}

	fmt.Printf("Created %d accounts across %d partitions\n", len(accounts), len(partitionCounts))
	for partition, count := range partitionCounts {
		fmt.Printf("  %s: %d accounts\n", partition, count)
	}

	// Fund accounts
	fmt.Println("\n💰 Funding accounts...")
	fundAccounts(accounts)

	// Add credits
	fmt.Println("\n💳 Adding credits...")
	addCreditsToAccounts(client, accounts)

	fmt.Println("\n✅ Accounts ready!")
	return accounts
}

// createAccount creates a new test account
func createAccount() (*Account, error) {
	seed := make([]byte, 32)
	_, err := cryptorand.Read(seed)
	if err != nil {
		return nil, err
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey[32:]

	tokenURL, err := protocol.LiteTokenAddress(publicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		return nil, err
	}

	return &Account{
		PrivateKey:  privateKey,
		TokenURL:    tokenURL,
		IdentityURL: tokenURL.Identity(),
		Partition:   getPartition(tokenURL.String()),
		Balance:     0,
		Credits:     0,
	}, nil
}

// getPartition determines partition for an account
func getPartition(accountURL string) string {
	hash := 0
	for _, c := range accountURL {
		hash = hash*31 + int(c)
	}
	bvn := (hash % 3) + 1
	return fmt.Sprintf("BVN%d", bvn)
}

// fundAccounts funds all accounts via faucet
func fundAccounts(accounts []*Account) {
	for round := 0; round < 3; round++ {
		for i, acc := range accounts {
			resp, err := http.Post(
				"http://127.0.0.1:26660/faucet",
				"text/plain",
				strings.NewReader(acc.TokenURL.String()),
			)
			if err != nil {
				log.Printf("Failed to fund account %d: %v", i, err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == 200 {
				atomic.AddInt64(&acc.Balance, 10000000) // 10M ACME from faucet
				atomic.AddInt64(&metrics.TokensMinted, 10000000)
			}

			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(2 * time.Second)
	}

	// Wait for accounts to be ready
	time.Sleep(5 * time.Second)
}

// addCreditsToAccounts adds credits to all accounts
func addCreditsToAccounts(client *jsonrpc.Client, accounts []*Account) {
	ctx := context.Background()

	for i, acc := range accounts {
		ns, err := client.NetworkStatus(ctx, v3api.NetworkStatusOptions{Partition: "Directory"})
		if err != nil {
			log.Printf("Failed to get network status: %v", err)
			continue
		}

		oracle := float64(ns.Oracle.Price) / 1e8
		if oracle == 0 {
			oracle = 0.01
		}

		timestamp := uint64(time.Now().UnixMilli())
		creditAmount := int64(1000000) // 10 ACME worth of credits

		env, err := build.Transaction().
			For(acc.TokenURL).
			Body(&protocol.AddCredits{
				Recipient: acc.IdentityURL,
				Amount:    *big.NewInt(creditAmount),
				Oracle:    uint64(oracle * 1e8),
			}).
			SignWith(acc.TokenURL).Version(1).Timestamp(&timestamp).PrivateKey(acc.PrivateKey).
			Done()

		if err != nil {
			log.Printf("Failed to build credits transaction for account %d: %v", i, err)
			continue
		}

		subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("Failed to submit credits transaction for account %d: %v", i, err)
			continue
		}

		success := true
		for _, sub := range subs {
			if err := sub.Status.AsError(); err != nil {
				success = false
				break
			}
		}

		if success {
			atomic.StoreInt64(&acc.Credits, creditAmount)
			atomic.AddInt64(&metrics.CreditsUsed, creditAmount)
			if *verbose {
				fmt.Printf("  ✅ Added credits to account %d\n", i)
			}
		}
	}

	time.Sleep(3 * time.Second)
}

// runFixedMode runs a fixed number of transactions
func runFixedMode(client *jsonrpc.Client, accounts []*Account, count int, wg *sync.WaitGroup) {
	fmt.Printf("\n📤 Sending %d transactions...\n", count)

	// Launch all transactions
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(txNum int) {
			defer wg.Done()

			from := accounts[txNum%len(accounts)]
			to := accounts[(txNum+1)%len(accounts)]

			err := sendTransaction(client, from, to, int64(*txAmount), txNum)

			if err != nil {
				if *verbose {
					log.Printf("TX %d failed: %v", txNum, err)
				}
			} else if *verbose {
				fmt.Printf("✅ TX %d: %s→%s\n", txNum, from.Partition, to.Partition)
			}
		}(i)

		// Control rate
		if i%10 == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// runContinuousMode runs transactions continuously at target TPS
func runContinuousMode(client *jsonrpc.Client, accounts []*Account, duration time.Duration, targetTPS int, stop chan bool, wg *sync.WaitGroup) {
	fmt.Printf("\n♾️ Running continuous mode for %v at %d TPS...\n", duration, targetTPS)

	ticker := time.NewTicker(time.Second / time.Duration(targetTPS))
	defer ticker.Stop()

	timeout := time.After(duration)
	txNum := 0

	for {
		select {
		case <-stop:
			fmt.Println("Stopping continuous mode...")
			return
		case <-timeout:
			fmt.Println("Duration reached, stopping...")
			return
		case <-ticker.C:
			wg.Add(1)
			go func(num int) {
				defer wg.Done()

				from := accounts[num%len(accounts)]
				to := accounts[(num+1)%len(accounts)]

				err := sendTransaction(client, from, to, int64(*txAmount), num)

				if err == nil && *verbose {
					fmt.Printf("✅ TX %d: %s→%s\n", num, from.Partition, to.Partition)
				}
			}(txNum)
			txNum++
		}
	}
}

// runBurstMode sends bursts of transactions
func runBurstMode(client *jsonrpc.Client, accounts []*Account, burstSize int, wg *sync.WaitGroup) {
	fmt.Printf("\n💥 Sending burst of %d transactions...\n", burstSize)

	// Send all transactions at once
	for i := 0; i < burstSize; i++ {
		wg.Add(1)
		go func(txNum int) {
			defer wg.Done()

			from := accounts[txNum%len(accounts)]
			to := accounts[(txNum+1)%len(accounts)]

			err := sendTransaction(client, from, to, int64(*txAmount), txNum)

			if err == nil && *verbose {
				fmt.Printf("✅ TX %d: %s→%s\n", txNum, from.Partition, to.Partition)
			}
		}(i)
	}
}

// sendTransaction sends a single transaction
func sendTransaction(client *jsonrpc.Client, from, to *Account, amount int64, txNum int) error {
	// Simulate drops if configured
	if *dropRate > 0 && shouldDrop() {
		atomic.AddInt64(&metrics.DroppedTx, 1)
		if *verbose {
			fmt.Printf("💥 TX %d dropped (simulated)\n", txNum)
		}
		return fmt.Errorf("simulated drop")
	}

	ctx := context.Background()
	timestamp := uint64(time.Now().UnixMilli())

	env, err := build.Transaction().
		For(from.TokenURL).
		Body(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    to.TokenURL,
				Amount: *big.NewInt(amount),
			}},
		}).
		SignWith(from.TokenURL).Version(1).Timestamp(&timestamp).PrivateKey(from.PrivateKey).
		Done()

	if err != nil {
		atomic.AddInt64(&metrics.FailedTx, 1)
		return err
	}

	subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		atomic.AddInt64(&metrics.FailedTx, 1)
		return err
	}

	for _, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			atomic.AddInt64(&metrics.FailedTx, 1)
			return err
		}
	}

	// Update metrics
	atomic.AddInt64(&metrics.TotalTransactions, 1)
	atomic.AddInt64(&metrics.SuccessfulTx, 1)
	atomic.AddInt64(&metrics.TokensSent, amount)

	// Track routing
	route := fmt.Sprintf("%s→%s", from.Partition, to.Partition)
	metrics.mu.Lock()
	metrics.PartitionRoutes[route]++
	if from.Partition != to.Partition {
		atomic.AddInt64(&metrics.CrossPartitionTx, 1)
	} else {
		atomic.AddInt64(&metrics.SamePartitionTx, 1)
	}
	metrics.mu.Unlock()

	// Update account balances
	atomic.AddInt64(&from.Balance, -amount)
	atomic.AddInt64(&to.Balance, amount)

	return nil
}

// shouldDrop determines if a transaction should be dropped
func shouldDrop() bool {
	n, _ := cryptorand.Int(cryptorand.Reader, big.NewInt(100))
	return float64(n.Int64()) < (*dropRate * 100)
}

// metricsReporter periodically reports metrics
func metricsReporter(stop chan bool) {
	ticker := time.NewTicker(*reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			printMetricsReport()
		}
	}
}

// printMetricsReport prints current metrics
func printMetricsReport() {
	now := time.Now()
	duration := now.Sub(metrics.StartTime)
	intervalDuration := now.Sub(metrics.LastReportTime)

	totalTx := atomic.LoadInt64(&metrics.TotalTransactions)
	successTx := atomic.LoadInt64(&metrics.SuccessfulTx)
	failedTx := atomic.LoadInt64(&metrics.FailedTx)
	droppedTx := atomic.LoadInt64(&metrics.DroppedTx)
	tokensSent := atomic.LoadInt64(&metrics.TokensSent)
	crossPartition := atomic.LoadInt64(&metrics.CrossPartitionTx)
	samePartition := atomic.LoadInt64(&metrics.SamePartitionTx)

	// Calculate TPS
	intervalTx := totalTx - metrics.LastTxCount
	currentTPS := float64(intervalTx) / intervalDuration.Seconds()
	averageTPS := float64(totalTx) / duration.Seconds()

	if currentTPS > metrics.PeakTPS {
		metrics.PeakTPS = currentTPS
	}

	fmt.Println("\n" + strings.Repeat("─", 60))
	fmt.Printf("📊 Metrics Report at %s\n", now.Format("15:04:05"))
	fmt.Println(strings.Repeat("─", 60))

	fmt.Printf("⏱️ Duration: %v\n", duration.Round(time.Second))
	fmt.Printf("📈 Current TPS: %.2f | Average: %.2f | Peak: %.2f\n",
		currentTPS, averageTPS, metrics.PeakTPS)

	fmt.Printf("\n📊 Transactions:\n")
	fmt.Printf("  Total: %d | Success: %d | Failed: %d", totalTx, successTx, failedTx)
	if droppedTx > 0 {
		fmt.Printf(" | Dropped: %d", droppedTx)
	}
	fmt.Println()

	if totalTx > 0 {
		successRate := float64(successTx) / float64(totalTx) * 100
		fmt.Printf("  Success Rate: %.1f%%\n", successRate)
	}

	fmt.Printf("\n💰 Tokens:\n")
	fmt.Printf("  Sent: %s ACME\n", formatACME(tokensSent))
	fmt.Printf("  Minted: %s ACME\n", formatACME(atomic.LoadInt64(&metrics.TokensMinted)))
	fmt.Printf("  Credits Used: %s\n", formatCredits(atomic.LoadInt64(&metrics.CreditsUsed)))

	fmt.Printf("\n🌐 Routing:\n")
	fmt.Printf("  Cross-partition: %d (%.1f%%)\n",
		crossPartition, float64(crossPartition)/float64(totalTx)*100)
	fmt.Printf("  Same-partition: %d (%.1f%%)\n",
		samePartition, float64(samePartition)/float64(totalTx)*100)

	// Update for next interval
	metrics.LastReportTime = now
	metrics.LastTxCount = totalTx
	metrics.CurrentTPS = currentTPS
}

// printFinalReport prints the final summary
func printFinalReport() {
	fmt.Println("\n" + strings.Repeat("═", 60))
	fmt.Println("🏁 Final Report")
	fmt.Println(strings.Repeat("═", 60))

	duration := time.Since(metrics.StartTime)
	totalTx := atomic.LoadInt64(&metrics.TotalTransactions)
	successTx := atomic.LoadInt64(&metrics.SuccessfulTx)
	failedTx := atomic.LoadInt64(&metrics.FailedTx)
	droppedTx := atomic.LoadInt64(&metrics.DroppedTx)
	tokensSent := atomic.LoadInt64(&metrics.TokensSent)
	tokensMinted := atomic.LoadInt64(&metrics.TokensMinted)
	creditsUsed := atomic.LoadInt64(&metrics.CreditsUsed)
	crossPartition := atomic.LoadInt64(&metrics.CrossPartitionTx)
	samePartition := atomic.LoadInt64(&metrics.SamePartitionTx)

	fmt.Printf("Total Duration: %v\n", duration.Round(time.Second))
	fmt.Printf("Total Transactions: %d\n", totalTx)
	fmt.Printf("  ✅ Successful: %d\n", successTx)
	fmt.Printf("  ❌ Failed: %d\n", failedTx)
	if droppedTx > 0 {
		fmt.Printf("  💥 Dropped: %d\n", droppedTx)
	}

	if totalTx > 0 {
		fmt.Printf("\nSuccess Rate: %.2f%%\n", float64(successTx)/float64(totalTx)*100)
		fmt.Printf("Average TPS: %.2f\n", float64(totalTx)/duration.Seconds())
		fmt.Printf("Peak TPS: %.2f\n", metrics.PeakTPS)
	}

	fmt.Printf("\n💰 Token Summary:\n")
	fmt.Printf("  Total Sent: %s ACME\n", formatACME(tokensSent))
	fmt.Printf("  Total Minted: %s ACME\n", formatACME(tokensMinted))
	fmt.Printf("  Credits Used: %s\n", formatCredits(creditsUsed))

	fmt.Printf("\n🌐 Partition Routing:\n")
	fmt.Printf("  Cross-partition: %d (%.1f%%)\n",
		crossPartition, float64(crossPartition)/float64(totalTx)*100)
	fmt.Printf("  Same-partition: %d (%.1f%%)\n",
		samePartition, float64(samePartition)/float64(totalTx)*100)

	fmt.Printf("\n📍 Route Distribution:\n")
	metrics.mu.RLock()
	for route, count := range metrics.PartitionRoutes {
		fmt.Printf("  %s: %d transactions\n", route, count)
	}
	metrics.mu.RUnlock()
}

// formatACME formats ACME amount for display
func formatACME(amount int64) string {
	acme := float64(amount) / 1e8
	return fmt.Sprintf("%.2f", acme)
}

// formatCredits formats credits for display
func formatCredits(credits int64) string {
	return fmt.Sprintf("%.2f", float64(credits)/100)
}
