// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package client

import (
	"context"
	"encoding/hex"
	"fmt"

	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// GetTransaction queries a transaction by its ID.
//
// Example:
//
//	tx, err := client.GetTransaction(ctx, "0123456789abcdef...")
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Transaction: %+v\n", tx)
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "query",
//	    "params": {
//	      "scope": "acc://dn.acme/network",
//	      "query": {
//	        "type": "message-hash",
//	        "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
//	      }
//	    },
//	    "id": 1
//	  }'
func (c *Client) GetTransaction(ctx context.Context, txIDHex string) (*v3.MessageRecord[messaging.Message], error) {
	// Parse transaction ID from hex
	txID, err := hex.DecodeString(txIDHex)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction ID: %w", err)
	}

	// Convert to [32]byte hash
	if len(txID) != 32 {
		return nil, fmt.Errorf("transaction ID must be 32 bytes, got %d", len(txID))
	}
	var hash [32]byte
	copy(hash[:], txID)

	// Use MessageHashSearchQuery to find the transaction
	query := &v3.MessageHashSearchQuery{
		Hash: hash,
	}

	// Query against the network scope
	networkScope, err := url.Parse("acc://dn.acme/network")
	if err != nil {
		return nil, fmt.Errorf("failed to parse network URL: %w", err)
	}

	record, err := c.v3Client.Query(ctx, networkScope, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query transaction: %w", err)
	}

	// Type assert to MessageRecord
	txRecord, ok := record.(*v3.MessageRecord[messaging.Message])
	if !ok {
		return nil, fmt.Errorf("unexpected record type: %T", record)
	}

	return txRecord, nil
}

// GetChainEntry queries a specific entry from an account's chain.
//
// Example:
//
//	entry, err := client.GetChainEntry(ctx, "acc://mytoken.acme", "main", 0)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Chain entry: %+v\n", entry)
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "query",
//	    "params": {
//	      "scope": "acc://mytoken.acme",
//	      "query": {
//	        "type": "chain",
//	        "name": "main",
//	        "index": 0
//	      }
//	    },
//	    "id": 1
//	  }'
func (c *Client) GetChainEntry(ctx context.Context, accountURL string, chainName string, index uint64) (*v3.ChainEntryRecord[v3.Record], error) {
	u, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	// Use ChainQuery to get the chain entry
	query := &v3.ChainQuery{
		Name:  chainName,
		Index: &index,
	}

	record, err := c.v3Client.Query(ctx, u, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query chain entry: %w", err)
	}

	// Type assert to ChainEntryRecord
	chainRecord, ok := record.(*v3.ChainEntryRecord[v3.Record])
	if !ok {
		return nil, fmt.Errorf("unexpected record type: %T", record)
	}

	return chainRecord, nil
}

// GetDataEntry queries a specific entry from an account's data chain.
//
// Example:
//
//	entry, err := client.GetDataEntry(ctx, "acc://mydata.acme", 0)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Data entry: %+v\n", entry)
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "query",
//	    "params": {
//	      "scope": "acc://mydata.acme",
//	      "query": {
//	        "type": "data",
//	        "index": 0
//	      }
//	    },
//	    "id": 1
//	  }'
func (c *Client) GetDataEntry(ctx context.Context, accountURL string, index uint64) (*v3.ChainEntryRecord[v3.Record], error) {
	u, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	// Use DataQuery to get the data entry
	query := &v3.DataQuery{
		Index: &index,
	}

	record, err := c.v3Client.Query(ctx, u, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query data entry: %w", err)
	}

	// Type assert to ChainEntryRecord (data entries are returned as chain entries)
	dataRecord, ok := record.(*v3.ChainEntryRecord[v3.Record])
	if !ok {
		return nil, fmt.Errorf("unexpected record type: %T", record)
	}

	return dataRecord, nil
}

// GetDirectory queries the directory entries of an account.
//
// Example:
//
//	entries, err := client.GetDirectory(ctx, "acc://myadi.acme", 0, 10)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Directory entries: %+v\n", entries)
//
// Curl equivalent:
//
//	curl -X POST http://localhost:8080/v3 \
//	  -H "Content-Type: application/json" \
//	  -d '{
//	    "jsonrpc": "2.0",
//	    "method": "query",
//	    "params": {
//	      "scope": "acc://myadi.acme",
//	      "query": {
//	        "type": "directory",
//	        "range": {
//	          "start": 0,
//	          "count": 10
//	        }
//	      }
//	    },
//	    "id": 1
//	  }'
func (c *Client) GetDirectory(ctx context.Context, accountURL string, start uint64, count uint64) (*v3.RecordRange[*v3.AccountRecord], error) {
	u, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	// Use DirectoryQuery to get the directory
	query := &v3.DirectoryQuery{
		Range: &v3.RangeOptions{
			Start: start,
			Count: &count,
		},
	}

	record, err := c.v3Client.Query(ctx, u, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query directory: %w", err)
	}

	// Type assert to RecordRange
	dirRecord, ok := record.(*v3.RecordRange[v3.Record])
	if !ok {
		// Try the account-specific version
		dirRecord2, ok2 := record.(*v3.RecordRange[*v3.AccountRecord])
		if ok2 {
			// Convert to the more general type
			result := &v3.RecordRange[*v3.AccountRecord]{
				Start:   dirRecord2.Start,
				Total:   dirRecord2.Total,
				Records: dirRecord2.Records,
			}
			return result, nil
		}
		return nil, fmt.Errorf("unexpected record type: %T", record)
	}

	// Convert to account records
	result := &v3.RecordRange[*v3.AccountRecord]{
		Start: dirRecord.Start,
		Total: dirRecord.Total,
	}
	for _, r := range dirRecord.Records {
		if acc, ok := r.(*v3.AccountRecord); ok {
			result.Records = append(result.Records, acc)
		}
	}

	return result, nil
}
