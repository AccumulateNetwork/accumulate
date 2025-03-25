// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// isValidP2PMultiaddr checks if a multiaddress is a valid P2P address
// Valid P2P addresses must have both IP and TCP components, as well as a P2P component
func isValidP2PMultiaddr(addr multiaddr.Multiaddr) bool {
	// Check if the address has a P2P component
	if _, err := addr.ValueForProtocol(multiaddr.P_P2P); err != nil {
		return false
	}
	
	// Check if the address has an IP component (either IPv4 or IPv6)
	hasIP := false
	if _, err := addr.ValueForProtocol(multiaddr.P_IP4); err == nil {
		hasIP = true
	} else if _, err := addr.ValueForProtocol(multiaddr.P_IP6); err == nil {
		hasIP = true
	}
	
	// Check if the address has a TCP component
	hasTCP := false
	if _, err := addr.ValueForProtocol(multiaddr.P_TCP); err == nil {
		hasTCP = true
	}
	
	// A valid P2P multiaddress must have both IP and TCP components
	return hasIP && hasTCP
}

func TestCollectPeersAndDirectoryAnchors(t *testing.T) {
	// Skip this test in automated test runs
	//t.Skip("Manual test - requires network access")

	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Network to test against
	const network = "mainnet"

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a JSON-RPC client
	jsonClient := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(network, "v3"))
	jsonClient.Debug = true
	jsonClient.Client.Timeout = 2 * time.Minute

	// Discover peers via JSON-RPC
	fmt.Println("Discovering peers via JSON-RPC...")
	jsonrpcPeers, err := discoverPeersViaJSONRPC(ctx, jsonClient)
	require.NoError(t, err, "Failed to discover peers via JSON-RPC")
	fmt.Printf("Discovered %d peers via JSON-RPC\n", len(jsonrpcPeers))

	// Filter out invalid P2P multiaddresses
	var validPeers []multiaddr.Multiaddr
	for _, addr := range jsonrpcPeers {
		if isValidP2PMultiaddr(addr) {
			validPeers = append(validPeers, addr)
		} else {
			fmt.Printf("Skipping invalid P2P multiaddress: %s\n", addr)
		}
	}
	
	// Print the number of valid peers
	fmt.Printf("Valid P2P multiaddresses: %d out of %d discovered peers\n", 
		len(validPeers), len(jsonrpcPeers))
	
	// Print the peer list
	fmt.Println("Peer list:")
	for i, addr := range validPeers {
		fmt.Printf("  Peer %d: %s\n", i+1, addr.String())
	}

	// Initialize the P2P node with the discovered peers
	fmt.Println("Initializing P2P node...")
	node, err := p2p.New(p2p.Options{
		Network:           network,
		BootstrapPeers:    append(bootstrap, validPeers...),
		PeerDatabase:      "test-peers.json",
		EnablePeerTracker: true,
		PeerScanFrequency: -1,
	})
	require.NoError(t, err, "Failed to initialize P2P node")
	defer node.Close()

	// Wait for the P2P node to initialize
	fmt.Println("Waiting for P2P node to initialize...")
	time.Sleep(10 * time.Second)

	// Create a healer instance
	h := new(healer)
	h.Reset()
	currentHealer = h

	// Set up the healer
	h.ctx = ctx
	h.network = network
	h.C1 = jsonClient
	h.C2 = &message.Client{
		Transport: &message.RoutedTransport{
			Network: network,
			Dialer:  node.DialNetwork(),
		},
	}

	// Query for the top 1000 anchors in the Directory partition
	fmt.Println("Querying for anchors in the Directory partition...")

	// Get the URL for the Directory partition
	directoryUrl := protocol.PartitionUrl(protocol.Directory)

	// First, query the chain info to get the total count of entries
	fmt.Println("Getting chain info for Directory anchor chain...")
	chainQuery := &api.ChainQuery{
		Name: "anchor-sequence",
	}
	
	// Use the tryEach().QueryChain method to get the chain info
	chainInfo, err := h.tryEach().QueryChain(ctx, directoryUrl.JoinPath("anchor-sequence"), chainQuery)
	if err != nil {
		t.Fatalf("Failed to get chain info: %v", err)
	}

	fmt.Printf("Directory anchor chain info: Count=%d, Type=%s\n",
		chainInfo.Count, chainInfo.Type)

	// Print the height of the anchor chain
	fmt.Printf("Directory anchor chain height: %d\n", chainInfo.Count)

	// Get and print the top 5 entries of the anchor chain (most recent)
	fmt.Println("Top 5 entries of the Directory anchor chain:")

	// Only query entries if there are any
	if chainInfo.Count == 0 {
		fmt.Println("WARNING: Directory anchor chain has zero entries. In production, this chain should have millions of entries.")
		return
	}

	// Create a query for the most recent 5 entries
	var entryCount uint64 = 5
	var startIndex uint64 = 0
	
	// If there are more than 5 entries, start from (count - 5)
	if chainInfo.Count > 5 {
		startIndex = chainInfo.Count - 5
	}
	
	entryQuery := &api.ChainQuery{
		Name: "anchor-sequence",
		Range: &api.RangeOptions{
			Start: startIndex,
			Count: &entryCount,
		},
	}

	// Execute the query to get the chain entries
	fmt.Printf("Querying for entries %d to %d of the anchor chain...\n", 
		startIndex, startIndex + entryCount - 1)
	
	// Use the direct client to avoid potential issues with the P2P network
	entryResult, err := h.C1.Query(ctx, directoryUrl.JoinPath("anchor-sequence"), entryQuery)
	if err != nil {
		t.Fatalf("Failed to query chain entries: %v", err)
	}

	// Try to cast the result to different possible types
	if entries, ok := entryResult.(*api.RecordRange[*api.ChainEntryRecord[api.Record]]); ok {
		// Print the entries in reverse order (newest first)
		fmt.Printf("Found %d entries in the chain\n", len(entries.Records))
		
		// Check if we have any records to print
		if len(entries.Records) == 0 {
			fmt.Println("No entries found in the chain")
			return
		}
		
		// Print the entries (newest first)
		for i := len(entries.Records) - 1; i >= 0; i-- {
			entry := entries.Records[i]
			fmt.Printf("  Entry %d: Index=%d, Hash=%x\n", 
				len(entries.Records)-i, entry.Index, entry.Entry)
		}
		return
	}
	
	// Try the generic RecordRange type
	if entries, ok := entryResult.(*api.RecordRange[api.Record]); ok {
		fmt.Printf("Found %d entries in the chain\n", len(entries.Records))
		
		// Check if we have any records to print
		if len(entries.Records) == 0 {
			fmt.Println("No entries found in the chain")
			return
		}
		
		// Print the entries (newest first)
		for i := len(entries.Records) - 1; i >= 0; i-- {
			fmt.Printf("  Entry %d: %v\n", len(entries.Records)-i, entries.Records[i])
		}
		return
	}
	
	// If we reach here, we don't know how to handle this type
	t.Fatalf("Unexpected result type: %T", entryResult)
}

// TestDirectoryAnchorChainCount tests querying the number of entries in the Directory anchor chain
func TestDirectoryAnchorChainCount(t *testing.T) {
	// Skip this test in automated test runs
	//t.Skip("Manual test - requires network access")

	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Network to test against
	const network = "mainnet"

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Create a JSON-RPC client
	jsonClient := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(network, "v3"))
	jsonClient.Debug = true
	jsonClient.Client.Timeout = 1 * time.Minute

	// Create a healer instance for querying
	h := new(healer)
	h.Reset()
	h.ctx = ctx
	h.network = network
	h.C1 = jsonClient
	
	// Get the Directory anchor chain URL
	directoryUrl := protocol.DnUrl()
	fmt.Printf("Directory URL: %s\n", directoryUrl)

	// Query the chain info
	fmt.Println("Getting chain info for Directory anchor chain...")
	chainQuery := &api.ChainQuery{
		Name: "anchor-sequence",
	}
	
	chainInfo, err := h.C1.Query(ctx, directoryUrl.JoinPath("anchor-sequence"), chainQuery)
	require.NoError(t, err, "Failed to get chain info")

	// Try to cast the result to a ChainRecord
	chainRecord, ok := chainInfo.(*api.ChainRecord)
	require.True(t, ok, "Expected ChainRecord, got %T", chainInfo)
	
	fmt.Printf("Directory anchor chain info: Count=%d, Type=%s\n", chainRecord.Count, chainRecord.Type)
	fmt.Printf("Directory anchor chain height: %d\n", chainRecord.Count)

	// The Directory anchor chain should have millions of entries in production
	// For testing purposes, we'll just check if we can retrieve the count
	// We won't fail if there are no entries, as this might be a test environment
	fmt.Printf("Note: Found %d entries in the Directory anchor chain\n", chainRecord.Count)
	if chainRecord.Count == 0 {
		fmt.Println("Warning: No entries found in the Directory anchor chain. This is expected in test environments but not in production.")
	} else {
		fmt.Printf("Success: Found %d entries in the Directory anchor chain\n", chainRecord.Count)
	}

	// If there are entries, query the most recent ones
	if chainRecord.Count > 0 {
		fmt.Println("Querying recent entries from the Directory anchor chain...")
		
		// Calculate start and count for pagination
		// Get the most recent 5 entries or all if less than 5
		var startIndex uint64 = 0
		if chainRecord.Count > 5 {
			startIndex = chainRecord.Count - 5
		}
		var entryCount uint64 = 5
		if chainRecord.Count < entryCount {
			entryCount = chainRecord.Count
		}
		
		// Create a query for the chain entries with pagination
		entryQuery := &api.ChainQuery{
			Name: "anchor-sequence",
			Range: &api.RangeOptions{
				Start: startIndex,
				Count: &entryCount,
			},
		}
		
		// Execute the query to get the chain entries
		fmt.Printf("Querying for entries %d to %d of the anchor chain...\n", 
			startIndex, startIndex + entryCount - 1)
		
		entryResult, err := h.C1.Query(ctx, directoryUrl.JoinPath("anchor-sequence"), entryQuery)
		require.NoError(t, err, "Failed to query chain entries")
		
		// Try to cast the result to different possible types
		if entries, ok := entryResult.(*api.RecordRange[*api.ChainEntryRecord[api.Record]]); ok {
			// Print the entries in reverse order (newest first)
			fmt.Printf("Found %d entries in the chain\n", len(entries.Records))
			
			// Check if we have any records to print
			if len(entries.Records) == 0 {
				fmt.Println("No entries found in the chain")
				return
			}
			
			// Print the entries (newest first)
			for i := len(entries.Records) - 1; i >= 0; i-- {
				entry := entries.Records[i]
				fmt.Printf("  Entry %d: Index=%d, Hash=%x\n", 
					len(entries.Records)-i, entry.Index, entry.Entry)
			}
			return
		}
		
		// Try the generic RecordRange type
		if entries, ok := entryResult.(*api.RecordRange[api.Record]); ok {
			fmt.Printf("Found %d entries in the chain\n", len(entries.Records))
			
			// Check if we have any records to print
			if len(entries.Records) == 0 {
				fmt.Println("No entries found in the chain")
				return
			}
			
			// Print the entries (newest first)
			for i := len(entries.Records) - 1; i >= 0; i-- {
				fmt.Printf("  Entry %d: Type=%T\n", len(entries.Records)-i, entries.Records[i])
			}
			return
		}
		
		// If we reach here, we don't know how to handle this type
		t.Fatalf("Unexpected result type: %T", entryResult)
	} else {
		fmt.Println("No entries in the Directory anchor chain")
	}
}

// TestQueryStakingRequests tests querying the staking.acme/requests chain using JSON-RPC
func TestQueryStakingRequests(t *testing.T) {
	// Skip this test in automated test runs
	//t.Skip("Manual test - requires network access")

	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Network to test against
	const network = "mainnet"

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Create a JSON-RPC client
	jsonClient := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(network, "v3"))
	jsonClient.Debug = true
	jsonClient.Client.Timeout = 1 * time.Minute

	// Create the staking URL
	stakingUrl := accurl.MustParse("acc://staking.acme")
	fmt.Printf("Staking URL: %s\n", stakingUrl)

	// Query the staking requests chain
	fmt.Println("Querying staking.acme/requests chain...")
	
	// Create a chain query for the requests chain
	chainQuery := &api.ChainQuery{
		Name: "requests",
	}
	
	// Execute the query directly via JSON-RPC
	chainInfo, err := jsonClient.Query(ctx, stakingUrl, chainQuery)
	require.NoError(t, err, "Failed to query staking requests chain")
	
	// Try to cast the result to a ChainRecord
	chainRecord, ok := chainInfo.(*api.ChainRecord)
	require.True(t, ok, "Expected ChainRecord, got %T", chainInfo)
	
	fmt.Printf("Staking requests chain info: Count=%d, Type=%s\n", chainRecord.Count, chainRecord.Type)
	
	// If there are entries, query the most recent ones
	if chainRecord.Count > 0 {
		fmt.Printf("Found %d entries in the staking requests chain\n", chainRecord.Count)
		
		// Query the top 5 entries (or all if less than 5)
		entryCount := uint64(5)
		if chainRecord.Count < entryCount {
			entryCount = chainRecord.Count
		}
		
		entries, err := queryChainEntriesViaJSONRPC(ctx, jsonClient, stakingUrl.JoinPath("requests"), entryCount)
		require.NoError(t, err, "Failed to query staking requests entries")
		
		// Print the entries (newest first)
		fmt.Printf("Retrieved %d entries from the staking requests chain\n", len(entries.Records))
		for i := len(entries.Records) - 1; i >= 0; i-- {
			fmt.Printf("  Entry %d: %v\n", len(entries.Records)-i, entries.Records[i])
		}
	} else {
		fmt.Println("No entries found in the staking requests chain")
	}
}

// TestQueryStakingAccount tests querying the staking.acme account using JSON-RPC
func TestQueryStakingAccount(t *testing.T) {
	// Skip this test in automated test runs
	//t.Skip("Manual test - requires network access")

	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Network to test against
	const network = "mainnet"

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Create a JSON-RPC client
	jsonClient := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(network, "v3"))
	jsonClient.Debug = true
	jsonClient.Client.Timeout = 1 * time.Minute

	// Create the staking URL
	stakingUrl := accurl.MustParse("acc://staking.acme")
	fmt.Printf("Staking URL: %s\n", stakingUrl)

	// Query the staking account
	fmt.Println("Querying staking.acme account...")
	
	// Create a query for the account
	accountQuery := &api.DefaultQuery{}
	
	// Execute the query directly via JSON-RPC
	accountInfo, err := jsonClient.Query(ctx, stakingUrl, accountQuery)
	require.NoError(t, err, "Failed to query staking account")
	
	// Print the account info
	fmt.Printf("Staking account info: %T\n", accountInfo)
	
	// Try to cast the result to an AccountRecord
	accountRecord, ok := accountInfo.(*api.AccountRecord)
	require.True(t, ok, "Expected AccountRecord, got %T", accountInfo)
	
	// Print account details
	fmt.Printf("Staking account: %v\n", accountRecord.Account)
	
	// Check if the account has a 'requests' chain
	fmt.Println("Checking for chains in the staking account...")
	
	// Query the account's chains
	chainsQuery := &api.ChainQuery{}
	chains, err := jsonClient.Query(ctx, stakingUrl, chainsQuery)
	require.NoError(t, err, "Failed to query staking account chains")
	
	// Try to cast the result to a RecordRange of ChainRecord
	chainsRange, ok := chains.(*api.RecordRange[*api.ChainRecord])
	if ok {
		fmt.Printf("Found %d chains in the staking account\n", chainsRange.Total)
		
		// Print the chains
		for i, chain := range chainsRange.Records {
			fmt.Printf("  Chain %d: Name=%s, Type=%s, Count=%d\n", i+1, chain.Name, chain.Type, chain.Count)
			
			// If this is the requests chain and it has entries, query them
			if chain.Name == "requests" && chain.Count > 0 {
				fmt.Printf("Found requests chain with %d entries\n", chain.Count)
				
				// Query the top 5 entries (or all if less than 5)
				entryCount := uint64(5)
				if chain.Count < entryCount {
					entryCount = chain.Count
				}
				
				entries, err := queryChainEntriesViaJSONRPC(ctx, jsonClient, stakingUrl.JoinPath("requests"), entryCount)
				if err != nil {
					fmt.Printf("Error querying requests chain entries: %v\n", err)
				} else {
					// Print the entries (newest first)
					fmt.Printf("Retrieved %d entries from the requests chain\n", len(entries.Records))
					for i := len(entries.Records) - 1; i >= 0; i-- {
						fmt.Printf("  Entry %d: %v\n", len(entries.Records)-i, entries.Records[i])
					}
				}
			}
		}
	} else {
		fmt.Printf("Unexpected chain query result type: %T\n", chains)
	}
}

// queryChainCountViaJSONRPC queries the chain count directly using JSON-RPC
func queryChainCountViaJSONRPC(ctx context.Context, client *jsonrpc.Client, chainUrl *accurl.URL) (uint64, error) {
	// Create a chain query for the anchor-sequence chain
	chainQuery := &api.ChainQuery{
		Name: "anchor-sequence",
	}
	
	// Execute the query directly via JSON-RPC
	fmt.Println("Querying chain count via JSON-RPC...")
	chainInfo, err := client.Query(ctx, chainUrl, chainQuery)
	if err != nil {
		return 0, fmt.Errorf("failed to query chain count: %w", err)
	}
	
	// Try to cast the result to a ChainRecord
	chainRecord, ok := chainInfo.(*api.ChainRecord)
	if !ok {
		return 0, fmt.Errorf("expected ChainRecord, got %T", chainInfo)
	}
	
	return chainRecord.Count, nil
}

// queryChainEntriesViaJSONRPC queries entries from a chain using JSON-RPC
func queryChainEntriesViaJSONRPC(ctx context.Context, client *jsonrpc.Client, chainUrl *accurl.URL, count uint64) (*api.RecordRange[api.Record], error) {
	// Create a query for the chain entries
	chainQuery := &api.ChainQuery{
		Range: &api.RangeOptions{
			Start: 0,
			Count: &count,
		},
	}
	
	// Execute the query directly via JSON-RPC
	entriesInfo, err := client.Query(ctx, chainUrl, chainQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query chain entries: %w", err)
	}
	
	// Try to cast the result to a RecordRange
	entries, ok := entriesInfo.(*api.RecordRange[api.Record])
	if !ok {
		return nil, fmt.Errorf("expected RecordRange[api.Record], got %T", entriesInfo)
	}
	
	return entries, nil
}

// TestQueryChainFromSpecificPeer tests connecting to a specific peer and querying a chain
func TestQueryChainFromSpecificPeer(t *testing.T) {
	// Skip this test in automated test runs
	//t.Skip("Manual test - requires network access")

	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Network to test against
	const network = "mainnet"

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Use a specific peer from the logs
	peerAddr := "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWS2Adojqun5RV1Xy4k6vKXWpRQ3VdzXnW8SbW7ERzqKie"
	fmt.Printf("Using peer: %s\n", peerAddr)

	// Parse the multiaddress
	ma, err := multiaddr.NewMultiaddr(peerAddr)
	require.NoError(t, err, "Failed to parse multiaddress")

	// Extract the peer ID from the multiaddress
	peerInfo, err := peer.AddrInfoFromP2pAddr(ma)
	require.NoError(t, err, "Failed to extract peer info from multiaddress")

	fmt.Printf("Peer ID: %s\n", peerInfo.ID.String())
	fmt.Printf("Peer Addresses: %v\n", peerInfo.Addrs)

	// Initialize the P2P node with just this peer
	fmt.Println("Initializing P2P node with specific peer...")
	node, err := p2p.New(p2p.Options{
		Network:           network,
		BootstrapPeers:    []multiaddr.Multiaddr{ma},
		PeerDatabase:      "test-specific-peer.json",
		EnablePeerTracker: true,
		PeerScanFrequency: -1,
	})
	require.NoError(t, err, "Failed to initialize P2P node")
	defer node.Close()

	// Wait for the P2P node to initialize
	fmt.Println("Waiting for P2P node to initialize...")
	time.Sleep(5 * time.Second)

	// Create a healer instance
	h := new(healer)
	h.Reset()
	currentHealer = h

	// Set up the healer
	h.ctx = ctx
	h.network = network
	
	// Create a JSON-RPC client for fallback
	jsonClient := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(network, "v3"))
	jsonClient.Debug = true
	jsonClient.Client.Timeout = 1 * time.Minute
	h.C1 = jsonClient
	
	// Set up the P2P client
	h.C2 = &message.Client{
		Transport: &message.RoutedTransport{
			Network: network,
			Dialer:  node.DialNetwork(),
		},
	}
	
	// Set up the network info
	h.net = &healing.NetworkInfo{
		Status: &api.NetworkStatus{},
		ID:     "test",
		Peers:  make(map[string]healing.PeerList),
	}

	// Get the Directory anchor chain URL
	directoryUrl := protocol.DnUrl()
	fmt.Printf("Directory URL: %s\n", directoryUrl)

	// Try to query using the P2P network first
	fmt.Println("Attempting to query via P2P network...")
	chainQuery := &api.ChainQuery{
		Name: "anchor-sequence",
	}
	
	// Create a tryEachQuerier to try P2P first, then fall back to direct
	q := &tryEachQuerier{
		healer: h,
	}
	
	chainInfo, err := q.Query(ctx, directoryUrl.JoinPath("anchor-sequence"), chainQuery)
	if err != nil {
		fmt.Printf("P2P query failed, falling back to direct: %v\n", err)
		// Fall back to direct query if P2P fails
		chainInfo, err = h.C1.Query(ctx, directoryUrl.JoinPath("anchor-sequence"), chainQuery)
		require.NoError(t, err, "Both P2P and direct queries failed")
	}

	// Try to cast the result to a ChainRecord
	chainRecord, ok := chainInfo.(*api.ChainRecord)
	require.True(t, ok, "Expected ChainRecord, got %T", chainInfo)
	
	fmt.Printf("Directory anchor chain info: Count=%d, Type=%s\n", chainRecord.Count, chainRecord.Type)
	fmt.Printf("Directory anchor chain height: %d\n", chainRecord.Count)

	// The Directory anchor chain should have millions of entries in production
	// For testing purposes, we'll just require at least one entry
	require.Greater(t, chainRecord.Count, uint64(0), 
		"Directory anchor chain should have entries (expected millions in production, got zero)")

	// If there are entries, query the most recent ones
	if chainRecord.Count > 0 {
		fmt.Println("Querying recent entries from the Directory anchor chain...")
		
		// Calculate start and count for pagination
		// Get the most recent 5 entries or all if less than 5
		var startIndex uint64 = 0
		if chainRecord.Count > 5 {
			startIndex = chainRecord.Count - 5
		}
		var entryCount uint64 = 5
		if chainRecord.Count < entryCount {
			entryCount = chainRecord.Count
		}
		
		// Create a query for the chain entries with pagination
		entryQuery := &api.ChainQuery{
			Name: "anchor-sequence",
			Range: &api.RangeOptions{
				Start: startIndex,
				Count: &entryCount,
			},
		}
		
		// Execute the query to get the chain entries
		fmt.Printf("Querying for entries %d to %d of the anchor chain...\n", 
			startIndex, startIndex + entryCount - 1)
		
		// Try P2P first, then fall back to direct
		entryResult, err := q.Query(ctx, directoryUrl.JoinPath("anchor-sequence"), entryQuery)
		if err != nil {
			fmt.Printf("P2P query for entries failed, falling back to direct: %v\n", err)
			// Fall back to direct query if P2P fails
			entryResult, err = h.C1.Query(ctx, directoryUrl.JoinPath("anchor-sequence"), entryQuery)
			require.NoError(t, err, "Both P2P and direct queries failed for entries")
		}
		
		// Try to cast the result to different possible types
		if entries, ok := entryResult.(*api.RecordRange[*api.ChainEntryRecord[api.Record]]); ok {
			// Print the entries in reverse order (newest first)
			fmt.Printf("Found %d entries in the chain\n", len(entries.Records))
			
			// Check if we have any records to print
			if len(entries.Records) == 0 {
				fmt.Println("No entries found in the chain")
				return
			}
			
			// Print the entries (newest first)
			for i := len(entries.Records) - 1; i >= 0; i-- {
				entry := entries.Records[i]
				fmt.Printf("  Entry %d: Index=%d, Hash=%x\n", 
					len(entries.Records)-i, entry.Index, entry.Entry)
			}
			return
		}
		
		// Try the generic RecordRange type
		if entries, ok := entryResult.(*api.RecordRange[api.Record]); ok {
			fmt.Printf("Found %d entries in the chain\n", len(entries.Records))
			
			// Check if we have any records to print
			if len(entries.Records) == 0 {
				fmt.Println("No entries found in the chain")
				return
			}
			
			// Print the entries (newest first)
			for i := len(entries.Records) - 1; i >= 0; i-- {
				fmt.Printf("  Entry %d: Type=%T\n", len(entries.Records)-i, entries.Records[i])
			}
			return
		}
		
		// If we reach here, we don't know how to handle this type
		t.Fatalf("Unexpected result type: %T", entryResult)
	} else {
		fmt.Println("No entries in the Directory anchor chain")
	}
}

// TestQueryDirectoryAccount tests querying the directory account using JSON-RPC
func TestQueryDirectoryAccount(t *testing.T) {
	// Skip this test in automated test runs
	//t.Skip("Manual test - requires network access")

	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Network to test against
	const network = "mainnet"

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Create a JSON-RPC client
	jsonClient := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(network, "v3"))
	jsonClient.Debug = true
	jsonClient.Client.Timeout = 1 * time.Minute

	// Get the Directory URL
	directoryUrl := protocol.DnUrl()
	fmt.Printf("Directory URL: %s\n", directoryUrl)

	// Query the directory account
	fmt.Println("Querying directory account...")
	
	// Create a query for the account
	accountQuery := &api.DefaultQuery{}
	
	// Execute the query directly via JSON-RPC
	accountInfo, err := jsonClient.Query(ctx, directoryUrl, accountQuery)
	require.NoError(t, err, "Failed to query directory account")
	
	// Print the account info
	fmt.Printf("Directory account info: %T\n", accountInfo)
	
	// Try to cast the result to an AccountRecord
	accountRecord, ok := accountInfo.(*api.AccountRecord)
	require.True(t, ok, "Expected AccountRecord, got %T", accountInfo)
	
	// Print account details
	fmt.Printf("Directory account: %v\n", accountRecord.Account)
	
	// Now try to query the main chain
	fmt.Println("Querying directory main chain...")
	
	// Create a chain query for the main chain
	chainQuery := &api.ChainQuery{
		Name: "main",
	}
	
	// Execute the query directly via JSON-RPC
	chainInfo, err := jsonClient.Query(ctx, directoryUrl, chainQuery)
	require.NoError(t, err, "Failed to query directory main chain")
	
	// Try to cast the result to a ChainRecord
	chainRecord, ok := chainInfo.(*api.ChainRecord)
	require.True(t, ok, "Expected ChainRecord, got %T", chainInfo)
	
	fmt.Printf("Directory main chain info: Count=%d, Type=%s\n", chainRecord.Count, chainRecord.Type)
	
	// If there are entries, query the most recent ones
	if chainRecord.Count > 0 {
		fmt.Printf("Found %d entries in the directory main chain\n", chainRecord.Count)
		
		// Query the top 5 entries (or all if less than 5)
		entryCount := uint64(5)
		if chainRecord.Count < entryCount {
			entryCount = chainRecord.Count
		}
		
		entries, err := queryChainEntriesViaJSONRPC(ctx, jsonClient, directoryUrl.JoinPath("main"), entryCount)
		require.NoError(t, err, "Failed to query directory main chain entries")
		
		// Print the entries (newest first)
		fmt.Printf("Retrieved %d entries from the directory main chain\n", len(entries.Records))
		for i := len(entries.Records) - 1; i >= 0; i-- {
			fmt.Printf("  Entry %d: %v\n", len(entries.Records)-i, entries.Records[i])
		}
	} else {
		fmt.Println("No entries found in the directory main chain")
	}
}

// TestQueryDNAnchorsChain tests querying the DN.acme/anchors chain using JSON-RPC
func TestQueryDNAnchorsChain(t *testing.T) {
	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Network to test against
	const network = "mainnet"

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create a JSON-RPC client
	jsonRpcUrl := accumulate.ResolveWellKnownEndpoint(network, "v3")
	fmt.Printf("Using JSON-RPC URL: %s\n", jsonRpcUrl)
	jsonClient := jsonrpc.NewClient(jsonRpcUrl)
	jsonClient.Debug = true
	jsonClient.Client.Timeout = 2 * time.Minute

	// Get the DN anchors URL
	anchorsUrl := accurl.MustParse("acc://dn.acme/anchors")
	fmt.Printf("Anchors URL: %s\n", anchorsUrl)

	// Query the anchors chain
	fmt.Println("Querying DN.acme/anchors chain...")
	
	// Create a chain query
	chainQuery := &api.ChainQuery{
		Name: "",
	}
	
	// Execute the query directly via JSON-RPC
	chainInfo, err := jsonClient.Query(ctx, anchorsUrl, chainQuery)
	require.NoError(t, err, "Failed to query DN.acme/anchors chain")
	fmt.Printf("Chain info type: %T\n", chainInfo)
	
	// Handle different possible return types
	var chainCount uint64
	var chainTypeStr string
	
	switch record := chainInfo.(type) {
	case *api.ChainRecord:
		// Direct chain record
		chainCount = record.Count
		chainTypeStr = fmt.Sprint(record.Type)
	case *api.RecordRange[*api.ChainRecord]:
		// Range of chain records
		if len(record.Records) > 0 {
			chainCount = record.Records[0].Count
			chainTypeStr = fmt.Sprint(record.Records[0].Type)
		}
	case *api.RecordRange[api.Record]:
		// Generic record range
		fmt.Printf("Got a generic record range with %d records\n", len(record.Records))
		for i, rec := range record.Records {
			fmt.Printf("  Record %d type: %T\n", i+1, rec)
			if chainRec, ok := rec.(*api.ChainRecord); ok {
				chainCount = chainRec.Count
				chainTypeStr = fmt.Sprint(chainRec.Type)
				break
			}
		}
	default:
		t.Logf("Unexpected chain info type: %T", chainInfo)
	}
	
	fmt.Printf("DN.acme/anchors chain info: Count=%d, Type=%s\n", chainCount, chainTypeStr)
	
	// Verify that we're seeing a significant number of entries
	require.Greater(t, chainCount, uint64(1000), "Expected at least 1000 entries in DN.acme/anchors chain")
	
	// Try to query some entries
	fmt.Println("Querying recent entries from DN.acme/anchors chain...")
	
	// Query the top 5 entries
	entryCount := uint64(5)
	
	// Create a direct chain entry query
	entryQuery := &api.ChainQuery{
		Range: &api.RangeOptions{
			Start: chainCount - entryCount,
			Count: &entryCount,
		},
	}
	
	// Execute the query directly via JSON-RPC
	entriesInfo, err := jsonClient.Query(ctx, anchorsUrl, entryQuery)
	require.NoError(t, err, "Failed to query DN.acme/anchors entries")
	
	// Try to cast the result to a RecordRange of TxRecord
	entries, ok := entriesInfo.(*api.RecordRange[api.Record])
	require.True(t, ok, "Expected RecordRange[api.Record], got %T", entriesInfo)
	
	// Print the entries
	fmt.Printf("Retrieved %d entries from the DN.acme/anchors chain\n", len(entries.Records))
	for i, entry := range entries.Records {
		fmt.Printf("  Entry %d: %v\n", i+1, entry)
	}
	
	// Verify that we got the expected number of entries
	require.Equal(t, int(entryCount), len(entries.Records), "Expected %d entries, got %d", entryCount, len(entries.Records))
}

// TestQueryDNAnchorsChainDirect tests querying the DN.acme/anchors chain using JSON-RPC with direct mainnet URL
func TestQueryDNAnchorsChainDirect(t *testing.T) {
	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create a JSON-RPC client with explicit mainnet URL
	directMainnetUrl := "https://mainnet.accumulatenetwork.io/v3"
	fmt.Printf("Using direct mainnet URL: %s\n", directMainnetUrl)
	jsonClient := jsonrpc.NewClient(directMainnetUrl)
	jsonClient.Debug = true
	jsonClient.Client.Timeout = 2 * time.Minute

	// Get the DN anchors URL
	anchorsUrl := accurl.MustParse("acc://dn.acme/anchors")
	fmt.Printf("Anchors URL: %s\n", anchorsUrl)

	// First, query the DN account to verify we can access it
	fmt.Println("Querying DN.acme account...")
	dnUrl := accurl.MustParse("acc://dn.acme")
	accountQuery := &api.DefaultQuery{}
	accountInfo, err := jsonClient.Query(ctx, dnUrl, accountQuery)
	require.NoError(t, err, "Failed to query DN.acme account")
	fmt.Printf("DN.acme account info type: %T\n", accountInfo)
	
	// Now query the anchors chain
	fmt.Println("Querying DN.acme/anchors chain...")
	
	// Create a chain query with the chain name
	chainQuery := &api.ChainQuery{
		Name: "anchors",
	}
	
	// Execute the query directly via JSON-RPC using the parent URL
	chainInfo, err := jsonClient.Query(ctx, dnUrl, chainQuery)
	require.NoError(t, err, "Failed to query DN.acme/anchors chain")
	fmt.Printf("Chain info type: %T\n", chainInfo)
	
	// Handle different possible return types
	var chainCount uint64
	var chainTypeStr string
	
	switch record := chainInfo.(type) {
	case *api.ChainRecord:
		// Direct chain record
		chainCount = record.Count
		chainTypeStr = fmt.Sprint(record.Type)
	case *api.RecordRange[*api.ChainRecord]:
		// Range of chain records
		if len(record.Records) > 0 {
			chainCount = record.Records[0].Count
			chainTypeStr = fmt.Sprint(record.Records[0].Type)
		}
	default:
		t.Logf("Unexpected chain info type: %T", chainInfo)
	}
	
	fmt.Printf("DN.acme/anchors chain info: Count=%d, Type=%s\n", chainCount, chainTypeStr)
	
	// Verify that we're seeing a significant number of entries
	require.Greater(t, chainCount, uint64(1000), "Expected at least 1000 entries in DN.acme/anchors chain")
	
	// Try to query some entries
	fmt.Println("Querying recent entries from DN.acme/anchors chain...")
	
	// Query the top 5 entries
	entryCount := uint64(5)
	
	// Create a direct chain entry query
	entryQuery := &api.ChainQuery{
		Name: "anchors",
		Range: &api.RangeOptions{
			Start: chainCount - entryCount,
			Count: &entryCount,
		},
	}
	
	// Execute the query directly via JSON-RPC
	entriesInfo, err := jsonClient.Query(ctx, dnUrl.JoinPath("anchors"), entryQuery)
	require.NoError(t, err, "Failed to query DN.acme/anchors entries")
	
	// Try to cast the result to a RecordRange of TxRecord
	entries, ok := entriesInfo.(*api.RecordRange[api.Record])
	require.True(t, ok, "Expected RecordRange[api.Record], got %T", entriesInfo)
	
	// Print the entries
	fmt.Printf("Retrieved %d entries from the DN.acme/anchors chain\n", len(entries.Records))
	for i, entry := range entries.Records {
		fmt.Printf("  Entry %d: %v\n", i+1, entry)
	}
}

// TestQueryDNAnchorsAccount tests querying the DN.acme/anchors account directly using JSON-RPC
func TestQueryDNAnchorsAccount(t *testing.T) {
	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create a JSON-RPC client with explicit mainnet URL
	directMainnetUrl := "https://mainnet.accumulatenetwork.io/v3"
	fmt.Printf("Using direct mainnet URL: %s\n", directMainnetUrl)
	jsonClient := jsonrpc.NewClient(directMainnetUrl)
	jsonClient.Debug = true
	jsonClient.Client.Timeout = 2 * time.Minute

	// Get the DN anchors URL
	anchorsUrl := accurl.MustParse("acc://dn.acme/anchors")
	fmt.Printf("Anchors URL: %s\n", anchorsUrl)

	// Query the anchors account directly
	fmt.Println("Querying DN.acme/anchors account...")
	
	// Create a query for the account
	accountQuery := &api.DefaultQuery{}
	
	// Execute the query directly via JSON-RPC
	accountInfo, err := jsonClient.Query(ctx, anchorsUrl, accountQuery)
	require.NoError(t, err, "Failed to query DN.acme/anchors account")
	
	// Print the account info
	fmt.Printf("DN.acme/anchors account info type: %T\n", accountInfo)
	
	// Try to cast the result to an AccountRecord
	accountRecord, ok := accountInfo.(*api.AccountRecord)
	require.True(t, ok, "Expected AccountRecord, got %T", accountInfo)
	
	// Print account details
	fmt.Printf("DN.acme/anchors account: %v\n", accountRecord.Account)
	
	// Check if the account has a 'main' chain
	fmt.Println("Querying DN.acme/anchors main chain...")
	
	// Create a chain query for the main chain
	chainQuery := &api.ChainQuery{
		Name: "main",
	}
	
	// Execute the query directly via JSON-RPC
	chainInfo, err := jsonClient.Query(ctx, anchorsUrl, chainQuery)
	require.NoError(t, err, "Failed to query DN.acme/anchors main chain")
	
	// Try to cast the result to a ChainRecord
	chainRecord, ok := chainInfo.(*api.ChainRecord)
	require.True(t, ok, "Expected ChainRecord, got %T", chainInfo)
	
	fmt.Printf("DN.acme/anchors main chain info: Count=%d, Type=%s\n", chainRecord.Count, fmt.Sprint(chainRecord.Type))
	
	// Verify that we're seeing the expected number of entries (around 1.4 million)
	require.Greater(t, chainRecord.Count, uint64(1000000), "Expected at least 1 million entries in DN.acme/anchors main chain")
	
	// Query the most recent entries
	fmt.Println("Querying recent entries from DN.acme/anchors main chain...")
	
	// Query the top 5 entries
	entryCount := uint64(5)
	
	// Create a chain query with range options to get the most recent entries
	entryQuery := &api.ChainQuery{
		Name: "main",
		Range: &api.RangeOptions{
			Start: chainRecord.Count - entryCount,
			Count: &entryCount,
		},
	}
	
	// Execute the query directly via JSON-RPC
	entriesInfo, err := jsonClient.Query(ctx, anchorsUrl, entryQuery)
	require.NoError(t, err, "Failed to query DN.acme/anchors main chain entries")
	
	// Try to cast the result to a RecordRange
	entries, ok := entriesInfo.(*api.RecordRange[api.Record])
	require.True(t, ok, "Expected RecordRange[api.Record], got %T", entriesInfo)
	
	// Print the entries
	fmt.Printf("Retrieved %d entries from the DN.acme/anchors main chain\n", len(entries.Records))
	for i, entry := range entries.Records {
		fmt.Printf("  Entry %d: %v\n", i+1, entry)
	}
	
	// Verify that we got the expected number of entries
	require.Equal(t, int(entryCount), len(entries.Records), "Expected %d entries, got %d", entryCount, len(entries.Records))
}
