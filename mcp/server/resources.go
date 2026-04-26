package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// handleListResources returns the list of available resources
func (s *Server) handleListResources(id interface{}) map[string]interface{} {
	resources := []map[string]interface{}{
		{
			"uri":         "wallet://config",
			"name":        "Wallet Configuration",
			"description": "Current wallet and network configuration",
			"mimeType":    "application/json",
		},
		{
			"uri":         "wallet://state",
			"name":        "Wallet State",
			"description": "Current wallet state including vault status",
			"mimeType":    "application/json",
		},
		{
			"uri":         "wallet://keys",
			"name":        "Wallet Keys",
			"description": "List of keys in the wallet (requires unlocked vault)",
			"mimeType":    "application/json",
		},
	}

	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"resources": resources,
		},
	}
}

// handleReadResource reads a specific resource
func (s *Server) handleReadResource(id interface{}, request map[string]interface{}) map[string]interface{} {
	params, ok := request["params"].(map[string]interface{})
	if !ok {
		return s.errorResponse(request, -32602, "Invalid params")
	}

	uri, ok := params["uri"].(string)
	if !ok {
		return s.errorResponse(request, -32602, "Missing resource URI")
	}

	content, mimeType, err := s.readResource(uri)
	if err != nil {
		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"error": map[string]interface{}{
				"code":    -32603,
				"message": fmt.Sprintf("Failed to read resource: %v", err),
			},
		}
	}

	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      uri,
					"mimeType": mimeType,
					"text":     content,
				},
			},
		},
	}
}

// readResource reads the content of a resource
func (s *Server) readResource(uri string) (string, string, error) {
	switch {
	case uri == "wallet://config":
		return s.getConfigResource()
	case uri == "wallet://state":
		return s.getStateResource()
	case uri == "wallet://keys":
		return s.getKeysResource()
	default:
		return "", "", fmt.Errorf("unknown resource: %s", uri)
	}
}

// getConfigResource returns the wallet configuration
func (s *Server) getConfigResource() (string, string, error) {
	config := map[string]interface{}{
		"walletDir": s.state.GetWalletDir(),
		"network":   s.state.GetNetwork(),
		"server":    s.state.GetServer(),
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", "", err
	}

	return string(data), "application/json", nil
}

// getStateResource returns the wallet state
func (s *Server) getStateResource() (string, string, error) {
	state := map[string]interface{}{
		"walletDir":   s.state.GetWalletDir(),
		"network":     s.state.GetNetwork(),
		"server":      s.state.GetServer(),
		"vaultLocked": s.state.IsVaultLocked(),
		"activeVault": s.state.GetActiveVault(),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", "", err
	}

	return string(data), "application/json", nil
}

// getKeysResource returns the list of keys in the wallet
func (s *Server) getKeysResource() (string, string, error) {
	if s.state.IsVaultLocked() {
		return "", "", fmt.Errorf("vault is locked - please unlock vault first")
	}

	if s.wallet == nil {
		return "", "", fmt.Errorf("wallet client not initialized")
	}

	ctx := context.Background()
	keys, err := s.wallet.ListKeys(ctx, s.state.GetVaultToken())
	if err != nil {
		return "", "", fmt.Errorf("failed to list keys: %w", err)
	}

	// Convert to simple format
	keyList := make([]map[string]interface{}, len(keys))
	for i, key := range keys {
		keyList[i] = map[string]interface{}{
			"name":        key.Name,
			"publicKey":   key.PublicKey,
			"liteAccount": key.LiteAccount,
		}
	}

	result := map[string]interface{}{
		"keys":  keyList,
		"count": len(keys),
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", "", err
	}

	return string(data), "application/json", nil
}

// Helper function to check if URI matches a pattern
func matchURI(uri, pattern string) (map[string]string, bool) {
	// Simple pattern matching for URIs like wallet://account/{url}
	if !strings.Contains(pattern, "{") {
		return nil, uri == pattern
	}

	// Extract variable parts
	params := make(map[string]string)
	uriParts := strings.Split(uri, "/")
	patternParts := strings.Split(pattern, "/")

	if len(uriParts) != len(patternParts) {
		return nil, false
	}

	for i := range uriParts {
		if strings.HasPrefix(patternParts[i], "{") && strings.HasSuffix(patternParts[i], "}") {
			paramName := strings.Trim(patternParts[i], "{}")
			params[paramName] = uriParts[i]
		} else if uriParts[i] != patternParts[i] {
			return nil, false
		}
	}

	return params, true
}
