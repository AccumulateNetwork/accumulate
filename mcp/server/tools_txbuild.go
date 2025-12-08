// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// registerTxBuildTools registers transaction building tools
func (s *Server) registerTxBuildTools(client *jsonrpc.Client) {
	s.RegisterTool(
		Tool{
			Name:        "send_tokens",
			Description: "Send tokens from one account to another",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"from":       {Type: "string"},
					"to":         {Type: "string"},
					"amount":     {Type: "number"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"from", "to", "amount", "signer_key"},
			},
		},
		s.makeSendTokensHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "add_credits",
			Description: "Convert ACME tokens to credits",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"from":       {Type: "string"},
					"to":         {Type: "string"},
					"amount":     {Type: "number"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"from", "to", "amount", "signer_key"},
			},
		},
		s.makeAddCreditsHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "create_identity",
			Description: "Create a new ADI (Accumulate Digital Identifier)",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":        {Type: "string"},
					"public_key": {Type: "string"},
					"signer":     {Type: "string"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"url", "public_key", "signer", "signer_key"},
			},
		},
		s.makeCreateIdentityHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "create_token_account",
			Description: "Create a token account under an ADI",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":        {Type: "string"},
					"token_url":  {Type: "string"},
					"signer":     {Type: "string"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"url", "signer", "signer_key"},
			},
		},
		s.makeCreateTokenAccountHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "create_data_account",
			Description: "Create a data account under an ADI",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":        {Type: "string"},
					"signer":     {Type: "string"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"url", "signer", "signer_key"},
			},
		},
		s.makeCreateDataAccountHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "write_data",
			Description: "Write data to a data account",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"account":    {Type: "string"},
					"data":       {Type: "string"},
					"signer":     {Type: "string"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"account", "data", "signer", "signer_key"},
			},
		},
		s.makeWriteDataHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "create_key_book",
			Description: "Create a key book under an ADI",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":        {Type: "string"},
					"public_key": {Type: "string"},
					"signer":     {Type: "string"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"url", "public_key", "signer", "signer_key"},
			},
		},
		s.makeCreateKeyBookHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "create_key_page",
			Description: "Create a key page in a key book",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"book":       {Type: "string"},
					"keys":       {Type: "array", Items: &JSONSchema{Type: "string"}},
					"signer":     {Type: "string"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"book", "keys", "signer", "signer_key"},
			},
		},
		s.makeCreateKeyPageHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "update_key_page",
			Description: "Add, remove, or update keys in a key page",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"page":       {Type: "string"},
					"operation":  {Type: "string", Enum: []string{"add", "remove", "update"}},
					"key":        {Type: "string"},
					"new_key":    {Type: "string"},
					"signer":     {Type: "string"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"page", "operation", "key", "signer", "signer_key"},
			},
		},
		s.makeUpdateKeyPageHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "create_token",
			Description: "Create a new token issuer",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":        {Type: "string"},
					"symbol":     {Type: "string"},
					"precision":  {Type: "number"},
					"signer":     {Type: "string"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"url", "symbol", "signer", "signer_key"},
			},
		},
		s.makeCreateTokenHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "issue_tokens",
			Description: "Issue tokens from a token issuer",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"issuer":     {Type: "string"},
					"to":         {Type: "string"},
					"amount":     {Type: "number"},
					"signer":     {Type: "string"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"issuer", "to", "amount", "signer", "signer_key"},
			},
		},
		s.makeIssueTokensHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "burn_tokens",
			Description: "Burn tokens from a token account",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"account":    {Type: "string"},
					"amount":     {Type: "number"},
					"signer":     {Type: "string"},
					"signer_key": {Type: "string"},
				},
				Required: []string{"account", "amount", "signer", "signer_key"},
			},
		},
		s.makeBurnTokensHandler(client),
	)
}

// Helper to build, sign, and submit a transaction
func (s *Server) buildAndSubmit(client *jsonrpc.Client, principal *url.URL, body protocol.TransactionBody, signerURL *url.URL, signerKeyHash []byte) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get the signer account to determine the version
	signerResp, err := client.Query(ctx, signerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("query signer: %w", err)
	}

	var signerVersion uint64 = 1
	if rec, ok := signerResp.(*api.AccountRecord); ok {
		if page, ok := rec.Account.(*protocol.KeyPage); ok {
			signerVersion = page.Version
		}
	}

	// Build the transaction
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: principal,
		},
		Body: body,
	}

	// Get the public key for signing
	pubKey, err := s.keys.GetPublicKey(signerKeyHash)
	if err != nil {
		return nil, fmt.Errorf("get public key: %w", err)
	}

	// Create signature
	sig := &protocol.ED25519Signature{
		PublicKey:     pubKey,
		Signer:        signerURL,
		SignerVersion: signerVersion,
		Timestamp:     uint64(time.Now().UTC().UnixMicro()),
	}

	// Set initiator hash from signature metadata
	txn.Header.Initiator = [32]byte(sig.Metadata().Hash())

	// Get private key for signing
	privKey, err := s.keys.GetPrivateKey(signerKeyHash)
	if err != nil {
		return nil, fmt.Errorf("get private key: %w", err)
	}

	// Sign the transaction using the protocol's signing function
	protocol.SignED25519(sig, privKey, nil, txn.GetHash())

	// Build envelope
	env := &messaging.Envelope{
		Transaction: []*protocol.Transaction{txn},
		Signatures:  []protocol.Signature{sig},
	}

	// Submit
	submissions, err := client.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		return nil, fmt.Errorf("submit: %w", err)
	}

	// Format result
	result := map[string]any{
		"txid":        txn.ID().String(),
		"hash":        hex.EncodeToString(txn.GetHash()),
		"submissions": len(submissions),
	}

	if len(submissions) > 0 {
		result["success"] = submissions[0].Success
		if submissions[0].Status != nil {
			result["status"] = submissions[0].Status.Code.String()
		}
		if submissions[0].Message != "" {
			result["message"] = submissions[0].Message
		}
	}

	return result, nil
}

func (s *Server) makeSendTokensHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		fromStr, _ := args["from"].(string)
		toStr, _ := args["to"].(string)
		amount, _ := args["amount"].(float64)
		signerKeyHex, _ := args["signer_key"].(string)

		if fromStr == "" || toStr == "" || amount <= 0 || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("from, to, amount, and signer_key are required")
		}

		from, err := url.Parse(fromStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid from URL: %w", err)
		}

		to, err := url.Parse(toStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid to URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		// Convert amount to fixed point (assuming ACME with 8 decimals)
		amountBig := big.NewInt(int64(amount * 1e8))

		body := &protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: to, Amount: *amountBig},
			},
		}

		// For lite accounts, signer is the account itself
		signer := from

		result, err := s.buildAndSubmit(client, from, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeAddCreditsHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		fromStr, _ := args["from"].(string)
		toStr, _ := args["to"].(string)
		amount, _ := args["amount"].(float64)
		signerKeyHex, _ := args["signer_key"].(string)

		if fromStr == "" || toStr == "" || amount <= 0 || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("from, to, amount, and signer_key are required")
		}

		from, err := url.Parse(fromStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid from URL: %w", err)
		}

		to, err := url.Parse(toStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid to URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		// Get current oracle price from network status
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		netStatus, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("get network status: %w", err)
		}

		oracle := netStatus.Oracle.Price
		if oracle == 0 {
			return ToolCallResult{}, fmt.Errorf("oracle price not available")
		}

		// Convert ACME amount to fixed point
		amountBig := big.NewInt(int64(amount * 1e8))

		body := &protocol.AddCredits{
			Recipient: to,
			Amount:    *amountBig,
			Oracle:    oracle,
		}

		signer := from

		result, err := s.buildAndSubmit(client, from, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeCreateIdentityHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, _ := args["url"].(string)
		pubKeyHex, _ := args["public_key"].(string)
		signerStr, _ := args["signer"].(string)
		signerKeyHex, _ := args["signer_key"].(string)

		if urlStr == "" || pubKeyHex == "" || signerStr == "" || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("url, public_key, signer, and signer_key are required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		pubKey, err := hex.DecodeString(pubKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid public_key hex: %w", err)
		}

		signer, err := url.Parse(signerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		keyHash := sha256.Sum256(pubKey)
		body := &protocol.CreateIdentity{
			Url:         u,
			KeyHash:     keyHash[:],
			KeyBookUrl:  u.JoinPath("book"),
		}

		result, err := s.buildAndSubmit(client, signer, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeCreateTokenAccountHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, _ := args["url"].(string)
		tokenURLStr, _ := args["token_url"].(string)
		signerStr, _ := args["signer"].(string)
		signerKeyHex, _ := args["signer_key"].(string)

		if urlStr == "" || signerStr == "" || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("url, signer, and signer_key are required")
		}

		if tokenURLStr == "" {
			tokenURLStr = "acc://ACME"
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		tokenURL, err := url.Parse(tokenURLStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid token_url: %w", err)
		}

		signer, err := url.Parse(signerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		body := &protocol.CreateTokenAccount{
			Url:      u,
			TokenUrl: tokenURL,
		}

		// Principal is the identity
		principal := u.Identity()

		result, err := s.buildAndSubmit(client, principal, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeCreateDataAccountHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, _ := args["url"].(string)
		signerStr, _ := args["signer"].(string)
		signerKeyHex, _ := args["signer_key"].(string)

		if urlStr == "" || signerStr == "" || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("url, signer, and signer_key are required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		signer, err := url.Parse(signerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		body := &protocol.CreateDataAccount{
			Url: u,
		}

		principal := u.Identity()

		result, err := s.buildAndSubmit(client, principal, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeWriteDataHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		accountStr, _ := args["account"].(string)
		dataStr, _ := args["data"].(string)
		signerStr, _ := args["signer"].(string)
		signerKeyHex, _ := args["signer_key"].(string)

		if accountStr == "" || dataStr == "" || signerStr == "" || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("account, data, signer, and signer_key are required")
		}

		account, err := url.Parse(accountStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid account URL: %w", err)
		}

		signer, err := url.Parse(signerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		// Data can be hex or plain text
		var dataBytes []byte
		if decoded, err := hex.DecodeString(dataStr); err == nil {
			dataBytes = decoded
		} else {
			dataBytes = []byte(dataStr)
		}

		body := &protocol.WriteData{
			Entry: &protocol.AccumulateDataEntry{
				Data: [][]byte{dataBytes},
			},
		}

		result, err := s.buildAndSubmit(client, account, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeCreateKeyBookHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, _ := args["url"].(string)
		pubKeyHex, _ := args["public_key"].(string)
		signerStr, _ := args["signer"].(string)
		signerKeyHex, _ := args["signer_key"].(string)

		if urlStr == "" || pubKeyHex == "" || signerStr == "" || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("url, public_key, signer, and signer_key are required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		pubKey, err := hex.DecodeString(pubKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid public_key hex: %w", err)
		}

		signer, err := url.Parse(signerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		keyHash := sha256.Sum256(pubKey)
		body := &protocol.CreateKeyBook{
			Url:         u,
			PublicKeyHash: keyHash[:],
		}

		principal := u.Identity()

		result, err := s.buildAndSubmit(client, principal, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeCreateKeyPageHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		bookStr, _ := args["book"].(string)
		signerStr, _ := args["signer"].(string)
		signerKeyHex, _ := args["signer_key"].(string)

		if bookStr == "" || signerStr == "" || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("book, signer, and signer_key are required")
		}

		book, err := url.Parse(bookStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid book URL: %w", err)
		}

		signer, err := url.Parse(signerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		// Parse keys array
		var keys []*protocol.KeySpecParams
		if keysRaw, ok := args["keys"].([]interface{}); ok {
			for _, k := range keysRaw {
				if keyHex, ok := k.(string); ok {
					keyBytes, err := hex.DecodeString(keyHex)
					if err != nil {
						return ToolCallResult{}, fmt.Errorf("invalid key hex: %w", err)
					}
					keyHash := sha256.Sum256(keyBytes)
					keys = append(keys, &protocol.KeySpecParams{
						KeyHash: keyHash[:],
					})
				}
			}
		}

		if len(keys) == 0 {
			return ToolCallResult{}, fmt.Errorf("at least one key is required")
		}

		body := &protocol.CreateKeyPage{
			Keys: keys,
		}

		result, err := s.buildAndSubmit(client, book, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeUpdateKeyPageHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		pageStr, _ := args["page"].(string)
		operation, _ := args["operation"].(string)
		keyHex, _ := args["key"].(string)
		signerStr, _ := args["signer"].(string)
		signerKeyHex, _ := args["signer_key"].(string)

		if pageStr == "" || operation == "" || keyHex == "" || signerStr == "" || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("page, operation, key, signer, and signer_key are required")
		}

		page, err := url.Parse(pageStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid page URL: %w", err)
		}

		signer, err := url.Parse(signerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		keyBytes, err := hex.DecodeString(keyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid key hex: %w", err)
		}
		keyHash := sha256.Sum256(keyBytes)

		body := &protocol.UpdateKeyPage{}

		switch operation {
		case "add":
			body.Operation = []protocol.KeyPageOperation{
				&protocol.AddKeyOperation{
					Entry: protocol.KeySpecParams{KeyHash: keyHash[:]},
				},
			}
		case "remove":
			body.Operation = []protocol.KeyPageOperation{
				&protocol.RemoveKeyOperation{
					Entry: protocol.KeySpecParams{KeyHash: keyHash[:]},
				},
			}
		case "update":
			newKeyHex, _ := args["new_key"].(string)
			if newKeyHex == "" {
				return ToolCallResult{}, fmt.Errorf("new_key is required for update operation")
			}
			newKeyBytes, err := hex.DecodeString(newKeyHex)
			if err != nil {
				return ToolCallResult{}, fmt.Errorf("invalid new_key hex: %w", err)
			}
			newKeyHash := sha256.Sum256(newKeyBytes)
			body.Operation = []protocol.KeyPageOperation{
				&protocol.UpdateKeyOperation{
					OldEntry: protocol.KeySpecParams{KeyHash: keyHash[:]},
					NewEntry: protocol.KeySpecParams{KeyHash: newKeyHash[:]},
				},
			}
		default:
			return ToolCallResult{}, fmt.Errorf("invalid operation: %s (must be add, remove, or update)", operation)
		}

		result, err := s.buildAndSubmit(client, page, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeCreateTokenHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, _ := args["url"].(string)
		symbol, _ := args["symbol"].(string)
		precision, _ := args["precision"].(float64)
		signerStr, _ := args["signer"].(string)
		signerKeyHex, _ := args["signer_key"].(string)

		if urlStr == "" || symbol == "" || signerStr == "" || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("url, symbol, signer, and signer_key are required")
		}

		if precision == 0 {
			precision = 8 // Default precision
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		signer, err := url.Parse(signerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		body := &protocol.CreateToken{
			Url:       u,
			Symbol:    symbol,
			Precision: uint64(precision),
		}

		principal := u.Identity()

		result, err := s.buildAndSubmit(client, principal, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeIssueTokensHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		issuerStr, _ := args["issuer"].(string)
		toStr, _ := args["to"].(string)
		amount, _ := args["amount"].(float64)
		signerStr, _ := args["signer"].(string)
		signerKeyHex, _ := args["signer_key"].(string)

		if issuerStr == "" || toStr == "" || amount <= 0 || signerStr == "" || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("issuer, to, amount, signer, and signer_key are required")
		}

		issuer, err := url.Parse(issuerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid issuer URL: %w", err)
		}

		to, err := url.Parse(toStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid to URL: %w", err)
		}

		signer, err := url.Parse(signerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		amountBig := big.NewInt(int64(amount * 1e8))

		body := &protocol.IssueTokens{
			To:     []*protocol.TokenRecipient{{Url: to, Amount: *amountBig}},
		}

		result, err := s.buildAndSubmit(client, issuer, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}

func (s *Server) makeBurnTokensHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		accountStr, _ := args["account"].(string)
		amount, _ := args["amount"].(float64)
		signerStr, _ := args["signer"].(string)
		signerKeyHex, _ := args["signer_key"].(string)

		if accountStr == "" || amount <= 0 || signerStr == "" || signerKeyHex == "" {
			return ToolCallResult{}, fmt.Errorf("account, amount, signer, and signer_key are required")
		}

		account, err := url.Parse(accountStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid account URL: %w", err)
		}

		signer, err := url.Parse(signerStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer URL: %w", err)
		}

		signerKeyHash, err := hex.DecodeString(signerKeyHex)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid signer_key hex: %w", err)
		}

		amountBig := big.NewInt(int64(amount * 1e8))

		body := &protocol.BurnTokens{
			Amount: *amountBig,
		}

		result, err := s.buildAndSubmit(client, account, body, signer, signerKeyHash)
		if err != nil {
			return ToolCallResult{}, err
		}

		return marshalResult(result)
	}
}
