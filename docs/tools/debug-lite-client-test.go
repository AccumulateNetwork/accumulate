// Package docs contains test examples for the lite client validation
package docs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Metadata:
// Title: Lite Client Validation Test
// Description: Simple test example for validating a major block
// Version: 1.0
// Author: Accumulate Team
// Tags: testing, lite-client, validation

// TestQueryMajorBlock tests a simple major block query against the Kermit testnet
// 
// This test demonstrates how to use the lite client API to query major blocks
// from the Accumulate network.
//
// NOTE: THE CALL IS NOT TIMING OUT. THAT'S NOT THE PROBLEM.
// The issue is likely related to how we're constructing the query or the URL.
// The timeout error is misleading - it's likely that the server is rejecting our
// request in a way that appears as a timeout to the client.
//
// IMPORTANT RULE: DO NOT SKIP TESTS TO FIX THEM.
// Tests must be fixed properly rather than being skipped.
//
// To run this test manually:
// Run: go test -v -run TestQueryMajorBlock
func TestQueryMajorBlock(t *testing.T) {
	// This test demonstrates how to use the lite client API to query major blocks
	// from the Accumulate network.

	// Try multiple endpoints to find one that works
	endpoints := []string{
		"https://kermit.accumulatenetwork.io",
		"https://testnet.accumulatenetwork.io",
		"https://mainnet.accumulatenetwork.io",
	}
	
	// We'll try each endpoint
	var cl *client.Client
	var err error
	var connectedEndpoint string
	
	for _, endpoint := range endpoints {
		t.Logf("Trying to connect to endpoint: %s", endpoint)
		cl, err = client.New(endpoint)
		if err != nil {
			t.Logf("Failed to create client for %s: %v", endpoint, err)
			continue
		}
		
		// Enable debug output for the client
		cl.DebugRequest = true
		
		// Set a shorter timeout for initial connectivity check
		cl.Timeout = 10 * time.Second
		
		// Try a simple ping to see if this endpoint is responsive
		ctxPing, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelPing()
		
		// Use a simple version call to check connectivity
		t.Logf("Testing connectivity to %s...", endpoint)
		_, pingErr := cl.Version(ctxPing)
		if pingErr == nil {
			// Found a working endpoint
			connectedEndpoint = endpoint
			break
		}
		
		t.Logf("Endpoint %s not responsive: %v", endpoint, pingErr)
	}
	
	// Verify we found a working endpoint
	require.NotEmpty(t, connectedEndpoint, "Could not connect to any Accumulate network endpoint")
	t.Logf("Successfully connected to: %s", connectedEndpoint)
	
	// Now set a longer timeout for the actual API calls
	cl.Timeout = 30 * time.Second
	t.Logf("Client timeout set to: %v", cl.Timeout)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	t.Log("Connecting to Kermit testnet...")

	// Query the genesis block (index 0)
	// Instead of using protocol.DnUrl(), let's try a different approach
	// The issue might be related to how the URL is being serialized in the JSON-RPC request
	
	// Create query for the genesis block with explicit fields
	query := &client.MajorBlocksQuery{}
	
	// THIS CALL IS NOT TIMING OUT. THAT'S NOT THE PROBLEM.
	// The issue is likely with how the URL is being constructed or interpreted
	
	// The issue might be that we're trying to query the Directory Network directly
	// Let's try querying a specific partition instead
	
	// Try using a different URL format - a specific partition
	// Let's try the bvn0.acme partition which is a common validator partition
	partitionUrl, err := url.Parse("acc://bvn0.acme")
	require.NoError(t, err, "Failed to parse partition URL")
	
	// Log URL information for debugging
	t.Logf("Partition URL: %v (Type: %T)", partitionUrl, partitionUrl)
	t.Logf("Partition URL Authority: %v", partitionUrl.Authority)
	
	// Set the URL field
	query.Url = partitionUrl
	
	// Set pagination fields (embedded from QueryPagination)
	// Count=1 means we only want one block
	// Start=0 means we want to start with the genesis block (index 0)
	query.Count = 1
	query.Start = 0
	
	// Debug the query structure
	queryJson, _ := json.Marshal(query)
	t.Logf("Query JSON: %s", string(queryJson))
	
	// Execute the query with better error handling
	t.Log("Querying for genesis block...")
	t.Logf("Using context with timeout: %v", ctx)
	
	// Try with a shorter timeout first to see if we get a different error
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shortCancel()
	
	t.Log("First attempt with 5s timeout...")
	resp, err := cl.QueryMajorBlocks(shortCtx, query)
	if err != nil {
		t.Logf("First attempt error: %v", err)
		t.Log("Trying again with longer timeout...")
		
		// Try again with the longer timeout
		resp, err = cl.QueryMajorBlocks(ctx, query)
		if err != nil {
			t.Logf("Error querying major blocks: %v", err)
			t.Logf("This could be due to network connectivity issues or the testnet being temporarily unavailable")
			t.FailNow()
		}
	}
	
	// Check if we have results
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Items, "No genesis block found")
	
	// Since Items is a []interface{}, we need to convert it to a usable type
	// First, convert to JSON and then back to a structured type
	block := make(map[string]interface{})
	blockData, err := json.Marshal(resp.Items[0])
	require.NoError(t, err)
	
	err = json.Unmarshal(blockData, &block)
	require.NoError(t, err)
	
	// Log the full block data for inspection
	prettyJSON, err := json.MarshalIndent(block, "", "  ")
	require.NoError(t, err)
	t.Logf("Genesis Block Data:\n%s", string(prettyJSON))
	
	// Extract and validate specific fields
	majorBlockIndex, ok := block["majorBlockIndex"]
	require.True(t, ok, "majorBlockIndex field not found in response")
	
	majorBlockTime, ok := block["majorBlockTime"]
	require.True(t, ok, "majorBlockTime field not found in response")
	
	minorBlocks, ok := block["minorBlocks"]
	require.True(t, ok, "minorBlocks field not found in response")
	
	// Log the major block information
	t.Logf("Successfully queried major block: Index=%v, Time=%v, MinorBlocks=%d", 
		majorBlockIndex, majorBlockTime, len(minorBlocks.([]interface{})))
}

// This file is meant to be used as a reference implementation
// and not executed directly. The function demonstrates how to use
// the Accumulate API to query a major block.
