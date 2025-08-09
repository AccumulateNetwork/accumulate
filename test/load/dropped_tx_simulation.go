package main

import (
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	v3api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// DroppedTxSimulator simulates network issues that drop transactions
type DroppedTxSimulator struct {
	dropRate      float64 // Percentage of transactions to drop (0.0 to 1.0)
	droppedCount  int64
	allowedCount  int64
	isActive      bool
	mu            sync.RWMutex
	blockedPorts  map[int]bool
	droppedTxIDs  map[string]time.Time
}

// Global simulator instance
var txDropper = &DroppedTxSimulator{
	dropRate:     0.3, // 30% drop rate
	blockedPorts: make(map[int]bool),
	droppedTxIDs: make(map[string]time.Time),
}

// SimulateNetworkDrop randomly drops transactions to simulate network issues
func (s *DroppedTxSimulator) ShouldDropTransaction() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.isActive {
		return false
	}
	
	// Random drop based on rate
	if rand.Float64() < s.dropRate {
		atomic.AddInt64(&s.droppedCount, 1)
		return true
	}
	
	atomic.AddInt64(&s.allowedCount, 1)
	return false
}

// SimulatePortBlock temporarily blocks a network port
func (s *DroppedTxSimulator) BlockPort(port int, duration time.Duration) {
	s.mu.Lock()
	s.blockedPorts[port] = true
	s.mu.Unlock()
	
	fmt.Printf("🚫 Blocking port %d for %v\n", port, duration)
	
	time.AfterFunc(duration, func() {
		s.mu.Lock()
		delete(s.blockedPorts, port)
		s.mu.Unlock()
		fmt.Printf("✅ Port %d unblocked\n", port)
	})
}

// IsPortBlocked checks if a port is currently blocked
func (s *DroppedTxSimulator) IsPortBlocked(port int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.blockedPorts[port]
}

// NetworkInterceptor wraps HTTP client to simulate network issues
type NetworkInterceptor struct {
	client    *http.Client
	simulator *DroppedTxSimulator
}

func (n *NetworkInterceptor) RoundTrip(req *http.Request) (*http.Response, error) {
	// Extract port from URL
	host := req.URL.Host
	
	// Simulate dropped transaction
	if n.simulator.ShouldDropTransaction() {
		// Log the dropped transaction
		fmt.Printf("💥 DROPPED: Transaction to %s\n", host)
		
		// Return network timeout error
		return nil, &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: &timeoutError{},
		}
	}
	
	// Otherwise, proceed normally
	return http.DefaultTransport.RoundTrip(req)
}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "simulated network timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

// Test account structure
type TestAccount struct {
	PrivateKey  ed25519.PrivateKey
	TokenURL    *url.URL
	IdentityURL *url.URL
	Partition   string
}

// Create test account
func createTestAccount() (*TestAccount, error) {
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
	
	return &TestAccount{
		PrivateKey:  privateKey,
		TokenURL:    tokenURL,
		IdentityURL: tokenURL.Identity(),
		Partition:   getPartitionForAccount(tokenURL.String()),
	}, nil
}

func getPartitionForAccount(accountURL string) string {
	hash := 0
	for _, c := range accountURL {
		hash = hash*31 + int(c)
	}
	bvn := (hash % 3) + 1
	return fmt.Sprintf("BVN%d", bvn)
}

// Fund account using faucet
func fundAccount(tokenURL *url.URL) error {
	resp, err := http.Post(
		"http://127.0.0.1:26660/faucet",
		"text/plain",
		strings.NewReader(tokenURL.String()),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return fmt.Errorf("faucet failed with status %d", resp.StatusCode)
	}
	
	return nil
}

// Main test function
func main() {
	fmt.Println("🧪 CrossChainConductor Dropped Transaction Test")
	fmt.Println("Testing error detection and retry mechanism")
	fmt.Println("=" + strings.Repeat("=", 50))
	
	// Create API client
	client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
	// Note: We'll simulate drops at the transaction submission level
	
	// Create test accounts - ensure we get accounts in each partition
	fmt.Println("\n📝 Creating test accounts strategically across partitions...")
	var accounts []*TestAccount
	partitionAccounts := make(map[string][]*TestAccount)
	targetPerPartition := 2
	maxAttempts := 100
	
	for attempt := 0; attempt < maxAttempts && len(accounts) < 6; attempt++ {
		acc, err := createTestAccount()
		if err != nil {
			log.Fatalf("Failed to create account: %v", err)
		}
		
		// Only add if we need more accounts in this partition
		if len(partitionAccounts[acc.Partition]) < targetPerPartition {
			accounts = append(accounts, acc)
			partitionAccounts[acc.Partition] = append(partitionAccounts[acc.Partition], acc)
			fmt.Printf("Account %d: %s (%s)\n", len(accounts)-1, acc.TokenURL.String(), acc.Partition)
		}
		
		// Check if we have enough accounts in different partitions
		if len(partitionAccounts) >= 3 {
			hasEnough := true
			for _, accs := range partitionAccounts {
				if len(accs) < 1 {
					hasEnough = false
					break
				}
			}
			if hasEnough && len(accounts) >= 6 {
				break
			}
		}
	}
	
	fmt.Printf("\nAccount distribution:\n")
	for partition, accs := range partitionAccounts {
		fmt.Printf("  %s: %d accounts\n", partition, len(accs))
	}
	
	// Fund accounts multiple times to ensure they have enough ACME
	fmt.Println("\n💰 Funding accounts (multiple rounds for sufficient balance)...")
	for round := 0; round < 5; round++ {
		fmt.Printf("  Funding round %d/5...\n", round+1)
		for i, acc := range accounts {
			if err := fundAccount(acc.TokenURL); err != nil {
				log.Printf("Failed to fund account %d in round %d: %v", i, round+1, err)
			}
			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(2 * time.Second) // Wait between rounds
	}
	
	fmt.Println("\n⏳ Waiting for accounts to be ready...")
	time.Sleep(10 * time.Second) // Give more time for accounts to be created
	
	// Add credits to accounts (with retry)
	fmt.Println("\n💳 Adding credits to accounts...")
	creditSuccess := 0
	for i, acc := range accounts {
		// Try multiple times to add credits
		for attempt := 0; attempt < 3; attempt++ {
			if err := addCredits(client, acc); err != nil {
				if attempt == 2 {
					log.Printf("Failed to add credits to account %d after 3 attempts: %v", i, err)
				}
				time.Sleep(500 * time.Millisecond)
			} else {
				creditSuccess++
				fmt.Printf("  ✅ Added credits to account %d\n", i)
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	
	fmt.Printf("Successfully added credits to %d/%d accounts\n", creditSuccess, len(accounts))
	
	if creditSuccess == 0 {
		log.Fatal("❌ Failed to add credits to any accounts. Cannot proceed with test.")
	}
	
	fmt.Println("\n⏳ Waiting for credits to propagate...")
	time.Sleep(5 * time.Second)
	
	// Start monitoring for retries
	fmt.Println("\n🔍 Starting transaction monitoring...")
	fmt.Println("Drop rate: 30% of transactions will be dropped")
	fmt.Println("Expected: CrossChainConductor should detect and retry")
	fmt.Println("")
	
	// Enable dropping
	txDropper.isActive = true
	
	// Track results
	var (
		totalAttempts    int64
		successfulTx     int64
		failedTx         int64
		retriedTx        int64
		startTime        = time.Now()
	)
	
	// Send cross-partition transactions
	fmt.Println("📤 Sending cross-partition transactions with simulated drops...")
	
	var wg sync.WaitGroup
	for round := 0; round < 3; round++ { // Reduce rounds to avoid exhausting credits
		fmt.Printf("\n🔄 Round %d:\n", round+1)
		
		for i := 0; i < len(accounts); i++ {
			for j := 0; j < len(accounts); j++ {
				if i == j || accounts[i].Partition == accounts[j].Partition {
					continue // Skip same account or same partition
				}
				
				wg.Add(1)
				go func(from, to *TestAccount, txNum int) {
					defer wg.Done()
					
					atomic.AddInt64(&totalAttempts, 1)
					
					// Try to send transaction (small amount to avoid exhausting balance)
					err := sendTransaction(client, from, to, 100) // Reduced from 10000 to 100
					
					if err != nil {
						if strings.Contains(err.Error(), "timeout") {
							fmt.Printf("  💥 TX %d: %s→%s DROPPED (will retry)\n", 
								txNum, from.Partition, to.Partition)
							atomic.AddInt64(&retriedTx, 1)
						} else {
							fmt.Printf("  ❌ TX %d: %s→%s failed: %v\n", 
								txNum, from.Partition, to.Partition, err)
							atomic.AddInt64(&failedTx, 1)
						}
					} else {
						fmt.Printf("  ✅ TX %d: %s→%s succeeded\n", 
							txNum, from.Partition, to.Partition)
						atomic.AddInt64(&successfulTx, 1)
					}
				}(accounts[i], accounts[j], round*100+i*10+j)
			}
		}
		
		wg.Wait()
		time.Sleep(2 * time.Second) // Allow time for retries
	}
	
	// Disable dropping
	txDropper.isActive = false
	
	// Wait for any retries to complete
	fmt.Println("\n⏳ Waiting for retry mechanism to complete...")
	time.Sleep(10 * time.Second)
	
	// Print results
	duration := time.Since(startTime)
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📊 Test Results:")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Total transaction attempts: %d\n", atomic.LoadInt64(&totalAttempts))
	fmt.Printf("✅ Successful: %d\n", atomic.LoadInt64(&successfulTx))
	fmt.Printf("❌ Failed: %d\n", atomic.LoadInt64(&failedTx))
	fmt.Printf("🔄 Dropped (for retry): %d\n", atomic.LoadInt64(&txDropper.droppedCount))
	fmt.Printf("➡️ Allowed through: %d\n", atomic.LoadInt64(&txDropper.allowedCount))
	
	dropRate := float64(txDropper.droppedCount) / float64(txDropper.droppedCount + txDropper.allowedCount) * 100
	fmt.Printf("\nActual drop rate: %.1f%%\n", dropRate)
	
	fmt.Println("\n🔍 Error Detection Analysis:")
	fmt.Println("Expected behavior:")
	fmt.Println("1. ~30% of transactions should be initially dropped")
	fmt.Println("2. CrossChainConductor should detect transmission errors")
	fmt.Println("3. Dropped transactions should be automatically retried")
	fmt.Println("4. Most dropped transactions should eventually succeed")
	
	if atomic.LoadInt64(&txDropper.droppedCount) > 0 {
		fmt.Println("\n✅ Successfully simulated network drops!")
		fmt.Println("Check CrossChainConductor logs for:")
		fmt.Println("  - 'Transmission error detected' messages")
		fmt.Println("  - 'Transaction queued for retry' messages")
		fmt.Println("  - 'Transaction retry successful' messages")
	}
}

// Add credits to account
func addCredits(client *jsonrpc.Client, account *TestAccount) error {
	ctx := context.Background()
	timestamp := uint64(time.Now().UnixMilli())
	
	ns, err := client.NetworkStatus(ctx, v3api.NetworkStatusOptions{Partition: "Directory"})
	if err != nil {
		return fmt.Errorf("failed to get network status: %v", err)
	}
	
	oracle := float64(ns.Oracle.Price) / 1e8
	if oracle == 0 {
		oracle = 0.01
	}
	
	env, err := build.Transaction().
		For(account.TokenURL).
		Body(&protocol.AddCredits{
			Recipient: account.IdentityURL,
			Amount:    *big.NewInt(1000000), // 10 ACME worth of credits for more transactions
			Oracle:    uint64(oracle * 1e8),
		}).
		SignWith(account.TokenURL).Version(1).Timestamp(&timestamp).PrivateKey(account.PrivateKey).
		Done()
	
	if err != nil {
		return err
	}
	
	subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		return err
	}
	
	for i, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			return fmt.Errorf("result %d failed: %v", i, err)
		}
	}
	
	return nil
}

// Send transaction
func sendTransaction(client *jsonrpc.Client, from, to *TestAccount, amount int64) error {
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
		return err
	}
	
	// Simulate drop before submission
	if txDropper.ShouldDropTransaction() {
		return &net.OpError{
			Op:  "write",
			Net: "tcp",
			Err: &timeoutError{},
		}
	}
	
	subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		return err
	}
	
	for i, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			return fmt.Errorf("result %d failed: %v", i, err)
		}
	}
	
	return nil
}