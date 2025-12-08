// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"encoding/json"
	"fmt"
)

// Resource describes an MCP resource
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceHandler is a function that handles a resource read
type ResourceHandler func() (ResourceContent, error)

// ResourceContent is the content of a resource
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// ResourcesListResult is returned from resources/list
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// ResourceReadParams is the params for resources/read
type ResourceReadParams struct {
	URI string `json:"uri"`
}

// ResourceReadResult is returned from resources/read
type ResourceReadResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ResourcesCapability describes the resources capability
type ResourcesCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// RegisterResource registers a resource with its handler
func (s *Server) RegisterResource(resource Resource, handler ResourceHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resources == nil {
		s.resources = make(map[string]Resource)
		s.resourceHandlers = make(map[string]ResourceHandler)
	}
	s.resources[resource.URI] = resource
	s.resourceHandlers[resource.URI] = handler
}

// handleResourcesList handles resources/list
func (s *Server) handleResourcesList() (any, *JSONRPCError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resources := make([]Resource, 0, len(s.resources))
	for _, resource := range s.resources {
		resources = append(resources, resource)
	}

	return ResourcesListResult{Resources: resources}, nil
}

// handleResourcesRead handles resources/read
func (s *Server) handleResourcesRead(params json.RawMessage) (any, *JSONRPCError) {
	var readParams ResourceReadParams
	if err := json.Unmarshal(params, &readParams); err != nil {
		return nil, &JSONRPCError{
			Code:    InvalidParams,
			Message: "Invalid params",
			Data:    err.Error(),
		}
	}

	s.mu.RLock()
	handler, ok := s.resourceHandlers[readParams.URI]
	s.mu.RUnlock()

	if !ok {
		return nil, &JSONRPCError{
			Code:    InvalidParams,
			Message: "Resource not found",
			Data:    readParams.URI,
		}
	}

	content, err := handler()
	if err != nil {
		return NewErrorResult(err), nil
	}

	return ResourceReadResult{Contents: []ResourceContent{content}}, nil
}

// registerResources registers the available resources
func (s *Server) registerResources() {
	s.RegisterResource(
		Resource{
			URI:         "accumulate://protocol/transaction-types",
			Name:        "Transaction Types",
			Description: "List of all supported transaction types in the Accumulate protocol",
			MimeType:    "application/json",
		},
		s.handleTransactionTypesResource,
	)

	s.RegisterResource(
		Resource{
			URI:         "accumulate://protocol/account-types",
			Name:        "Account Types",
			Description: "List of all supported account types in the Accumulate protocol",
			MimeType:    "application/json",
		},
		s.handleAccountTypesResource,
	)

	s.RegisterResource(
		Resource{
			URI:         "accumulate://protocol/signature-types",
			Name:        "Signature Types",
			Description: "List of all supported signature types in the Accumulate protocol",
			MimeType:    "application/json",
		},
		s.handleSignatureTypesResource,
	)
}

func (s *Server) handleTransactionTypesResource() (ResourceContent, error) {
	types := []map[string]any{
		{"name": "SendTokens", "description": "Transfer tokens between accounts"},
		{"name": "AddCredits", "description": "Convert ACME tokens to credits"},
		{"name": "BurnTokens", "description": "Burn tokens from a token account"},
		{"name": "CreateIdentity", "description": "Create a new ADI (Accumulate Digital Identifier)"},
		{"name": "CreateTokenAccount", "description": "Create a token account under an ADI"},
		{"name": "CreateDataAccount", "description": "Create a data account under an ADI"},
		{"name": "WriteData", "description": "Write data to a data account"},
		{"name": "CreateKeyBook", "description": "Create a key book for authorization"},
		{"name": "CreateKeyPage", "description": "Create a key page in a key book"},
		{"name": "UpdateKeyPage", "description": "Add, remove, or update keys in a key page"},
		{"name": "CreateToken", "description": "Create a new token issuer"},
		{"name": "IssueTokens", "description": "Issue tokens from a token issuer"},
		{"name": "LockAccount", "description": "Lock an account to prevent modifications"},
		{"name": "UpdateAccountAuth", "description": "Update the authorization rules for an account"},
		{"name": "BurnCredits", "description": "Burn credits from an account"},
	}

	data, err := json.MarshalIndent(types, "", "  ")
	if err != nil {
		return ResourceContent{}, fmt.Errorf("marshal: %w", err)
	}

	return ResourceContent{
		URI:      "accumulate://protocol/transaction-types",
		MimeType: "application/json",
		Text:     string(data),
	}, nil
}

func (s *Server) handleAccountTypesResource() (ResourceContent, error) {
	types := []map[string]any{
		{"name": "LiteTokenAccount", "description": "Lightweight token account derived from a public key"},
		{"name": "LiteIdentity", "description": "Lightweight identity derived from a public key"},
		{"name": "LiteDataAccount", "description": "Lightweight data account for storing data"},
		{"name": "ADI", "description": "Accumulate Digital Identifier - a named identity"},
		{"name": "TokenAccount", "description": "Token account under an ADI"},
		{"name": "DataAccount", "description": "Data account under an ADI for storing data"},
		{"name": "KeyBook", "description": "Key book for managing authorization"},
		{"name": "KeyPage", "description": "Key page containing authorized keys"},
		{"name": "TokenIssuer", "description": "Token issuer for creating custom tokens"},
	}

	data, err := json.MarshalIndent(types, "", "  ")
	if err != nil {
		return ResourceContent{}, fmt.Errorf("marshal: %w", err)
	}

	return ResourceContent{
		URI:      "accumulate://protocol/account-types",
		MimeType: "application/json",
		Text:     string(data),
	}, nil
}

func (s *Server) handleSignatureTypesResource() (ResourceContent, error) {
	types := []map[string]any{
		{"name": "ED25519", "description": "Standard ED25519 signature"},
		{"name": "RCD1", "description": "Factom RCD1 signature type"},
		{"name": "BTC", "description": "Bitcoin-compatible ECDSA signature"},
		{"name": "BTCLegacy", "description": "Legacy Bitcoin ECDSA signature"},
		{"name": "ETH", "description": "Ethereum-compatible ECDSA signature"},
		{"name": "Delegated", "description": "Signature delegated to another authority"},
		{"name": "Authority", "description": "Authority signature (multi-sig threshold)"},
	}

	data, err := json.MarshalIndent(types, "", "  ")
	if err != nil {
		return ResourceContent{}, fmt.Errorf("marshal: %w", err)
	}

	return ResourceContent{
		URI:      "accumulate://protocol/signature-types",
		MimeType: "application/json",
		Text:     string(data),
	}, nil
}
