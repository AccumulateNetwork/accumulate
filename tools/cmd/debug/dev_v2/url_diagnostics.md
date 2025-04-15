# URL Diagnostics Table Implementation Guide

## Overview

The URL diagnostics table provides transparency into how URLs are constructed for each partition pair, helping to identify inconsistencies or errors in URL construction. This document outlines the implementation details for the URL diagnostics table in the `reportChainHeightDifferences` function in `heal_synth.go`.

## Chain Types and Healing Behavior

For a detailed explanation of the different chain types (root chains, synthetic sequence chains, anchor chains) and how they are handled during the healing process, please refer to the [chains_urls.md](../heal_v1/chains_urls.md) document. This document explains:

1. Which chains are synced between networks and which are not
2. The purpose and construction of different chain types
3. How the healing process treats different chain types
4. Implications for URL construction and diagnostics

## Key Requirements

1. Each partition pair (source → destination) should have only one entry in the URL diagnostics table
2. The table should clearly show the base URL construction for both source and destination partitions
3. For each partition pair, we should display information about all relevant chains
4. The table should indicate which chains are actually healed (synthetic chains) and which are just checked for diagnostics (root chains are not healed)

## Data Structure

```go
// Structure to hold URL diagnostic information for a partition pair
type urlDiagnostic struct {
    srcPartition  string
    dstPartition  string
    baseUrl       struct {
        src string
        dst string
    }
    chains        map[string]struct {
        srcPath      string
        dstPath      string
        queryResult  string  // Success, Height Diff, Not Found, Error, etc.
        isHealed     bool    // Whether this chain type is healed
    }
}
```

## Collection Process

The collection process should:

1. Iterate through all partition pairs (source → destination)
2. Create a single diagnostic entry for each partition pair
3. Add information about each chain type to the entry's chains map
4. Execute queries against the chains to fill in the queryResult field
5. Collect all diagnostics into a single array for reporting

```go
// Pseudocode for collection process
var diagnostics []urlDiagnostic

// For each partition pair
for _, src := range partitions {
    srcUrl := protocol.PartitionUrl(src)
    
    for _, dst := range partitions {
        if src == dst {
            continue
        }
        
        dstUrl := protocol.PartitionUrl(dst)
        
        // Create a single diagnostic entry for this partition pair
        diag := urlDiagnostic{
            srcPartition: src,
            dstPartition: dst,
            baseUrl: struct {
                src string
                dst string
            }{
                src: srcUrl.String(),
                dst: dstUrl.String(),
            },
            chains: make(map[string]struct {
                srcPath      string
                dstPath      string
                queryResult  string
                isHealed     bool
            }),
        }
        
        // Add main chain (not healed, diagnostic only)
        diag.chains["main"] = struct {
            srcPath      string
            dstPath      string
            queryResult  string
            isHealed     bool
        }{
            srcPath:     "",  // Base URL
            dstPath:     "",  // Base URL
            queryResult: "Pending",
            isHealed:    false,
        }
        
        // Add synthetic chains (these are healed)
        synthPath := protocol.Synthetic
        for _, seqName := range []string{"synthetic-sequence-0", "synthetic-sequence-1", "synthetic-sequence-index"} {
            diag.chains[seqName] = struct {
                srcPath      string
                dstPath      string
                queryResult  string
                isHealed     bool
            }{
                srcPath:     synthPath,
                dstPath:     synthPath,
                queryResult: "Pending",
                isHealed:    true,
            }
        }
        
        // Add root chain (not healed, diagnostic only)
        diag.chains["root"] = struct {
            srcPath      string
            dstPath      string
            queryResult  string
            isHealed     bool
        }{
            srcPath:     protocol.Ledger,
            dstPath:     protocol.Ledger,
            queryResult: "Pending",
            isHealed:    false,
        }
        
        // Execute queries to fill in queryResult
        for chainName, chain := range diag.chains {
            // Query source chain
            srcChainUrl := srcUrl
            if chain.srcPath != "" {
                srcChainUrl = srcUrl.JoinPath(chain.srcPath)
            }
            
            srcChain, err := h.tryEach().QueryChain(ctx, srcChainUrl, &api.ChainQuery{Name: chainName})
            if err != nil || srcChain == nil {
                chain.queryResult = fmt.Sprintf("Source Error: %v", err)
                continue
            }
            
            // Query destination chain
            dstChainUrl := dstUrl
            if chain.dstPath != "" {
                dstChainUrl = dstUrl.JoinPath(chain.dstPath)
            }
            
            dstChain, err := h.tryEach().QueryChain(ctx, dstChainUrl, &api.ChainQuery{Name: chainName})
            if err != nil || dstChain == nil {
                chain.queryResult = fmt.Sprintf("Dst Error: %v", err)
                continue
            }
            
            // Calculate the differential
            if srcChain.Count > dstChain.Count {
                chain.queryResult = fmt.Sprintf("Height Diff: %d", srcChain.Count - dstChain.Count)
            } else {
                chain.queryResult = "No Diff"
            }
            
            // Update the chain in the map
            diag.chains[chainName] = chain
        }
        
        diagnostics = append(diagnostics, diag)
    }
}
```

## Reporting Format

The reporting format should:

1. Display a clear header with column names
2. For each partition pair:
   - Show the base URLs for source and destination
   - Show information about each chain type
   - Indicate which chains are healed and which are not
3. Use a separator between partition pairs for better readability

```go
// Pseudocode for reporting format
fmt.Printf("\n============= URL DIAGNOSTICS REPORT =============\n")
fmt.Printf("%-15s %-15s %-20s %-50s %-50s %-15s %-10s\n", 
    "Source", "Destination", "Chain", "Source URL", "Destination URL", "Result", "Healed")
fmt.Printf("%s\n", strings.Repeat("-", 175))

for _, diag := range diagnostics {
    // Print base URLs first
    fmt.Printf("%-15s %-15s %-20s %-50s %-50s %-15s %-10s\n",
        diag.srcPartition,
        diag.dstPartition,
        "BASE",
        diag.baseUrl.src,
        diag.baseUrl.dst,
        "N/A",
        "N/A")
        
    // Then print each chain
    for chainName, chain := range diag.chains {
        srcUrl := diag.baseUrl.src
        dstUrl := diag.baseUrl.dst
        
        if chain.srcPath != "" {
            srcUrl = fmt.Sprintf("%s/%s", srcUrl, chain.srcPath)
        }
        
        if chain.dstPath != "" {
            dstUrl = fmt.Sprintf("%s/%s", dstUrl, chain.dstPath)
        }
        
        healedStr := "No"
        if chain.isHealed {
            healedStr = "Yes"
        }
        
        fmt.Printf("%-15s %-15s %-20s %-50s %-50s %-15s %-10s\n",
            "",  // No need to repeat partition names
            "",
            chainName,
            srcUrl,
            dstUrl,
            chain.queryResult,
            healedStr)
    }
    
    // Add a separator between partition pairs
    fmt.Printf("%s\n", strings.Repeat("-", 175))
}
```

## Integration with Chain Height Differences Report

The URL diagnostics table should be integrated with the chain height differences report:

1. The URL diagnostics table should be displayed after the chain height differences report
2. Both reports should use the same data collection process to avoid redundant queries
3. The URL diagnostics table should provide additional context for the chain height differences report

## Example Output

```
============= URL DIAGNOSTICS REPORT =============
Source          Destination    Chain                Source URL                                        Destination URL                                  Result          Healed    
-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
bvn-Apollo      bvn-Betelgeuse BASE                 acc://bvn-Apollo.acme                             acc://bvn-Betelgeuse.acme                        N/A             N/A       
                               main                 acc://bvn-Apollo.acme                             acc://bvn-Betelgeuse.acme                        No Diff         No        
                               synthetic-sequence-0 acc://bvn-Apollo.acme/synthetic                   acc://bvn-Betelgeuse.acme/synthetic              Height Diff: 5  Yes       
                               synthetic-sequence-1 acc://bvn-Apollo.acme/synthetic                   acc://bvn-Betelgeuse.acme/synthetic              No Diff         Yes       
                               synthetic-sequence-index acc://bvn-Apollo.acme/synthetic               acc://bvn-Betelgeuse.acme/synthetic              No Diff         Yes       
                               root                 acc://bvn-Apollo.acme/ledger                      acc://bvn-Betelgeuse.acme/ledger                 No Diff         No        
-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
bvn-Apollo      dn             BASE                 acc://bvn-Apollo.acme                             acc://dn.acme                                    N/A             N/A       
                               main                 acc://bvn-Apollo.acme                             acc://dn.acme                                    No Diff         No        
                               synthetic-sequence-0 acc://bvn-Apollo.acme/synthetic                   acc://dn.acme/synthetic                          Height Diff: 3  Yes       
                               synthetic-sequence-1 acc://bvn-Apollo.acme/synthetic                   acc://dn.acme/synthetic                          No Diff         Yes       
                               synthetic-sequence-index acc://bvn-Apollo.acme/synthetic               acc://dn.acme/synthetic                          No Diff         Yes       
                               root                 acc://bvn-Apollo.acme/ledger                      acc://dn.acme/ledger                             No Diff         No        
-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
```

## Implementation Notes

1. The implementation should handle errors gracefully, especially when querying chains that don't exist
2. The code should be optimized to minimize redundant queries
3. The reporting format should be consistent and easy to read
4. The implementation should clearly indicate which chains are healed and which are not
5. The URL construction should be consistent with the rest of the codebase

## Next Steps

1. Update the `reportChainHeightDifferences` function in `heal_synth.go` to implement this design
2. Test the implementation with various partition configurations
3. Verify that the URL diagnostics table correctly identifies URL construction issues
4. Ensure that the implementation is consistent with the rest of the codebase
