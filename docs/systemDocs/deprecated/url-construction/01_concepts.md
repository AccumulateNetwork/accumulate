> **DEPRECATED**: This document has been moved to the new documentation structure. Please refer to the [new location](../new_structure/03_core_components/01_url_system/01_concepts.md) for the most up-to-date version.

# URL Construction and Validation

```yaml
# AI-METADATA
document_type: concept
project: accumulate_network
component: url_construction
version: current
key_concepts:
  - url_normalization
  - partition_url
  - synthetic_ledger_url
  - sequence_chain_url
  - anchor_url
related_files:
  - ../implementations/v1/chains-urls.md
  - ../implementations/v2/url-diagnostics.md
  - ../implementations/v3/development-plan.md
```

## Overview

This document serves as the authoritative source for URL construction and validation in the Accumulate network. It explains the different URL formats used throughout the codebase, the standardization decisions made, and the correct approach to constructing and validating URLs.

## URL Types in Accumulate

The Accumulate network uses several URL types for different purposes:

1. **Partition URLs**: Basic URLs that identify a partition (Block Validator Network or Directory Network)
   - Example: `acc://bvn-apollo.acme` or `acc://dn.acme`
2. **Synthetic Ledger URLs**: URLs for synthetic ledger accounts in a partition
   - Example: `acc://bvn-apollo.acme/synthetic`
3. **Sequence Chain URLs**: URLs for synthetic sequence chains between partitions
   - Example: `acc://bvn-apollo.acme/synthetic/synthetic-sequence-artemis`
4. **Anchor URLs**: URLs for anchor pools in a partition
   - Example: `acc://dn.acme/anchors`
5. **Anchor Sequence URLs**: URLs for anchor sequences between partitions
   - Example: `acc://dn.acme/anchors/apollo`

## URL Construction Issue

A fundamental issue identified in the codebase is the inconsistent approach to URL construction between different components:

| Component | Approach | Example |
|-----------|----------|---------|
| `sequence.go` | Raw partition URLs | `acc://bvn-Apollo.acme` |
| `heal_anchor.go` | Anchor pool URLs with partition ID appended | `acc://dn.acme/anchors/Apollo` |

This inconsistency causes several problems:
- Code might look for anchors at different URL paths
- Queries might return "element does not exist" errors when checking the wrong URL format
- Anchor relationships might not be properly maintained between partitions

### Standardization Decision

After analysis, the decision was made to standardize on the raw partition URL approach used in `sequence.go`. This approach is more consistent with the overall URL design in Accumulate and avoids double prefixes and other issues.

## Correct URL Construction

### Partition URLs

Partition URLs identify a specific partition (BVN or Directory Network) in the Accumulate network.

```go
// Use protocol.PartitionUrl for consistent URL construction
url := protocol.PartitionUrl(partitionID)
```

**Format**:
- BVN: `acc://bvn-{partition-name}.acme` (e.g., `acc://bvn-Apollo.acme`)
- DN: `acc://dn.acme` (no "bvn-" prefix)

**Important Notes**:
- The Directory Network (DN) is not a BVN and should not have the "bvn-" prefix
- `protocol.PartitionUrl` handles the special case for DN correctly
- Always normalize partition IDs before passing to `protocol.PartitionUrl` to avoid double prefixes

### Synthetic Ledger URLs

Synthetic ledger URLs identify synthetic ledger accounts in a partition.

```go
// Use wrapper function for consistent URL construction
url := ConstructSyntheticLedgerURL(partitionID)
```

**Format**:
- BVN: `acc://bvn-{partition-name}.acme/synthetic`
- DN: `acc://dn.acme/synthetic`

**Implementation**:

```go
func ConstructSyntheticLedgerURL(partitionID string) *url.URL {
    // Special case for Directory Network
    if strings.EqualFold(partitionID, "dn") {
        return protocol.DnUrl().JoinPath(protocol.Synthetic)
    }
    
    // For all other partitions, normalize and then use PartitionUrl
    normalizedID := NormalizePartitionID(partitionID)
    return protocol.PartitionUrl(normalizedID).JoinPath(protocol.Synthetic)
}
```

### Sequence Chain URLs

Sequence chain URLs identify synthetic sequence chains between partitions.

```go
// Use wrapper function for consistent URL construction
url := ConstructSequenceChainURL(srcPartitionID, dstPartitionID)
```

**Format**:
- BVN to BVN: `acc://bvn-{source-partition}.acme/synthetic/synthetic-sequence-{dest-partition}`
- DN to BVN: `acc://dn.acme/synthetic/synthetic-sequence-{dest-partition}`

**Implementation**:

```go
func ConstructSequenceChainURL(srcPartitionID, dstPartitionID string) *url.URL {
    // Special case for Directory Network as source
    if strings.EqualFold(srcPartitionID, "dn") {
        normalizedDst := NormalizePartitionID(dstPartitionID)
        return protocol.DnUrl().JoinPath(protocol.Synthetic, "synthetic-sequence-"+normalizedDst)
    }
    
    // For all other partitions, normalize both source and destination
    normalizedSrc := NormalizePartitionID(srcPartitionID)
    normalizedDst := NormalizePartitionID(dstPartitionID)
    return protocol.PartitionUrl(normalizedSrc).JoinPath(protocol.Synthetic, "synthetic-sequence-"+normalizedDst)
}
```

### Anchor URLs

Anchor URLs identify anchor pools in a partition.

```go
// Use protocol.PartitionUrl for consistent URL construction
url := protocol.PartitionUrl(partitionID).JoinPath(protocol.AnchorPool)
```

**Format**:
- BVN: `acc://bvn-{partition-name}.acme/anchors`
- DN: `acc://dn.acme/anchors`

### Anchor Sequence URLs

Anchor sequence URLs identify anchor sequences between partitions.

```go
// Use wrapper function for consistent URL construction
url := ConstructAnchorSequenceURL(srcPartitionID, dstPartitionID)
```

**Format**:
- BVN to BVN: `acc://bvn-{source-partition}.acme/anchors/{dest-partition}`
- DN to BVN: `acc://dn.acme/anchors/{dest-partition}`

**Implementation**:

```go
func ConstructAnchorSequenceURL(srcPartitionID, dstPartitionID string) *url.URL {
    // Special case for Directory Network as source
    if strings.EqualFold(srcPartitionID, "dn") {
        normalizedDst := NormalizePartitionID(dstPartitionID)
        return protocol.DnUrl().JoinPath(protocol.AnchorPool, normalizedDst)
    }
    
    // For all other partitions, normalize both source and destination
    normalizedSrc := NormalizePartitionID(srcPartitionID)
    normalizedDst := NormalizePartitionID(dstPartitionID)
    return protocol.PartitionUrl(normalizedSrc).JoinPath(protocol.AnchorPool, normalizedDst)
}
```

## Partition ID Normalization

Partition IDs should be normalized to avoid double prefixes and ensure consistent URL construction.

```go
func NormalizePartitionID(id string) string {
    if strings.HasPrefix(strings.ToLower(id), "bvn-") {
        return id[4:]
    }
    if strings.HasPrefix(id, "dn-") {
        return id[3:]
    }
    return id
}
```

## URL Validation

URLs should be validated to ensure they meet the expected format. The following validation function can be used:

```go
func validateSynthURL(u *url.URL) error {
    // Validate authority format
    authority := u.Authority
    if !strings.HasSuffix(authority, protocol.TLD) {
        return fmt.Errorf("URL authority must end with .acme: %s", u)
    }

    // Extract the partition name from the authority
    var partitionName string
    if strings.HasPrefix(authority, "bvn-") {
        // For BVN URLs: acc://bvn-{partition}.acme
        partitionName = strings.TrimSuffix(strings.TrimPrefix(authority, "bvn-"), protocol.TLD)
    } else if authority == "dn"+protocol.TLD {
        // For DN URLs: acc://dn.acme
        partitionName = "dn"
    } else {
        return fmt.Errorf("URL authority must start with bvn- or be dn.acme: %s", u)
    }

    // Validate the partition name
    if !IsValidPartition(partitionName) {
        return fmt.Errorf("Invalid partition name in URL: %s", partitionName)
    }

    // Check for double bvn- prefix
    if strings.Contains(authority, "bvn-bvn-") {
        return fmt.Errorf("URL authority contains double bvn- prefix: %s", u)
    }

    // Validate path
    path := u.Path
    if path == "" {
        return fmt.Errorf("URL path cannot be empty: %s", u)
    }

    // Must start with /synthetic
    if !strings.HasPrefix(path, "/synthetic") {
        return fmt.Errorf("URL path must start with /synthetic: %s", u)
    }

    // Check for double /synthetic
    if strings.Contains(path, "/synthetic/synthetic") && !strings.Contains(path, "/synthetic/synthetic-sequence-") {
        return fmt.Errorf("URL path contains double /synthetic: %s", u)
    }

    // If path has more than just /synthetic, it must be a sequence chain
    if path != "/synthetic" && !strings.Contains(path, "/synthetic/synthetic-sequence-") {
        return fmt.Errorf("Invalid path format for sequence chain URL: %s", u)
    }

    // If it's a sequence chain, validate the destination partition name
    if strings.Contains(path, "/synthetic/synthetic-sequence-") {
        parts := strings.Split(path, "-")
        if len(parts) > 0 {
            destPartition := parts[len(parts)-1]
            if !IsValidPartition(destPartition) {
                return fmt.Errorf("Invalid destination partition name in sequence chain URL: %s", destPartition)
            }
        }
    }

    return nil
}
```

## Common Pitfalls

1. **Double Prefixes**: Adding "bvn-" to a partition ID that already has the prefix
   - Incorrect: `protocol.PartitionUrl("bvn-Apollo")` → `acc://bvn-bvn-Apollo.acme`
   - Correct: `protocol.PartitionUrl(NormalizePartitionID("bvn-Apollo"))` → `acc://bvn-Apollo.acme`

2. **Incorrect DN Handling**: Treating DN as a BVN
   - Incorrect: `protocol.PartitionUrl("bvn-dn")` → `acc://bvn-dn.acme`
   - Correct: `protocol.PartitionUrl("dn")` → `acc://dn.acme`

3. **Double Path Components**: Adding duplicate path components
   - Incorrect: `url.JoinPath(protocol.Synthetic, protocol.Synthetic)`
   - Correct: `url.JoinPath(protocol.Synthetic)`

4. **Missing Normalization**: Not normalizing partition IDs before URL construction
   - Incorrect: `protocol.PartitionUrl(partitionID)`
   - Correct: `protocol.PartitionUrl(NormalizePartitionID(partitionID))`

## Implementation Notes

1. Always use the wrapper functions from the `internal/urlutils` package for consistent URL construction.
2. Always normalize partition IDs before passing to `protocol.PartitionUrl`.
3. Handle the Directory Network (DN) as a special case.
4. Validate URLs to ensure they meet the expected format.

## Standardized URL Utilities Package

To address the URL construction and validation issues, a standardized URL utilities package has been created at `internal/urlutils`. This package provides a consistent API for URL operations across the codebase.

### Package Location

```
/tools/cmd/debug/internal/urlutils/
```

### Key Components

- **URL Construction Functions**:
  - `ConstructSyntheticLedgerURL(partitionID string) *url.URL` - Creates a synthetic ledger URL
  - `ConstructSequenceChainURL(srcPartitionID, dstPartitionID string) *url.URL` - Creates a sequence chain URL
  - `ConstructAnchorSequenceURL(srcPartitionID, dstPartitionID string) *url.URL` - Creates an anchor sequence URL

- **URL Validation Functions**:
  - `ValidateSynthURL(u *url.URL) error` - Validates a synthetic URL
  - `ValidateSynthURLOrPanic(u *url.URL)` - Validates a URL or panics if invalid

- **Partition Management Functions**:
  - `RegisterPartition(partitionID string)` - Registers a valid partition ID
  - `IsValidPartition(partitionID string) bool` - Checks if a partition ID is valid
  - `NormalizePartitionID(partitionID string) string` - Normalizes a partition ID

### Usage Example

```go
import (
    "fmt"
    "gitlab.com/accumulatenetwork/accumulate/tools/cmd/debug/internal/urlutils"
)

func ExampleURLUtilities() {
    // Register valid partitions (only needs to be done once in your application)
    urlutils.RegisterPartition("apollo") // Note: Will be normalized to lowercase internally
    urlutils.RegisterPartition("artemis")
    urlutils.RegisterPartition("dn")
    
    // Construct a synthetic ledger URL for a BVN
    bvnLedgerURL := urlutils.ConstructSyntheticLedgerURL("apollo")
    fmt.Println("BVN Ledger URL:", bvnLedgerURL)
    // Output: BVN Ledger URL: acc://bvn-apollo.acme/synthetic
    
    // Construct a synthetic ledger URL for the Directory Network
    dnLedgerURL := urlutils.ConstructSyntheticLedgerURL("dn")
    fmt.Println("DN Ledger URL:", dnLedgerURL)
    // Output: DN Ledger URL: acc://dn.acme/synthetic
    
    // Construct a sequence chain URL from one BVN to another
    bvnToBvnURL := urlutils.ConstructSequenceChainURL("apollo", "artemis")
    fmt.Println("BVN to BVN Sequence URL:", bvnToBvnURL)
    // Output: BVN to BVN Sequence URL: acc://bvn-apollo.acme/synthetic/synthetic-sequence-artemis
    
    // Construct a sequence chain URL from DN to a BVN
    dnToBvnURL := urlutils.ConstructSequenceChainURL("dn", "apollo")
    fmt.Println("DN to BVN Sequence URL:", dnToBvnURL)
    // Output: DN to BVN Sequence URL: acc://dn.acme/synthetic/synthetic-sequence-apollo
    
    // Validate a URL
    err := urlutils.ValidateSynthURL(bvnLedgerURL)
    if err != nil {
        fmt.Println("Validation error:", err)
    } else {
        fmt.Println("URL is valid")
    }
    // Output: URL is valid
    
    // Validate or panic (for use in tests or initialization code)
    urlutils.ValidateSynthURLOrPanic(bvnLedgerURL) // Will panic if URL is invalid
}
```

For more information, see the [URL Utilities README](../../internal/urlutils/README.md).

## See Also

- [Healing Process Overview](./healing-process.md)
- [Caching Strategies](./caching-strategies.md)
- [Error Handling](./error-handling.md)
- [Original URL Construction (v1)](../implementations/v1/chains-urls.md)
- [URL Diagnostics (v2)](../implementations/v2/url-diagnostics.md)
- [URL Standardization Plan (v3)](../implementations/v3/development-plan.md)
- [URL Utilities Package README](../../internal/urlutils/README.md)
