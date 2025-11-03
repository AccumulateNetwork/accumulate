package client

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// CreateKeyPage creates a new KeyPage in an existing KeyBook
func (c *Client) CreateKeyPage(ctx context.Context, keybookURL, signerURL string, privateKeyHex string, keys []string) ([]byte, error) {
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
	keybookUrl, err := accurl.Parse(keybookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid keybook URL: %w", err)
	}

	signerUrl, err := accurl.Parse(signerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid signer URL: %w", err)
	}

	// Parse and convert keys
	keySpecs := make([]*protocol.KeySpecParams, len(keys))
	for i, keyHex := range keys {
		if len(keyHex) > 2 && keyHex[:2] == "0x" {
			keyHex = keyHex[2:]
		}
		keyBytes, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("invalid key %d: %w", i, err)
		}

		// For public key hashes, use SHA256 of the public key
		keyHash, err := protocol.PublicKeyHash(keyBytes, protocol.SignatureTypeED25519)
		if err != nil {
			return nil, fmt.Errorf("failed to hash key %d: %w", i, err)
		}

		keySpecs[i] = &protocol.KeySpecParams{
			KeyHash: keyHash,
		}
	}

	// Create transaction
	body := &protocol.CreateKeyPage{
		Keys: keySpecs,
	}

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: keybookUrl,
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

// UpdateKeyPage updates an existing KeyPage by adding or removing keys, or updating threshold
func (c *Client) UpdateKeyPage(ctx context.Context, keypageURL, signerURL string, privateKeyHex string, operation string, key string, newThreshold uint64) ([]byte, error) {
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
	keypageUrl, err := accurl.Parse(keypageURL)
	if err != nil {
		return nil, fmt.Errorf("invalid keypage URL: %w", err)
	}

	signerUrl, err := accurl.Parse(signerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid signer URL: %w", err)
	}

	// Create update operation
	body := &protocol.UpdateKeyPage{}

	switch operation {
	case "add":
		if key == "" {
			return nil, fmt.Errorf("key is required for add operation")
		}
		if len(key) > 2 && key[:2] == "0x" {
			key = key[2:]
		}
		keyBytes, err := hex.DecodeString(key)
		if err != nil {
			return nil, fmt.Errorf("invalid key: %w", err)
		}
		body.Operation = []protocol.KeyPageOperation{
			&protocol.AddKeyOperation{
				Entry: protocol.KeySpecParams{
					KeyHash: keyBytes,
				},
			},
		}

	case "remove":
		if key == "" {
			return nil, fmt.Errorf("key is required for remove operation")
		}
		if len(key) > 2 && key[:2] == "0x" {
			key = key[2:]
		}
		keyBytes, err := hex.DecodeString(key)
		if err != nil {
			return nil, fmt.Errorf("invalid key: %w", err)
		}
		body.Operation = []protocol.KeyPageOperation{
			&protocol.RemoveKeyOperation{
				Entry: protocol.KeySpecParams{
					KeyHash: keyBytes,
				},
			},
		}

	case "set_threshold":
		if newThreshold == 0 {
			return nil, fmt.Errorf("threshold is required for set_threshold operation")
		}
		body.Operation = []protocol.KeyPageOperation{
			&protocol.SetThresholdKeyPageOperation{
				Threshold: newThreshold,
			},
		}

	default:
		return nil, fmt.Errorf("invalid operation: %s (must be add, remove, or set_threshold)", operation)
	}

	// Create transaction
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: keypageUrl,
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

// CreateKeyBook creates a new KeyBook for an ADI
func (c *Client) CreateKeyBook(ctx context.Context, keybookURL, signerURL string, privateKeyHex string, publicKeyHash string) ([]byte, error) {
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
	keybookUrl, err := accurl.Parse(keybookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid keybook URL: %w", err)
	}

	signerUrl, err := accurl.Parse(signerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid signer URL: %w", err)
	}

	// Parse public key hash
	if len(publicKeyHash) > 2 && publicKeyHash[:2] == "0x" {
		publicKeyHash = publicKeyHash[2:]
	}
	keyHashBytes, err := hex.DecodeString(publicKeyHash)
	if err != nil {
		return nil, fmt.Errorf("invalid public key hash: %w", err)
	}

	// Create transaction
	body := &protocol.CreateKeyBook{
		Url: keybookUrl,
		PublicKeyHash: keyHashBytes,
	}

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: keybookUrl.RootIdentity(),
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

// UpdateAccountAuth updates account authorities by adding or removing authority URLs
func (c *Client) UpdateAccountAuth(ctx context.Context, accountURL, signerURL string, privateKeyHex string, operations []map[string]interface{}) ([]byte, error) {
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

	// Create update operations
	body := &protocol.UpdateAccountAuth{
		Operations: []protocol.AccountAuthOperation{},
	}

	for i, op := range operations {
		opType, ok := op["type"].(string)
		if !ok {
			return nil, fmt.Errorf("operation %d: missing type", i)
		}

		switch opType {
		case "add":
			authURL, ok := op["authority"].(string)
			if !ok {
				return nil, fmt.Errorf("operation %d: missing authority", i)
			}
			authUrl, err := accurl.Parse(authURL)
			if err != nil {
				return nil, fmt.Errorf("operation %d: invalid authority URL: %w", i, err)
			}
			body.Operations = append(body.Operations, &protocol.AddAccountAuthorityOperation{
				Authority: authUrl,
			})

		case "remove":
			authURL, ok := op["authority"].(string)
			if !ok {
				return nil, fmt.Errorf("operation %d: missing authority", i)
			}
			authUrl, err := accurl.Parse(authURL)
			if err != nil {
				return nil, fmt.Errorf("operation %d: invalid authority URL: %w", i, err)
			}
			body.Operations = append(body.Operations, &protocol.RemoveAccountAuthorityOperation{
				Authority: authUrl,
			})

		case "enable":
			authURL, ok := op["authority"].(string)
			if !ok {
				return nil, fmt.Errorf("operation %d: missing authority", i)
			}
			authUrl, err := accurl.Parse(authURL)
			if err != nil {
				return nil, fmt.Errorf("operation %d: invalid authority URL: %w", i, err)
			}
			body.Operations = append(body.Operations, &protocol.EnableAccountAuthOperation{
				Authority: authUrl,
			})

		case "disable":
			authURL, ok := op["authority"].(string)
			if !ok {
				return nil, fmt.Errorf("operation %d: missing authority", i)
			}
			authUrl, err := accurl.Parse(authURL)
			if err != nil {
				return nil, fmt.Errorf("operation %d: invalid authority URL: %w", i, err)
			}
			body.Operations = append(body.Operations, &protocol.DisableAccountAuthOperation{
				Authority: authUrl,
			})

		default:
			return nil, fmt.Errorf("operation %d: invalid type: %s (must be add, remove, enable, or disable)", i, opType)
		}
	}

	if len(body.Operations) == 0 {
		return nil, fmt.Errorf("no operations provided")
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
