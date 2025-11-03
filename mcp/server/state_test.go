package server

import (
	"sync"
	"testing"
)

func TestNewState(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	if state == nil {
		t.Fatal("NewState() returned nil")
	}

	if state.Config != cfg {
		t.Error("state.Config does not match provided config")
	}

	if !state.IsLocked {
		t.Error("new state should be locked by default")
	}

	if state.ActiveVault != "" {
		t.Error("new state should have empty ActiveVault")
	}

	if state.VaultToken != nil {
		t.Error("new state should have nil VaultToken")
	}
}

func TestSetActiveVault(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	vaultName := "test-vault"
	token := []byte("test-token")

	state.SetActiveVault(vaultName, token)

	if state.ActiveVault != vaultName {
		t.Errorf("expected ActiveVault '%s', got '%s'", vaultName, state.ActiveVault)
	}

	if string(state.VaultToken) != string(token) {
		t.Errorf("expected VaultToken '%s', got '%s'", string(token), string(state.VaultToken))
	}

	if state.IsLocked {
		t.Error("vault should be unlocked after SetActiveVault")
	}
}

func TestLockVault(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	// First unlock
	state.SetActiveVault("test-vault", []byte("test-token"))

	// Then lock
	state.LockVault()

	if state.ActiveVault != "" {
		t.Errorf("expected empty ActiveVault after lock, got '%s'", state.ActiveVault)
	}

	if state.VaultToken != nil {
		t.Errorf("expected nil VaultToken after lock, got '%v'", state.VaultToken)
	}

	if !state.IsLocked {
		t.Error("vault should be locked after LockVault")
	}
}

func TestGetVaultToken(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	// Initially should be nil
	token := state.GetVaultToken()
	if token != nil {
		t.Error("expected nil token for locked vault")
	}

	// Set token
	testToken := []byte("test-token")
	state.SetActiveVault("test", testToken)

	token = state.GetVaultToken()
	if string(token) != string(testToken) {
		t.Errorf("expected token '%s', got '%s'", string(testToken), string(token))
	}
}

func TestIsVaultLocked(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	// Should be locked initially
	if !state.IsVaultLocked() {
		t.Error("vault should be locked initially")
	}

	// Unlock
	state.SetActiveVault("test", []byte("token"))
	if state.IsVaultLocked() {
		t.Error("vault should be unlocked after SetActiveVault")
	}

	// Lock again
	state.LockVault()
	if !state.IsVaultLocked() {
		t.Error("vault should be locked after LockVault")
	}
}

func TestGetActiveVault(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	// Initially empty
	if state.GetActiveVault() != "" {
		t.Error("expected empty ActiveVault initially")
	}

	// Set vault
	vaultName := "my-vault"
	state.SetActiveVault(vaultName, []byte("token"))

	if state.GetActiveVault() != vaultName {
		t.Errorf("expected ActiveVault '%s', got '%s'", vaultName, state.GetActiveVault())
	}
}

func TestSetNetwork(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	// Set to testnet
	state.SetNetwork("testnet")

	if state.GetNetwork() != "testnet" {
		t.Errorf("expected network 'testnet', got '%s'", state.GetNetwork())
	}

	if state.GetServer() != "https://testnet.accumulatenetwork.io/v3" {
		t.Errorf("expected testnet server URL, got '%s'", state.GetServer())
	}
}

func TestGetNetwork(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	// Should return default network
	if state.GetNetwork() != "mainnet" {
		t.Errorf("expected default network 'mainnet', got '%s'", state.GetNetwork())
	}

	// Change network
	state.SetNetwork("devnet")
	if state.GetNetwork() != "devnet" {
		t.Errorf("expected network 'devnet', got '%s'", state.GetNetwork())
	}
}

func TestGetServer(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	// Should return default server
	if state.GetServer() != "https://mainnet.accumulatenetwork.io/v3" {
		t.Errorf("expected mainnet server, got '%s'", state.GetServer())
	}

	// Change network
	state.SetNetwork("devnet")
	if state.GetServer() != "http://127.0.0.1:26660/v2" {
		t.Errorf("expected devnet server, got '%s'", state.GetServer())
	}
}

func TestGetWalletDir(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WalletDir = "/test/wallet"
	state := NewState(cfg)

	if state.GetWalletDir() != "/test/wallet" {
		t.Errorf("expected WalletDir '/test/wallet', got '%s'", state.GetWalletDir())
	}
}

// Test thread safety with concurrent access
func TestConcurrentVaultOperations(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent locks and unlocks
	for i := 0; i < iterations; i++ {
		wg.Add(2)

		go func(n int) {
			defer wg.Done()
			vaultName := "vault"
			token := []byte("token")
			state.SetActiveVault(vaultName, token)
		}(i)

		go func(n int) {
			defer wg.Done()
			state.LockVault()
		}(i)
	}

	wg.Wait()

	// Should not panic - final state doesn't matter, just that it's thread-safe
	t.Log("Concurrent operations completed without panic")
}

func TestConcurrentReads(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	// Set some initial state
	state.SetActiveVault("test-vault", []byte("test-token"))
	state.SetNetwork("testnet")

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(5)

		go func() {
			defer wg.Done()
			_ = state.GetActiveVault()
		}()

		go func() {
			defer wg.Done()
			_ = state.GetVaultToken()
		}()

		go func() {
			defer wg.Done()
			_ = state.IsVaultLocked()
		}()

		go func() {
			defer wg.Done()
			_ = state.GetNetwork()
		}()

		go func() {
			defer wg.Done()
			_ = state.GetServer()
		}()
	}

	wg.Wait()
	t.Log("Concurrent reads completed without panic")
}

func TestConcurrentNetworkChanges(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	var wg sync.WaitGroup
	networks := []string{"mainnet", "testnet", "devnet"}

	// Concurrent network changes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			network := networks[n%len(networks)]
			state.SetNetwork(network)
			_ = state.GetNetwork()
			_ = state.GetServer()
		}(i)
	}

	wg.Wait()
	t.Log("Concurrent network changes completed without panic")
}

func TestStateLockUnlockCycle(t *testing.T) {
	cfg := DefaultConfig()
	state := NewState(cfg)

	// Multiple lock/unlock cycles
	for i := 0; i < 10; i++ {
		// Should be locked
		if !state.IsVaultLocked() {
			t.Errorf("iteration %d: expected vault to be locked", i)
		}

		// Unlock
		state.SetActiveVault("vault", []byte("token"))
		if state.IsVaultLocked() {
			t.Errorf("iteration %d: expected vault to be unlocked", i)
		}

		// Lock
		state.LockVault()
		if !state.IsVaultLocked() {
			t.Errorf("iteration %d: expected vault to be locked after LockVault", i)
		}
	}
}
