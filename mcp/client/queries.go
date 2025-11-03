package client

import (
	"context"
	"encoding/hex"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// QueryChain queries a chain by name, index, or entry hash
func (c *Client) QueryChain(ctx context.Context, accountURL string, params map[string]interface{}) (interface{}, error) {
	accountUrl, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	query := &api.ChainQuery{}

	// Set chain name if provided
	if name, ok := params["chain_name"].(string); ok {
		query.Name = name
	}

	// Set index if provided
	if index, ok := params["index"].(uint64); ok {
		query.Index = &index
	} else if indexFloat, ok := params["index"].(float64); ok {
		idx := uint64(indexFloat)
		query.Index = &idx
	}

	// Set range if provided
	if start, ok := params["start"].(uint64); ok {
		if count, ok := params["count"].(uint64); ok {
			query.Range = &api.RangeOptions{
				Start: start,
				Count: &count,
			}
		}
	} else if startFloat, ok := params["start"].(float64); ok {
		if countFloat, ok := params["count"].(float64); ok {
			cnt := uint64(countFloat)
			query.Range = &api.RangeOptions{
				Start: uint64(startFloat),
				Count: &cnt,
			}
		}
	}

	record, err := c.client.Query(ctx, accountUrl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query chain: %w", err)
	}

	return record, nil
}

// QueryData queries data entries from a data account
func (c *Client) QueryData(ctx context.Context, accountURL string, params map[string]interface{}) (interface{}, error) {
	accountUrl, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	query := &api.DataQuery{}

	// Set index if provided
	if index, ok := params["index"].(uint64); ok {
		query.Index = &index
	} else if indexFloat, ok := params["index"].(float64); ok {
		idx := uint64(indexFloat)
		query.Index = &idx
	}

	// Set entry if provided (hex encoded)
	if entryHex, ok := params["entry"].(string); ok {
		entry, err := hex.DecodeString(entryHex)
		if err == nil {
			query.Entry = entry
		}
	}

	// Set range if provided
	if start, ok := params["start"].(uint64); ok {
		if count, ok := params["count"].(uint64); ok {
			query.Range = &api.RangeOptions{
				Start: start,
				Count: &count,
			}
		}
	} else if startFloat, ok := params["start"].(float64); ok {
		if countFloat, ok := params["count"].(float64); ok {
			cnt := uint64(countFloat)
			query.Range = &api.RangeOptions{
				Start: uint64(startFloat),
				Count: &cnt,
			}
		}
	}

	record, err := c.client.Query(ctx, accountUrl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query data: %w", err)
	}

	return record, nil
}

// QueryDirectory queries the directory of an identity (lists sub-accounts)
func (c *Client) QueryDirectory(ctx context.Context, accountURL string, params map[string]interface{}) (interface{}, error) {
	accountUrl, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	query := &api.DirectoryQuery{}

	// Set range if provided
	if start, ok := params["start"].(uint64); ok {
		if count, ok := params["count"].(uint64); ok {
			query.Range = &api.RangeOptions{
				Start: start,
				Count: &count,
			}
		}
	} else if startFloat, ok := params["start"].(float64); ok {
		if countFloat, ok := params["count"].(float64); ok {
			cnt := uint64(countFloat)
			query.Range = &api.RangeOptions{
				Start: uint64(startFloat),
				Count: &cnt,
			}
		}
	}

	record, err := c.client.Query(ctx, accountUrl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query directory: %w", err)
	}

	return record, nil
}

// QueryPending queries pending transactions for an account
func (c *Client) QueryPending(ctx context.Context, accountURL string, params map[string]interface{}) (interface{}, error) {
	accountUrl, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	query := &api.PendingQuery{}

	// Set range if provided
	if start, ok := params["start"].(uint64); ok {
		if count, ok := params["count"].(uint64); ok {
			query.Range = &api.RangeOptions{
				Start: start,
				Count: &count,
			}
		}
	} else if startFloat, ok := params["start"].(float64); ok {
		if countFloat, ok := params["count"].(float64); ok {
			cnt := uint64(countFloat)
			query.Range = &api.RangeOptions{
				Start: uint64(startFloat),
				Count: &cnt,
			}
		}
	}

	record, err := c.client.Query(ctx, accountUrl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending: %w", err)
	}

	return record, nil
}

// QueryMinorBlock queries a minor block by index
func (c *Client) QueryMinorBlock(ctx context.Context, partition string, index interface{}) (interface{}, error) {
	partitionUrl, err := url.Parse(partition)
	if err != nil {
		return nil, fmt.Errorf("invalid partition URL: %w", err)
	}

	query := &api.BlockQuery{}

	// Set index if provided
	if idx, ok := index.(uint64); ok {
		query.Minor = &idx
	} else if idxFloat, ok := index.(float64); ok {
		idx := uint64(idxFloat)
		query.Minor = &idx
	}

	record, err := c.client.Query(ctx, partitionUrl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query minor block: %w", err)
	}

	return record, nil
}

// QueryMajorBlock queries a major block by index
func (c *Client) QueryMajorBlock(ctx context.Context, partition string, index interface{}) (interface{}, error) {
	partitionUrl, err := url.Parse(partition)
	if err != nil {
		return nil, fmt.Errorf("invalid partition URL: %w", err)
	}

	query := &api.BlockQuery{}

	// Set index if provided
	if idx, ok := index.(uint64); ok {
		query.Major = &idx
	} else if idxFloat, ok := index.(float64); ok {
		idx := uint64(idxFloat)
		query.Major = &idx
	}

	record, err := c.client.Query(ctx, partitionUrl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query major block: %w", err)
	}

	return record, nil
}

// SearchAnchor searches for anchors across the network
func (c *Client) SearchAnchor(ctx context.Context, anchor string, params map[string]interface{}) (interface{}, error) {
	// Decode anchor as bytes (could be hex or URL path)
	var anchorBytes []byte
	if len(anchor) > 2 && anchor[:2] == "0x" {
		decoded, err := hex.DecodeString(anchor[2:])
		if err != nil {
			return nil, fmt.Errorf("invalid anchor hex: %w", err)
		}
		anchorBytes = decoded
	} else {
		// Treat as URL and use path bytes
		anchorUrl, err := url.Parse(anchor)
		if err != nil {
			return nil, fmt.Errorf("invalid anchor URL: %w", err)
		}
		anchorBytes = []byte(anchorUrl.String())
	}

	query := &api.AnchorSearchQuery{
		Anchor: anchorBytes,
	}

	// Set includeReceipt if provided
	if includeReceipt, ok := params["includeReceipt"].(bool); ok {
		query.IncludeReceipt = &api.ReceiptOptions{
			ForAny: includeReceipt,
		}
	}

	record, err := c.client.Query(ctx, nil, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search anchor: %w", err)
	}

	return record, nil
}

// SearchPublicKey searches for accounts by public key
func (c *Client) SearchPublicKey(ctx context.Context, publicKeyHex string, params map[string]interface{}) (interface{}, error) {
	// Decode public key
	if len(publicKeyHex) > 2 && publicKeyHex[:2] == "0x" {
		publicKeyHex = publicKeyHex[2:]
	}
	publicKey, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	// Default to ED25519 signature type
	sigType := protocol.SignatureTypeED25519

	// Set type if provided in params
	if typeStr, ok := params["type"].(string); ok {
		// TODO: Add proper SignatureType parsing from string
		// For now, we only support ED25519 as default
		_ = typeStr
	}

	query := &api.PublicKeySearchQuery{
		PublicKey: publicKey,
		Type:      sigType,
	}

	// Use DN as default scope for search queries
	scope, _ := url.Parse("acc://dn.acme")
	if scopeStr, ok := params["scope"].(string); ok {
		if parsedScope, err := url.Parse(scopeStr); err == nil {
			scope = parsedScope
		}
	}

	record, err := c.client.Query(ctx, scope, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search public key: %w", err)
	}

	return record, nil
}

// SearchPublicKeyHash searches for accounts by public key hash
func (c *Client) SearchPublicKeyHash(ctx context.Context, publicKeyHashHex string, params map[string]interface{}) (interface{}, error) {
	// Decode public key hash
	if len(publicKeyHashHex) > 2 && publicKeyHashHex[:2] == "0x" {
		publicKeyHashHex = publicKeyHashHex[2:]
	}
	publicKeyHash, err := hex.DecodeString(publicKeyHashHex)
	if err != nil {
		return nil, fmt.Errorf("invalid public key hash: %w", err)
	}

	query := &api.PublicKeyHashSearchQuery{
		PublicKeyHash: publicKeyHash,
	}

	record, err := c.client.Query(ctx, nil, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search public key hash: %w", err)
	}

	return record, nil
}

// SearchDelegate searches for delegated authority
func (c *Client) SearchDelegate(ctx context.Context, delegate string, params map[string]interface{}) (interface{}, error) {
	delegateUrl, err := url.Parse(delegate)
	if err != nil {
		return nil, fmt.Errorf("invalid delegate URL: %w", err)
	}

	query := &api.DelegateSearchQuery{
		Delegate: delegateUrl,
	}

	record, err := c.client.Query(ctx, nil, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search delegate: %w", err)
	}

	return record, nil
}

// SearchMessageHash searches for messages by hash
func (c *Client) SearchMessageHash(ctx context.Context, hashHex string, params map[string]interface{}) (interface{}, error) {
	// Decode hash
	if len(hashHex) > 2 && hashHex[:2] == "0x" {
		hashHex = hashHex[2:]
	}
	hashBytes, err := hex.DecodeString(hashHex)
	if err != nil {
		return nil, fmt.Errorf("invalid hash: %w", err)
	}

	hash := [32]byte{}
	copy(hash[:], hashBytes)

	query := &api.MessageHashSearchQuery{
		Hash: hash,
	}

	record, err := c.client.Query(ctx, nil, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search message hash: %w", err)
	}

	return record, nil
}

// QueryKeyBook queries a KeyBook account
// KeyBooks contain KeyPages and define the authority structure for an ADI
func (c *Client) QueryKeyBook(ctx context.Context, keyBookURL string) (interface{}, error) {
	keyBookUrl, err := url.Parse(keyBookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid KeyBook URL: %w", err)
	}

	// Use default query - KeyBooks are standard accounts
	query := &api.DefaultQuery{}

	record, err := c.client.Query(ctx, keyBookUrl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query KeyBook: %w", err)
	}

	return record, nil
}

// QueryKeyPage queries a KeyPage account
// KeyPages contain public keys and define signature thresholds
func (c *Client) QueryKeyPage(ctx context.Context, keyPageURL string) (interface{}, error) {
	keyPageUrl, err := url.Parse(keyPageURL)
	if err != nil {
		return nil, fmt.Errorf("invalid KeyPage URL: %w", err)
	}

	// Use default query - KeyPages are standard accounts
	query := &api.DefaultQuery{}

	record, err := c.client.Query(ctx, keyPageUrl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query KeyPage: %w", err)
	}

	return record, nil
}
