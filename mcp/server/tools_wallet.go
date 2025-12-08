// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"encoding/hex"
	"fmt"
)

// registerWalletTools registers wallet/key management tools
func (s *Server) registerWalletTools() {
	s.RegisterTool(
		Tool{
			Name:        "generate_key",
			Description: "Generate a new ED25519 signing key pair",
			InputSchema: JSONSchema{
				Type:       "object",
				Properties: map[string]JSONSchema{},
			},
		},
		s.handleGenerateKey,
	)

	s.RegisterTool(
		Tool{
			Name:        "import_key",
			Description: "Import an existing ED25519 private key",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"private_key": {Type: "string"},
				},
				Required: []string{"private_key"},
			},
		},
		s.handleImportKey,
	)

	s.RegisterTool(
		Tool{
			Name:        "list_keys",
			Description: "List all keys currently in memory",
			InputSchema: JSONSchema{
				Type:       "object",
				Properties: map[string]JSONSchema{},
			},
		},
		s.handleListKeys,
	)

	s.RegisterTool(
		Tool{
			Name:        "get_lite_address",
			Description: "Compute lite token account address from a public key",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"public_key": {Type: "string"},
					"token_url":  {Type: "string"},
				},
				Required: []string{"public_key"},
			},
		},
		s.handleGetLiteAddress,
	)

	s.RegisterTool(
		Tool{
			Name:        "get_lite_identity",
			Description: "Compute lite identity URL from a public key",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"public_key": {Type: "string"},
				},
				Required: []string{"public_key"},
			},
		},
		s.handleGetLiteIdentity,
	)
}

func (s *Server) handleGenerateKey(args map[string]any) (ToolCallResult, error) {
	pubKey, pubKeyHash, err := s.keys.GenerateKey()
	if err != nil {
		return ToolCallResult{}, fmt.Errorf("generate key: %w", err)
	}

	liteAddr, _ := LiteAddress(pubKey, "acc://ACME")
	liteId := LiteIdentity(pubKey)

	result := map[string]any{
		"public_key":      hex.EncodeToString(pubKey),
		"public_key_hash": hex.EncodeToString(pubKeyHash),
		"lite_identity":   liteId.String(),
		"lite_acme":       liteAddr.String(),
	}

	return marshalResult(result)
}

func (s *Server) handleImportKey(args map[string]any) (ToolCallResult, error) {
	privateKeyHex, ok := args["private_key"].(string)
	if !ok || privateKeyHex == "" {
		return ToolCallResult{}, fmt.Errorf("private_key is required")
	}

	pubKey, pubKeyHash, err := s.keys.ImportKey(privateKeyHex)
	if err != nil {
		return ToolCallResult{}, err
	}

	liteAddr, _ := LiteAddress(pubKey, "acc://ACME")
	liteId := LiteIdentity(pubKey)

	result := map[string]any{
		"public_key":      hex.EncodeToString(pubKey),
		"public_key_hash": hex.EncodeToString(pubKeyHash),
		"lite_identity":   liteId.String(),
		"lite_acme":       liteAddr.String(),
		"imported":        true,
	}

	return marshalResult(result)
}

func (s *Server) handleListKeys(args map[string]any) (ToolCallResult, error) {
	keys := s.keys.ListKeys()

	result := map[string]any{
		"count": len(keys),
		"keys":  keys,
	}

	return marshalResult(result)
}

func (s *Server) handleGetLiteAddress(args map[string]any) (ToolCallResult, error) {
	pubKeyHex, ok := args["public_key"].(string)
	if !ok || pubKeyHex == "" {
		return ToolCallResult{}, fmt.Errorf("public_key is required")
	}

	pubKey, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return ToolCallResult{}, fmt.Errorf("invalid public key hex: %w", err)
	}

	tokenURL := "acc://ACME"
	if t, ok := args["token_url"].(string); ok && t != "" {
		tokenURL = t
	}

	liteAddr, err := LiteAddress(pubKey, tokenURL)
	if err != nil {
		return ToolCallResult{}, err
	}

	result := map[string]any{
		"lite_address": liteAddr.String(),
		"token_url":    tokenURL,
	}

	return marshalResult(result)
}

func (s *Server) handleGetLiteIdentity(args map[string]any) (ToolCallResult, error) {
	pubKeyHex, ok := args["public_key"].(string)
	if !ok || pubKeyHex == "" {
		return ToolCallResult{}, fmt.Errorf("public_key is required")
	}

	pubKey, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return ToolCallResult{}, fmt.Errorf("invalid public key hex: %w", err)
	}

	liteId := LiteIdentity(pubKey)

	result := map[string]any{
		"lite_identity": liteId.String(),
	}

	return marshalResult(result)
}
