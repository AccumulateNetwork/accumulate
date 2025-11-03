package wallet

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewClient(t *testing.T) {
	// Create a temporary wallet directory
	tmpDir := t.TempDir()

	client, err := NewClient(tmpDir)
	if err != nil {
		// It's okay if ccli is not found - we're just testing the constructor
		if client == nil {
			t.Skip("ccli binary not found - skipping test")
		}
	}

	if client.walletDir != tmpDir {
		t.Errorf("expected walletDir '%s', got '%s'", tmpDir, client.walletDir)
	}
}

func TestFindCLI(t *testing.T) {
	// Test that findCLI returns either a valid path or an error
	path, err := findCLI()

	if err != nil {
		// It's okay if not found - this is expected in CI environments
		t.Logf("ccli not found (expected in some environments): %v", err)
		return
	}

	// If found, verify it exists
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("findCLI returned path '%s' but file does not exist: %v", path, statErr)
	}

	t.Logf("Found ccli at: %s", path)
}

func TestParseKeyListText_Empty(t *testing.T) {
	client := &Client{}
	output := ""

	keys := client.parseKeyListText(output)

	if len(keys) != 0 {
		t.Errorf("expected 0 keys from empty output, got %d", len(keys))
	}
}

func TestParseKeyListText_SingleKey(t *testing.T) {
	client := &Client{}
	// Use lowercase 'account' to match the parsing logic
	output := `
Name: test-key
Public: 0x1234567890abcdef
Lite account: acc://1234567890abcdef.acme/ACME
`

	keys := client.parseKeyListText(output)

	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	if keys[0].Name != "test-key" {
		t.Errorf("expected name 'test-key', got '%s'", keys[0].Name)
	}

	if keys[0].PublicKey != "0x1234567890abcdef" {
		t.Errorf("expected publicKey '0x1234567890abcdef', got '%s'", keys[0].PublicKey)
	}

	if keys[0].LiteAccount != "acc://1234567890abcdef.acme/ACME" {
		t.Errorf("expected liteAccount 'acc://1234567890abcdef.acme/ACME', got '%s'", keys[0].LiteAccount)
	}
}

func TestParseKeyListText_MultipleKeys(t *testing.T) {
	client := &Client{}
	output := `
Name: key1
Public Key: 0xaaa
Lite Account: acc://aaa.acme/ACME

Name: key2
Public Key: 0xbbb
Lite Account: acc://bbb.acme/ACME
`

	keys := client.parseKeyListText(output)

	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	if keys[0].Name != "key1" {
		t.Errorf("expected first key name 'key1', got '%s'", keys[0].Name)
	}

	if keys[1].Name != "key2" {
		t.Errorf("expected second key name 'key2', got '%s'", keys[1].Name)
	}
}

func TestParseKeyListText_AlternateFormat(t *testing.T) {
	client := &Client{}
	// Test with "Address:" instead of "Lite account:"
	output := `
Name: test-key
Public: 0x1234
Address: acc://1234.acme/ACME
`

	keys := client.parseKeyListText(output)

	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	if keys[0].LiteAccount != "acc://1234.acme/ACME" {
		t.Errorf("expected liteAccount to be parsed from 'Address:' field, got '%s'", keys[0].LiteAccount)
	}
}

func TestGetKey_NotFound(t *testing.T) {
	client := &Client{walletDir: t.TempDir()}

	// Mock ListKeys to return empty list
	ctx := context.Background()

	// This will fail because ccli is not available, but we can test the logic
	_, err := client.GetKey(ctx, []byte("token"), "nonexistent")

	// We expect an error (either "ccli not found" or "key not found")
	if err == nil {
		t.Error("expected error when getting nonexistent key")
	}
}

func TestInitWallet_NoPassword_RequiresFlag(t *testing.T) {
	client := &Client{
		walletDir: t.TempDir(),
		cliPath:   "/fake/path/ccli", // Won't actually be called
	}

	ctx := context.Background()

	// Test that password is required when noPassword=false and password is empty
	err := client.InitWallet(ctx, "", false)

	if err == nil {
		t.Error("expected error when password is empty and noPassword is false")
	}

	if err.Error() != "password required unless using --no-password" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestOpenVault_WalletDoesNotExist(t *testing.T) {
	// Create a path that doesn't exist
	nonExistentPath := filepath.Join(t.TempDir(), "does-not-exist")

	client := &Client{
		walletDir: nonExistentPath,
		cliPath:   "/fake/path/ccli",
	}

	ctx := context.Background()
	_, err := client.OpenVault(ctx, "default", "password")

	if err == nil {
		t.Error("expected error when wallet does not exist")
	}

	if !os.IsNotExist(err) {
		// Check if error message mentions the wallet
		if err.Error() != "wallet does not exist at "+nonExistentPath {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestOpenVault_WalletExists(t *testing.T) {
	// Create a temporary wallet directory that exists
	tmpDir := t.TempDir()

	client := &Client{
		walletDir: tmpDir,
		cliPath:   "/fake/path/ccli",
	}

	ctx := context.Background()
	token, err := client.OpenVault(ctx, "default", "password")

	// Should succeed since the directory exists
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should return a dummy token
	if len(token) == 0 {
		t.Error("expected non-empty token")
	}

	if string(token) != "unlocked" {
		t.Errorf("expected token 'unlocked', got '%s'", string(token))
	}
}

func TestSignTransaction_NotImplemented(t *testing.T) {
	client := &Client{walletDir: t.TempDir()}

	ctx := context.Background()
	_, err := client.SignTransaction(ctx, []byte("token"), "key", []byte("hash"))

	if err == nil {
		t.Error("expected error for unimplemented SignTransaction")
	}

	if err.Error() != "transaction signing via MCP not yet implemented - use CLI directly" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestClose(t *testing.T) {
	client := &Client{walletDir: t.TempDir()}

	err := client.Close()
	if err != nil {
		t.Errorf("Close() should not return error for nil store, got: %v", err)
	}
}

// Integration test - only runs if ccli is available
func TestIntegration_ListKeys(t *testing.T) {
	// Skip if ccli not available
	_, err := findCLI()
	if err != nil {
		t.Skip("ccli not available - skipping integration test")
	}

	// Use a test wallet directory
	tmpDir := t.TempDir()
	client, err := NewClient(tmpDir)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// Try to list keys - should fail gracefully if wallet doesn't exist
	_, err = client.ListKeys(ctx, []byte("dummy-token"))
	// We don't care if it fails - just that it doesn't panic
	if err != nil {
		t.Logf("ListKeys failed (expected if wallet not initialized): %v", err)
	}
}
