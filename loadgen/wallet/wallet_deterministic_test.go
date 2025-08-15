package wallet

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeterministicLiteAccountGeneration tests that lite account generation is deterministic
func TestDeterministicLiteAccountGeneration(t *testing.T) {
	// Use a specific seed for testing
	testSeed := []byte("test-seed-for-deterministic-generation")
	
	// Create first wallet with the seed
	wallet1 := NewWalletWithSeed(testSeed)
	
	// Create 3 lite accounts in the first wallet
	accounts1 := make([]*LiteIdentity, 3)
	for i := 0; i < 3; i++ {
		account, err := wallet1.CreateLiteAccount()
		require.NoError(t, err)
		require.NotNil(t, account)
		accounts1[i] = account
		
		t.Logf("Wallet1 Account %d: %s", i+1, account.URL)
		t.Logf("  PublicKeyHash: %x", account.PublicKeyHash)
	}
	
	// Create second wallet with the same seed
	wallet2 := NewWalletWithSeed(testSeed)
	
	// Create 3 lite accounts in the second wallet
	accounts2 := make([]*LiteIdentity, 3)
	for i := 0; i < 3; i++ {
		account, err := wallet2.CreateLiteAccount()
		require.NoError(t, err)
		require.NotNil(t, account)
		accounts2[i] = account
		
		t.Logf("Wallet2 Account %d: %s", i+1, account.URL)
		t.Logf("  PublicKeyHash: %x", account.PublicKeyHash)
	}
	
	// Verify that the accounts match between the two wallets
	for i := 0; i < 3; i++ {
		// Check URLs match
		assert.Equal(t, accounts1[i].URL.String(), accounts2[i].URL.String(),
			"Account %d URLs should match", i+1)
		
		// Check public keys match
		assert.Equal(t, accounts1[i].Key.PublicKey, accounts2[i].Key.PublicKey,
			"Account %d public keys should match", i+1)
		
		// Check private keys match
		assert.Equal(t, accounts1[i].Key.PrivateKey, accounts2[i].Key.PrivateKey,
			"Account %d private keys should match", i+1)
		
		// Check public key hashes match
		assert.Equal(t, accounts1[i].PublicKeyHash, accounts2[i].PublicKeyHash,
			"Account %d public key hashes should match", i+1)
		
		// Check key types match
		assert.Equal(t, accounts1[i].Key.Type, accounts2[i].Key.Type,
			"Account %d key types should match", i+1)
	}
	
	t.Log("✓ All 3 accounts match between wallets with same seed")
}

// TestDifferentSeedsProduceDifferentAccounts tests that different seeds produce different accounts
func TestDifferentSeedsProduceDifferentAccounts(t *testing.T) {
	// Create two wallets with different seeds
	wallet1 := NewWalletWithSeed([]byte("seed-one"))
	wallet2 := NewWalletWithSeed([]byte("seed-two"))
	
	// Create one account in each wallet
	account1, err := wallet1.CreateLiteAccount()
	require.NoError(t, err)
	
	account2, err := wallet2.CreateLiteAccount()
	require.NoError(t, err)
	
	// Verify the accounts are different
	assert.NotEqual(t, account1.URL.String(), account2.URL.String(),
		"Different seeds should produce different URLs")
	assert.NotEqual(t, account1.Key.PublicKey, account2.Key.PublicKey,
		"Different seeds should produce different public keys")
	assert.NotEqual(t, account1.Key.PrivateKey, account2.Key.PrivateKey,
		"Different seeds should produce different private keys")
	
	t.Logf("Seed1 Account: %s", account1.URL)
	t.Logf("Seed2 Account: %s", account2.URL)
	t.Log("✓ Different seeds produce different accounts")
}

// TestSequentialAccountsAreDifferent tests that sequential accounts from same wallet are different
func TestSequentialAccountsAreDifferent(t *testing.T) {
	wallet := NewWalletWithSeed([]byte("test-seed"))
	
	// Create 3 accounts
	accounts := make([]*LiteIdentity, 3)
	for i := 0; i < 3; i++ {
		account, err := wallet.CreateLiteAccount()
		require.NoError(t, err)
		accounts[i] = account
	}
	
	// Verify all accounts are unique
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			assert.NotEqual(t, accounts[i].URL.String(), accounts[j].URL.String(),
				"Account %d and %d should have different URLs", i+1, j+1)
			assert.NotEqual(t, accounts[i].Key.PublicKey, accounts[j].Key.PublicKey,
				"Account %d and %d should have different public keys", i+1, j+1)
		}
	}
	
	t.Log("✓ Sequential accounts from same wallet are all different")
}

// TestWalletStoresCreatedAccounts tests that created accounts are properly stored in wallet
func TestWalletStoresCreatedAccounts(t *testing.T) {
	wallet := NewWalletWithSeed([]byte("storage-test"))
	
	// Create 3 accounts
	createdAccounts := make([]*LiteIdentity, 3)
	for i := 0; i < 3; i++ {
		account, err := wallet.CreateLiteAccount()
		require.NoError(t, err)
		createdAccounts[i] = account
	}
	
	// Verify accounts are stored and retrievable
	for i, account := range createdAccounts {
		// Check identity is stored
		retrieved := wallet.GetLiteIdentity(account.URL)
		require.NotNil(t, retrieved, "Account %d should be retrievable", i+1)
		assert.Equal(t, account.URL.String(), retrieved.URL.String())
		
		// Check key is stored
		keyHashHex := fmt.Sprintf("%x", account.Key.PublicKeyHash)
		retrievedKey := wallet.GetKey(keyHashHex)
		require.NotNil(t, retrievedKey, "Key for account %d should be retrievable", i+1)
		assert.Equal(t, account.Key.PublicKey, retrievedKey.PublicKey)
	}
	
	// Check GetAllLiteIdentities returns all accounts
	allIdentities := wallet.GetAllLiteIdentities()
	assert.Len(t, allIdentities, 3, "Should have 3 lite identities")
	
	t.Log("✓ All created accounts are properly stored in wallet")
}

// TestDefaultSeedBehavior tests wallet behavior with default seed
func TestDefaultSeedBehavior(t *testing.T) {
	// Create two wallets with no explicit seed (should use default)
	wallet1 := NewWallet()
	wallet2 := NewWallet()
	
	// Create an account in each
	account1, err := wallet1.CreateLiteAccount()
	require.NoError(t, err)
	
	account2, err := wallet2.CreateLiteAccount()
	require.NoError(t, err)
	
	// With default seed, both wallets should generate the same first account
	assert.Equal(t, account1.URL.String(), account2.URL.String(),
		"Wallets with default seed should generate same accounts")
	
	t.Logf("Default wallet account: %s", account1.URL)
	t.Log("✓ Default seed produces consistent accounts")
}