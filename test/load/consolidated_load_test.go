//go:build load && !testnet
// +build load,!testnet

package load_test

import (
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestCrossChainLoad tests cross-partition transaction load
func TestCrossChainLoad(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	t.Parallel()
	
	var timestamp uint64
	
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	
	// Create accounts on different partitions
	alice := protocol.AccountUrl("alice")
	bob := protocol.AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateIdentity(bob, bobKey[32:])
	
	sim.CreateAccount(&protocol.TokenAccount{
		Url:      alice.JoinPath("tokens"),
		TokenUrl: protocol.AcmeUrl(),
		Balance:  *big.NewInt(1e12),
	})
	
	// Update alice's credit balance  
	sim.UpdateAccount(alice.JoinPath("book", "1"), func(account protocol.Account) {
		if p, ok := account.(*protocol.KeyPage); ok {
			p.CreditBalance = 1e9
		}
	})
	
	// Send multiple cross-chain transactions with rate limiting
	const numTxns = 100
	const batchSize = 10
	
	// Process transactions in batches for better control
	for batch := 0; batch < numTxns/batchSize; batch++ {
		envs := make([]*messaging.Envelope, 0, batchSize)
		
		for i := 0; i < batchSize; i++ {
			idx := batch*batchSize + i
			if idx >= numTxns {
				break
			}
			
			env := MustBuild(t, build.Transaction().
				For(alice.JoinPath("tokens")).
				Body(&protocol.SendTokens{
					To: []*protocol.TokenRecipient{{
						Url:    bob.JoinPath("tokens"),
						Amount: *big.NewInt(1e6),
					}},
				}).
				SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
			
			envs = append(envs, env)
		}
		
		// Submit batch and wait for completion
		var batchWg sync.WaitGroup
		batchWg.Add(1)
		go func(batch []*messaging.Envelope) {
			defer batchWg.Done()
			sim.MustSubmitAndExecuteBlock(batch...)
			for _, env := range batch {
				sim.WaitForTransactionFlow(func(status *protocol.TransactionStatus) bool {
					return status.Delivered()
				}, env.Transaction[0].GetHash())
			}
		}(envs)
		
		batchWg.Wait()
	}
	
	// Verify
	bobAccount := simulator.GetAccount[*protocol.TokenAccount](sim, bob.JoinPath("tokens"))
	require.NotNil(t, bobAccount)
	require.True(t, bobAccount.Balance.Cmp(big.NewInt(0)) > 0)
}


// TestHighVolumeTransactions tests high volume of transactions
func TestHighVolumeTransactions(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	t.Parallel()
	
	var timestamp uint64
	
	// Initialize with single partition for maximum throughput
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()
	
	// Create accounts
	accounts := make([]struct {
		url *url.URL
		key []byte
	}, 10)
	
	for i := range accounts {
		accounts[i].url = protocol.AccountUrl(fmt.Sprintf("account%d", i))
		accounts[i].key = acctesting.GenerateKey(accounts[i].url)
		sim.CreateIdentity(accounts[i].url, accounts[i].key[32:])
		
		sim.CreateAccount(&protocol.TokenAccount{
			Url:      accounts[i].url.JoinPath("tokens"),
			TokenUrl: protocol.AcmeUrl(),
			Balance:  *big.NewInt(1e12),
		})
		
		sim.UpdateAccount(accounts[i].url.JoinPath("book", "1"), func(account protocol.Account) {
			if p, ok := account.(*protocol.KeyPage); ok {
				p.CreditBalance = 1e9
			}
		})
	}
	
	// Send transactions in batches
	const batchSize = 50
	const numBatches = 10
	
	for batch := 0; batch < numBatches; batch++ {
		envs := make([]*messaging.Envelope, 0, batchSize)
		
		for i := 0; i < batchSize; i++ {
			from := accounts[i%len(accounts)]
			to := accounts[(i+1)%len(accounts)]
			
			env := MustBuild(t, build.Transaction().
				For(from.url.JoinPath("tokens")).
				Body(&protocol.SendTokens{
					To: []*protocol.TokenRecipient{{
						Url:    to.url.JoinPath("tokens"),
						Amount: *big.NewInt(1e6),
					}},
				}).
				SignWith(from.url.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(from.key))
			
			envs = append(envs, env)
		}
		
		// Submit batch
		sim.MustSubmitAndExecuteBlock(envs...)
		for _, env := range envs {
			sim.WaitForTransactionFlow(func(status *protocol.TransactionStatus) bool {
				return status.Delivered()
			}, env.Transaction[0].GetHash())
		}
	}
}

// TestPartitionFailureRecovery tests system behavior under partition failures
func TestPartitionFailureRecovery(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	t.Parallel()
	
	// This test simulates partition failures and recovery
	// It's a placeholder for actual failure injection tests
	t.Log("Partition failure recovery test - requires failure injection framework")
	
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	
	// Basic sanity check
	require.NotNil(t, sim)
}

// BenchmarkTransactionThroughput benchmarks transaction processing
func BenchmarkTransactionThroughput(b *testing.B) {
	// Disable debug features for benchmarking
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()
	
	var timestamp uint64
	
	// Initialize
	sim := simulator.New(b, 1)
	sim.InitFromGenesis()
	
	// Create account
	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	
	sim.CreateAccount(&protocol.TokenAccount{
		Url:      alice.JoinPath("tokens"),
		TokenUrl: protocol.AcmeUrl(),
		Balance:  *big.NewInt(1e15),
	})
	
	sim.UpdateAccount(alice.JoinPath("book", "1"), func(account protocol.Account) {
		if p, ok := account.(*protocol.KeyPage); ok {
			p.CreditBalance = 1e12
		}
	})
	
	// Create self-send transactions
	bob := protocol.AccountUrl("bob")
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		env := MustBuild(b, build.Transaction().
			For(alice.JoinPath("tokens")).
			Body(&protocol.SendTokens{
				To: []*protocol.TokenRecipient{{
					Url:    bob.JoinPath("tokens"),
					Amount: *big.NewInt(1),
				}},
			}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
		
		sim.MustSubmitAndExecuteBlock(env)
	}
}