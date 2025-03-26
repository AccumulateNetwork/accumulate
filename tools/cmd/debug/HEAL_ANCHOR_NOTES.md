# Anchor Healing Implementation Notes

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (that follows) will not be deleted.

## Overview

This document captures critical observations and implementation details for the anchor healing process in Accumulate. It serves as a reference to prevent regression and ensure consistent behavior across the codebase.

## Critical Findings

### URL Construction Differences

1. **Issue Identified:** There is a fundamental difference in how URLs are constructed between sequence.go and heal_anchor.go:
   - sequence.go uses raw partition URLs for tracking (e.g., `acc://bvn-Apollo.acme`)
   - heal_anchor.go was incorrectly appending the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

2. **Impact:** This discrepancy causes anchor healing to fail because:
   - The code looks for anchors at different URL paths
   - Queries return "element does not exist" errors when checking the wrong URL format
   - Anchor relationships are not properly maintained between partitions

3. **Resolution:** 
   - Standardize on the sequence.go approach (using raw partition URLs)
   - Update heal_anchor.go to use the same URL construction logic
   - Ensure all related code (including the caching system) is aware of the correct URL format

### Anchor Chain Lookup

1. **Correct Method:** Use `anchorLedger.Anchor(srcUrl)` to get the tracking information
   - This matches the approach used in sequence.go
   - This avoids manual iteration through the sequence array which can be error-prone

2. **URL Construction for Queries:**
   - For the source partition's own anchor chain: `srcUrl.JoinPath(protocol.AnchorPool)`
   - For tracking other partitions: Use the raw partition URL directly

### URL Comparison Table

The table below shows the correct URL construction for different partition combinations:

| Partition | URL Type | Correct Format (sequence.go) | Incorrect Format (old heal_anchor.go) |
|-----------|----------|------------------------------|--------------------------------------|
| Directory | Own | acc://dn.acme/anchors | acc://dn.acme/anchors |
| Directory | Tracking Apollo | acc://bvn-Apollo.acme | acc://dn.acme/anchors/Apollo |
| Apollo | Own | acc://bvn-Apollo.acme/anchors | acc://bvn-Apollo.acme/anchors |
| Apollo | Tracking Directory | acc://dn.acme | acc://bvn-Apollo.acme/anchors/Directory |

## Implementation Guidelines

1. **Finding Anchor Chains:**
   ```go
   // Correct way to get the anchor ledger
   anchorLedgerUrl := dstUrl.JoinPath(protocol.AnchorPool)
   anchorAccount, err := client.QueryAccount(ctx, anchorLedgerUrl, nil)
   
   // Correct way to find the anchor chain for the source partition
   anchorChain := anchorLedger.Anchor(srcUrl)
   ```

2. **Querying Chain Entries:**
   ```go
   // Correct URL for querying anchor chain entries
   anchorChainUrl := srcUrl.JoinPath(protocol.AnchorPool)
   ```

3. **Healing Anchors:**
   - Use the partition IDs for logging and tracking
   - Use the URL objects for actual queries and lookups

## Testing

1. **Verification Method:** Use the anchor_height_test.go to compare URL construction
2. **Expected Results:** All URLs should match the sequence.go approach
3. **Common Failure Modes:**
   - "element does not exist" errors when querying anchor chains
   - Missing anchors that should be present
   - Inconsistent anchor heights between partitions

## Troubleshooting

1. **Missing Anchors:**
   - Verify the URL construction is correct
   - Check that the anchor chain exists at the expected URL
   - Ensure the anchor ledger contains the correct tracking information

2. **Query Failures:**
   - Check for "element does not exist" errors which often indicate incorrect URL paths
   - Verify that the anchor chain name is "anchor-sequence"
   - Ensure the chain query parameters are correct

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (above) will not be deleted.
