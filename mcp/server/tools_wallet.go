package server

import (
	"context"
	"fmt"
)

// walletInit initializes a new wallet
func (s *Server) walletInit(args map[string]interface{}) (map[string]interface{}, error) {
	password, _ := args["password"].(string)
	noPassword, _ := args["no_password"].(bool)

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet client not initialized")
	}

	ctx := context.Background()
	err := s.wallet.InitWallet(ctx, password, noPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize wallet: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("✓ Wallet initialized successfully at %s", s.state.GetWalletDir()),
			},
		},
	}, nil
}

// vaultOpen opens and unlocks a vault
func (s *Server) vaultOpen(args map[string]interface{}) (map[string]interface{}, error) {
	vaultName, ok := args["vault"].(string)
	if !ok || vaultName == "" {
		vaultName = "default"
	}

	password, _ := args["password"].(string)

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet client not initialized")
	}

	ctx := context.Background()
	token, err := s.wallet.OpenVault(ctx, vaultName, password)
	if err != nil {
		return nil, fmt.Errorf("failed to open vault: %w", err)
	}

	// Update state
	s.state.SetActiveVault(vaultName, token)

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("✓ Vault '%s' unlocked successfully", vaultName),
			},
		},
	}, nil
}

// vaultLock locks the current vault
func (s *Server) vaultLock(args map[string]interface{}) (map[string]interface{}, error) {
	if s.state.IsVaultLocked() {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "Vault is already locked",
				},
			},
		}, nil
	}

	vaultName := s.state.GetActiveVault()
	s.state.LockVault()

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("✓ Vault '%s' locked successfully", vaultName),
			},
		},
	}, nil
}

// walletGenerateKey generates a new key in the wallet
func (s *Server) walletGenerateKey(args map[string]interface{}) (map[string]interface{}, error) {
	if s.state.IsVaultLocked() {
		return nil, fmt.Errorf("vault is locked - please unlock vault first")
	}

	keyName, ok := args["key_name"].(string)
	if !ok || keyName == "" {
		return nil, fmt.Errorf("key_name is required")
	}

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet client not initialized")
	}

	ctx := context.Background()
	publicKey, liteAccount, err := s.wallet.GenerateKey(ctx, s.state.GetVaultToken(), keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("✓ Key '%s' generated successfully\n\nPublic Key: %s\nLite Account: %s",
					keyName, publicKey, liteAccount),
			},
		},
	}, nil
}

// walletListKeys lists all keys in the wallet
func (s *Server) walletListKeys(args map[string]interface{}) (map[string]interface{}, error) {
	if s.state.IsVaultLocked() {
		return nil, fmt.Errorf("vault is locked - please unlock vault first")
	}

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet client not initialized")
	}

	ctx := context.Background()
	keys, err := s.wallet.ListKeys(ctx, s.state.GetVaultToken())
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	if len(keys) == 0 {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "No keys found in wallet",
				},
			},
		}, nil
	}

	// Format keys for display
	result := fmt.Sprintf("Keys in wallet (%d):\n\n", len(keys))
	for i, key := range keys {
		result += fmt.Sprintf("%d. %s\n", i+1, key.Name)
		result += fmt.Sprintf("   Public Key: %s\n", key.PublicKey)
		result += fmt.Sprintf("   Lite Account: %s\n\n", key.LiteAccount)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": result,
			},
		},
	}, nil
}

// walletSetNetwork sets the network for the wallet
func (s *Server) walletSetNetwork(args map[string]interface{}) (map[string]interface{}, error) {
	network, ok := args["network"].(string)
	if !ok || network == "" {
		return nil, fmt.Errorf("network is required (mainnet, testnet, devnet, or custom URL)")
	}

	s.state.SetNetwork(network)

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("✓ Network set to '%s'\nServer: %s",
					s.state.GetNetwork(), s.state.GetServer()),
			},
		},
	}, nil
}

// walletGetStatus gets the current wallet status
func (s *Server) walletGetStatus(args map[string]interface{}) (map[string]interface{}, error) {
	status := fmt.Sprintf("Wallet Status:\n\n")
	status += fmt.Sprintf("Directory: %s\n", s.state.GetWalletDir())
	status += fmt.Sprintf("Network: %s\n", s.state.GetNetwork())
	status += fmt.Sprintf("Server: %s\n", s.state.GetServer())
	status += fmt.Sprintf("Vault Locked: %t\n", s.state.IsVaultLocked())

	if !s.state.IsVaultLocked() {
		status += fmt.Sprintf("Active Vault: %s\n", s.state.GetActiveVault())
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": status,
			},
		},
	}, nil
}
