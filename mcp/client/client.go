package client

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const (
	MainnetEndpoint = "https://mainnet.accumulatenetwork.io/v3"
	TestnetEndpoint = "https://testnet.accumulatenetwork.io/v3"
)

// Client wraps the Accumulate SDK jsonrpc.Client
type Client struct {
	client  *jsonrpc.Client
	network string
}

// NewClient creates a new Accumulate client using the SDK
func NewClient(network string) (*Client, error) {
	// Validate network parameter
	if network == "" {
		return nil, fmt.Errorf("network parameter cannot be empty")
	}

	endpoint := getEndpoint(network)

	// Validate endpoint is a valid URL for custom endpoints
	if network != "mainnet" && network != "testnet" {
		if _, err := url.Parse(endpoint); err != nil {
			return nil, fmt.Errorf("invalid network URL: %w", err)
		}
		// Basic validation that it looks like a URL
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			return nil, fmt.Errorf("invalid network URL: must start with http:// or https://")
		}
	}

	// Create SDK client
	sdkClient := jsonrpc.NewClient(endpoint)
	sdkClient.Client.Timeout = 15 * time.Second

	return &Client{
		client:  sdkClient,
		network: network,
	}, nil
}

// getEndpoint returns the appropriate RPC endpoint for the network
func getEndpoint(network string) string {
	switch network {
	case "mainnet", "":
		return MainnetEndpoint
	case "testnet":
		return TestnetEndpoint
	default:
		return network
	}
}

// QueryAccount queries an Accumulate account by URL
func (c *Client) QueryAccount(ctx context.Context, accountURL string) (interface{}, error) {
	// Parse account URL
	accountUrl, err := accurl.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	// Create typed query - DefaultQuery doesn't have a Url field
	// The URL is passed as the scope parameter to Query()
	query := &api.DefaultQuery{}

	// Execute query using SDK
	record, err := c.client.Query(ctx, accountUrl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}

	return record, nil
}

// QueryTransaction queries a transaction by hash
func (c *Client) QueryTransaction(ctx context.Context, txidHex string) (interface{}, error) {
	// Decode transaction ID
	if len(txidHex) > 2 && txidHex[:2] == "0x" {
		txidHex = txidHex[2:]
	}
	txid, err := hex.DecodeString(txidHex)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction ID: %w", err)
	}

	// Create typed query - it's MessageHashSearchQuery, not MessageHashQuery
	txHash := [32]byte{}
	copy(txHash[:], txid)
	query := &api.MessageHashSearchQuery{
		Hash: txHash,
	}

	// Use DN as default scope for search queries
	scope, _ := accurl.Parse("acc://dn.acme")

	// Execute query using SDK
	record, err := c.client.Query(ctx, scope, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query transaction: %w", err)
	}

	return record, nil
}

// CreateLiteAccountURL generates a lite account URL from a public key using SDK
func CreateLiteAccountURL(publicKeyHex string) (string, error) {
	// Remove 0x prefix if present
	if len(publicKeyHex) > 2 && publicKeyHex[:2] == "0x" {
		publicKeyHex = publicKeyHex[2:]
	}

	// Decode hex public key
	keyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return "", fmt.Errorf("invalid public key hex: %w", err)
	}

	// Use SDK's LiteAuthorityForKey to generate the correct lite account URL
	liteUrl := protocol.LiteAuthorityForKey(keyBytes, protocol.SignatureTypeED25519)

	// Return as ACME token account
	return liteUrl.JoinPath("ACME").String(), nil
}

// SendTokens creates, signs, and submits a token transfer transaction using SDK
func (c *Client) SendTokens(ctx context.Context, from, to string, amount int64, privateKeyHex string) ([]byte, error) {
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

	// Parse URLs using SDK
	fromUrl, err := accurl.Parse(from)
	if err != nil {
		return nil, fmt.Errorf("invalid from URL: %w", err)
	}

	toUrl, err := accurl.Parse(to)
	if err != nil {
		return nil, fmt.Errorf("invalid to URL: %w", err)
	}

	// Create transaction using protocol types
	// Amount is big.Int, not protocol.BigInt
	body := &protocol.SendTokens{
		To: []*protocol.TokenRecipient{{
			Url:    toUrl,
			Amount: *big.NewInt(amount),
		}},
	}

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: fromUrl,
		},
		Body: body,
	}

	// Create signature
	sig := &protocol.ED25519Signature{
		PublicKey: privateKey.Public().(ed25519.PublicKey),
		Signer:    fromUrl.RootIdentity(),
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

// SubmitEnvelope submits an envelope with multiple transactions and their signatures
// This allows batching multiple operations in a single submission for efficiency
func (c *Client) SubmitEnvelope(ctx context.Context, transactions []*protocol.Transaction, signatures []protocol.Signature) ([][]byte, error) {
	if len(transactions) == 0 {
		return nil, fmt.Errorf("no transactions provided")
	}
	if len(signatures) == 0 {
		return nil, fmt.Errorf("no signatures provided")
	}

	// Create envelope with multiple transactions
	envelope := &messaging.Envelope{
		Transaction: transactions,
		Signatures:  signatures,
	}

	// Submit using SDK
	submissions, err := c.client.Submit(ctx, envelope, api.SubmitOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to submit envelope: %w", err)
	}

	if len(submissions) == 0 {
		return nil, fmt.Errorf("no submission result returned")
	}

	// Return all transaction hashes
	hashes := make([][]byte, len(transactions))
	for i, txn := range transactions {
		txnHash := txn.GetHash()
		hashes[i] = txnHash[:]
	}

	return hashes, nil
}

// Close closes the API client connection
func (c *Client) Close() error {
	return nil
}
