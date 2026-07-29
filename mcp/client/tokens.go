package client

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// CreateToken creates a new token type on the Accumulate network
func (c *Client) CreateToken(ctx context.Context, tokenURL, signerURL string, privateKeyHex string, symbol string, precision uint64, properties map[string]interface{}) ([]byte, error) {
	// Decode private key
	if len(privateKeyHex) > 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	privateKey := ed25519.PrivateKey(privateKeyBytes)
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privateKey))
	}

	// Parse URLs
	tokenUrl, err := accurl.Parse(tokenURL)
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %w", err)
	}

	signerUrl, err := accurl.Parse(signerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid signer URL: %w", err)
	}

	// Create transaction body
	body := &protocol.CreateToken{
		Url:       tokenUrl,
		Symbol:    symbol,
		Precision: precision,
	}

	// Add optional properties
	if supplyLimit, ok := properties["supply_limit"].(int64); ok && supplyLimit > 0 {
		body.SupplyLimit = big.NewInt(supplyLimit)
	}

	// Create transaction
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: tokenUrl.RootIdentity(),
		},
		Body: body,
	}

	// Create signature
	sig := &protocol.ED25519Signature{
		PublicKey: privateKey.Public().(ed25519.PublicKey),
		Signer:    signerUrl,
		Timestamp: uint64(time.Now().UnixMilli()),
	}

	// Sign the transaction
	txnHash := txn.GetHash()
	sig.Signature = ed25519.Sign(privateKey, txnHash[:])

	// Create envelope
	envelope := &messaging.Envelope{
		Transaction: []*protocol.Transaction{txn},
		Signatures:  []protocol.Signature{sig},
	}

	// Submit using SDK
	submissions, err := c.client.Submit(ctx, envelope, api.SubmitOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to submit transaction: %w", err)
	}

	if len(submissions) == 0 {
		return nil, fmt.Errorf("no submission result returned")
	}

	return txnHash[:], nil
}

// IssueTokens mints new tokens to a recipient account
func (c *Client) IssueTokens(ctx context.Context, tokenURL, recipientURL, signerURL string, privateKeyHex string, amount int64) ([]byte, error) {
	// Decode private key
	if len(privateKeyHex) > 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	privateKey := ed25519.PrivateKey(privateKeyBytes)
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privateKey))
	}

	// Parse URLs
	tokenUrl, err := accurl.Parse(tokenURL)
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %w", err)
	}

	recipientUrl, err := accurl.Parse(recipientURL)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient URL: %w", err)
	}

	signerUrl, err := accurl.Parse(signerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid signer URL: %w", err)
	}

	// Create transaction body
	body := &protocol.IssueTokens{
		Recipient: recipientUrl,
		Amount:    *big.NewInt(amount),
	}

	// Create transaction
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: tokenUrl,
		},
		Body: body,
	}

	// Create signature
	sig := &protocol.ED25519Signature{
		PublicKey: privateKey.Public().(ed25519.PublicKey),
		Signer:    signerUrl,
		Timestamp: uint64(time.Now().UnixMilli()),
	}

	// Sign the transaction
	txnHash := txn.GetHash()
	sig.Signature = ed25519.Sign(privateKey, txnHash[:])

	// Create envelope
	envelope := &messaging.Envelope{
		Transaction: []*protocol.Transaction{txn},
		Signatures:  []protocol.Signature{sig},
	}

	// Submit using SDK
	submissions, err := c.client.Submit(ctx, envelope, api.SubmitOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to submit transaction: %w", err)
	}

	if len(submissions) == 0 {
		return nil, fmt.Errorf("no submission result returned")
	}

	return txnHash[:], nil
}

// BurnTokens destroys tokens from an account
func (c *Client) BurnTokens(ctx context.Context, accountURL, signerURL string, privateKeyHex string, amount int64) ([]byte, error) {
	// Decode private key
	if len(privateKeyHex) > 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	privateKey := ed25519.PrivateKey(privateKeyBytes)
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privateKey))
	}

	// Parse URLs
	accountUrl, err := accurl.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	signerUrl, err := accurl.Parse(signerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid signer URL: %w", err)
	}

	// Create transaction body
	body := &protocol.BurnTokens{
		Amount: *big.NewInt(amount),
	}

	// Create transaction
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: accountUrl,
		},
		Body: body,
	}

	// Create signature
	sig := &protocol.ED25519Signature{
		PublicKey: privateKey.Public().(ed25519.PublicKey),
		Signer:    signerUrl,
		Timestamp: uint64(time.Now().UnixMilli()),
	}

	// Sign the transaction
	txnHash := txn.GetHash()
	sig.Signature = ed25519.Sign(privateKey, txnHash[:])

	// Create envelope
	envelope := &messaging.Envelope{
		Transaction: []*protocol.Transaction{txn},
		Signatures:  []protocol.Signature{sig},
	}

	// Submit using SDK
	submissions, err := c.client.Submit(ctx, envelope, api.SubmitOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to submit transaction: %w", err)
	}

	if len(submissions) == 0 {
		return nil, fmt.Errorf("no submission result returned")
	}

	return txnHash[:], nil
}
