package client

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// CreateDataAccount creates a new data account under an ADI
func (c *Client) CreateDataAccount(ctx context.Context, accountURL, sponsor string, privateKeyHex string, authorities []string) ([]byte, error) {
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
	accountUrl, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	sponsorUrl, err := url.Parse(sponsor)
	if err != nil {
		return nil, fmt.Errorf("invalid sponsor URL: %w", err)
	}

	// Parse optional authorities
	var authUrls []*url.URL
	if len(authorities) > 0 {
		authUrls = make([]*url.URL, len(authorities))
		for i, auth := range authorities {
			authUrl, err := url.Parse(auth)
			if err != nil {
				return nil, fmt.Errorf("invalid authority URL %d: %w", i, err)
			}
			authUrls[i] = authUrl
		}
	}

	// Create CreateDataAccount transaction
	body := &protocol.CreateDataAccount{
		Url:         accountUrl,
		Authorities: authUrls,
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

// WriteData writes data to a data account
func (c *Client) WriteData(ctx context.Context, accountURL, signerURL string, privateKeyHex string, data []byte, writeToState bool) ([]byte, error) {
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
	accountUrl, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	signerUrl, err := url.Parse(signerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid signer URL: %w", err)
	}

	// Create data entry
	dataEntry := &protocol.AccumulateDataEntry{
		Data: [][]byte{data},
	}

	// Create WriteData transaction
	body := &protocol.WriteData{
		Entry:        dataEntry,
		WriteToState: writeToState,
		Scratch:      false,
	}

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

	// Return the transaction hash
	return txnHash[:], nil
}

// CreateTokenAccount creates a new token account under an ADI
func (c *Client) CreateTokenAccount(ctx context.Context, accountURL, tokenURL, sponsor string, privateKeyHex string, authorities []string) ([]byte, error) {
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
	accountUrl, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	tokenUrl, err := url.Parse(tokenURL)
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %w", err)
	}

	sponsorUrl, err := url.Parse(sponsor)
	if err != nil {
		return nil, fmt.Errorf("invalid sponsor URL: %w", err)
	}

	// Parse optional authorities
	var authUrls []*url.URL
	if len(authorities) > 0 {
		authUrls = make([]*url.URL, len(authorities))
		for i, auth := range authorities {
			authUrl, err := url.Parse(auth)
			if err != nil {
				return nil, fmt.Errorf("invalid authority URL %d: %w", i, err)
			}
			authUrls[i] = authUrl
		}
	}

	// Create CreateTokenAccount transaction
	body := &protocol.CreateTokenAccount{
		Url:         accountUrl,
		TokenUrl:    tokenUrl,
		Authorities: authUrls,
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

// EncodeData encodes data based on the specified encoding
func EncodeData(data string, encoding string) ([]byte, error) {
	switch encoding {
	case "hex":
		if len(data) > 2 && data[:2] == "0x" {
			data = data[2:]
		}
		return hex.DecodeString(data)
	case "base64":
		return base64.StdEncoding.DecodeString(data)
	case "utf8", "":
		return []byte(data), nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %s (supported: hex, base64, utf8)", encoding)
	}
}
