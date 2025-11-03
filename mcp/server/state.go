package server

import (
	"sync"
)

// State holds the runtime state of the MCP server
type State struct {
	mu sync.RWMutex

	// Config is the current configuration
	Config *Config

	// ActiveVault is the name of the currently unlocked vault
	ActiveVault string

	// VaultToken is the authentication token for the active vault
	VaultToken []byte

	// IsLocked indicates whether the vault is currently locked
	IsLocked bool
}

// NewState creates a new state instance
func NewState(cfg *Config) *State {
	return &State{
		Config:   cfg,
		IsLocked: true,
	}
}

// SetActiveVault sets the active vault and token
func (s *State) SetActiveVault(vaultName string, token []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ActiveVault = vaultName
	s.VaultToken = token
	s.IsLocked = false
}

// LockVault locks the current vault
func (s *State) LockVault() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ActiveVault = ""
	s.VaultToken = nil
	s.IsLocked = true
}

// GetVaultToken returns the current vault token (if unlocked)
func (s *State) GetVaultToken() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.VaultToken
}

// IsVaultLocked returns whether the vault is locked
func (s *State) IsVaultLocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.IsLocked
}

// GetActiveVault returns the name of the active vault
func (s *State) GetActiveVault() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.ActiveVault
}

// SetNetwork updates the network configuration
func (s *State) SetNetwork(network string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Config.SetNetwork(network)
}

// GetNetwork returns the current network
func (s *State) GetNetwork() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Config.Network
}

// GetServer returns the current API server URL
func (s *State) GetServer() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Config.Server
}

// GetWalletDir returns the wallet directory
func (s *State) GetWalletDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Config.WalletDir
}
