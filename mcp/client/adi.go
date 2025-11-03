package client

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// GenerateKey generates a new ED25519 key pair
func GenerateKey() (publicKeyHex string, privateKeyHex string, liteAccountURL string, err error) {
	// Generate key pair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate key: %w", err)
	}

	// Convert to hex
	publicKeyHex = hex.EncodeToString(publicKey)
	privateKeyHex = hex.EncodeToString(privateKey)

	// Generate lite account URL
	liteUrl := protocol.LiteAuthorityForKey(publicKey, protocol.SignatureTypeED25519)
	liteAccountURL = liteUrl.JoinPath("ACME").String()

	return publicKeyHex, privateKeyHex, liteAccountURL, nil
}

// AddCredits adds credits to an account
func (c *Client) AddCredits(ctx context.Context, recipient, payer string, amount int64, privateKeyHex string) ([]byte, error) {
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
	recipientUrl, err := url.Parse(recipient)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient URL: %w", err)
	}

	payerUrl, err := url.Parse(payer)
	if err != nil {
		return nil, fmt.Errorf("invalid payer URL: %w", err)
	}

	// Get current oracle price from network
	status, err := c.client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get network status: %w", err)
	}

	oracle := status.Oracle.Price

	// Create AddCredits transaction
	body := &protocol.AddCredits{
		Recipient: recipientUrl,
		Amount:    *big.NewInt(amount),
		Oracle:    oracle,
	}

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: payerUrl,
		},
		Body: body,
	}

	// Create signature
	sig := &protocol.ED25519Signature{
		PublicKey: privateKey.Public().(ed25519.PublicKey),
		Signer:    payerUrl.RootIdentity(),
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

	// Return the transaction hash
	return txnHash[:], nil
}

// CreateIdentity creates a new ADI (Accumulate Digital Identifier)
func (c *Client) CreateIdentity(ctx context.Context, adiURL, publicKeyHex, sponsor string, privateKeyHex string) ([]byte, error) {
	// Decode keys
	if len(publicKeyHex) > 2 && publicKeyHex[:2] == "0x" {
		publicKeyHex = publicKeyHex[2:]
	}
	publicKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

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
	adiUrl, err := url.Parse(adiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid ADI URL: %w", err)
	}

	sponsorUrl, err := url.Parse(sponsor)
	if err != nil {
		return nil, fmt.Errorf("invalid sponsor URL: %w", err)
	}

	// Hash the public key
	keyHash := sha256.Sum256(publicKeyBytes)

	// Create CreateIdentity transaction
	body := &protocol.CreateIdentity{
		Url:     adiUrl,
		KeyHash: keyHash[:],
	}

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: sponsorUrl,
		},
		Body: body,
	}

	// Create signature
	sig := &protocol.ED25519Signature{
		PublicKey: privateKey.Public().(ed25519.PublicKey),
		Signer:    sponsorUrl.RootIdentity(),
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

	// Return the transaction hash
	return txnHash[:], nil
}
