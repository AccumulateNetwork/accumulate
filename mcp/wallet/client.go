package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client wraps the wallet CLI functionality
type Client struct {
	walletDir string
	cliPath   string
}

// KeyInfo represents key information
type KeyInfo struct {
	Name        string `json:"name"`
	PublicKey   string `json:"publicKey"`
	LiteAccount string `json:"liteAccount"`
}

// NewClient creates a new wallet client
func NewClient(walletDir string) (*Client, error) {
	// Find ccli binary
	cliPath, err := findCLI()
	if err != nil {
		return nil, fmt.Errorf("failed to find ccli binary: %w", err)
	}

	return &Client{
		walletDir: walletDir,
		cliPath:   cliPath,
	}, nil
}

// findCLI locates the ccli binary
func findCLI() (string, error) {
	// Check common locations
	paths := []string{
		"./ccli",
		"../wallet/ccli",
		"/usr/local/bin/ccli",
		filepath.Join(os.Getenv("HOME"), "go/bin/ccli"),
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Try PATH
	path, err := exec.LookPath("ccli")
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("ccli binary not found")
}

// runCommand executes a CLI command
func (c *Client) runCommand(ctx context.Context, args ...string) (string, error) {
	// Prepend wallet flag
	fullArgs := append([]string{"--wallet", c.walletDir}, args...)

	cmd := exec.CommandContext(ctx, c.cliPath, fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// InitWallet initializes a new wallet
func (c *Client) InitWallet(ctx context.Context, password string, noPassword bool) error {
	args := []string{"wallet", "init"}

	if noPassword {
		args = append(args, "--no-password")
	}

	// For password, we'd need interactive input or env var
	// For now, require --no-password for MCP usage
	if !noPassword && password == "" {
		return fmt.Errorf("password required unless using --no-password")
	}

	_, err := c.runCommand(ctx, args...)
	return err
}

// OpenVault opens and unlocks a vault
// Returns a dummy token since CLI doesn't expose tokens
func (c *Client) OpenVault(ctx context.Context, vaultName, password string) ([]byte, error) {
	// The CLI doesn't have explicit vault opening - it prompts for password
	// For now, we'll just verify the wallet exists
	if _, err := os.Stat(c.walletDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("wallet does not exist at %s", c.walletDir)
	}

	// Return a dummy token to indicate success
	// Real vault management would require daemon integration
	return []byte("unlocked"), nil
}

// GenerateKey generates a new key in the vault
func (c *Client) GenerateKey(ctx context.Context, token []byte, keyName string) (string, string, error) {
	// Use key generate command
	output, err := c.runCommand(ctx, "key", "generate", keyName)
	if err != nil {
		return "", "", err
	}

	// Parse output to extract public key and lite account
	// Output format: "Generated key: <name>\nPublic key: <pubkey>\nLite account: <account>"
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var pubKey, liteAccount string

	for _, line := range lines {
		if strings.Contains(line, "Public key:") || strings.Contains(line, "Public Key:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				pubKey = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "Lite account:") || strings.Contains(line, "Address:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				liteAccount = strings.TrimSpace(parts[1])
			}
		}
	}

	if pubKey == "" {
		return "", "", fmt.Errorf("failed to parse public key from output: %s", output)
	}

	return pubKey, liteAccount, nil
}

// ListKeys lists all keys in the vault
func (c *Client) ListKeys(ctx context.Context, token []byte) ([]KeyInfo, error) {
	output, err := c.runCommand(ctx, "key", "list", "--json")
	if err != nil {
		// Try without --json if not supported
		output, err = c.runCommand(ctx, "key", "list")
		if err != nil {
			return nil, err
		}

		// Parse text output
		return c.parseKeyListText(output), nil
	}

	// Parse JSON output
	var keys []KeyInfo
	if err := json.Unmarshal([]byte(output), &keys); err != nil {
		// If JSON parse fails, try text parsing
		return c.parseKeyListText(output), nil
	}

	return keys, nil
}

// parseKeyListText parses text-formatted key list output
func (c *Client) parseKeyListText(output string) []KeyInfo {
	var keys []KeyInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	var currentKey *KeyInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Extract key information first
		if strings.Contains(line, "Name:") {
			if currentKey != nil {
				keys = append(keys, *currentKey)
			}
			currentKey = &KeyInfo{}
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				currentKey.Name = strings.TrimSpace(parts[1])
			}
		}
		if currentKey != nil && strings.Contains(line, "Public") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				currentKey.PublicKey = strings.TrimSpace(parts[1])
			}
		}
		if currentKey != nil && (strings.Contains(line, "Lite") || strings.Contains(line, "Address")) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) >= 2 {
				// The value might contain colons (e.g., acc://...), so join everything after first colon
				currentKey.LiteAccount = strings.TrimSpace(parts[1])
			}
		}
	}

	if currentKey != nil {
		keys = append(keys, *currentKey)
	}

	return keys
}

// SignTransaction would sign a transaction with a key from the vault
// For now, this is a placeholder - actual signing would go through CLI
func (c *Client) SignTransaction(ctx context.Context, token []byte, keyName string, txHash []byte) ([]byte, error) {
	return nil, fmt.Errorf("transaction signing via MCP not yet implemented - use CLI directly")
}

// GetKey retrieves key information
func (c *Client) GetKey(ctx context.Context, token []byte, keyName string) (*KeyInfo, error) {
	keys, err := c.ListKeys(ctx, token)
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		if key.Name == keyName {
			return &key, nil
		}
	}

	return nil, fmt.Errorf("key %s not found", keyName)
}

// Close closes the wallet client (no-op for CLI-based client)
func (c *Client) Close() error {
	return nil
}
