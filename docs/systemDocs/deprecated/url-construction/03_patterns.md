# URL Construction Pattern

```yaml
# AI-METADATA
document_type: pattern
project: accumulate_network
component: debug_tools
version: current
ai_specific: true
```

## Overview

This document describes the URL construction patterns used in the Accumulate debug tools. Different components require different URL formats, and it's critical to use the correct format for each component to maintain compatibility with the network.

## Pattern Description

### Sequence Operations URL Pattern

Sequence operations use raw partition URLs for tracking:

```go
// Format: acc://bvn-{partitionID}.acme
// Example: acc://bvn-Apollo.acme
func ConstructSequenceChainURL(partitionID string) *url.URL {
    return url.MustParse(fmt.Sprintf("acc://bvn-%s.acme", partitionID))
}
```

This format is used in:
- `sequence.go`
- Functions that track sequence chains
- Functions that query partition information

### Anchor Healing URL Pattern

Anchor healing operations append the partition ID to the anchor pool URL:

```go
// Format: {baseURL}/anchors/{partitionID}
// Example: acc://dn.acme/anchors/Apollo
func ConstructAnchorPoolURL(baseURL, partitionID string) *url.URL {
    return url.MustParse(fmt.Sprintf("%s/anchors/%s", baseURL, partitionID))
}
```

This format is used in:
- `heal_anchor.go`
- Functions that heal anchor relationships
- Functions that query anchor pools

### Synthetic Ledger URL Pattern

Synthetic ledger operations use a specific URL format:

```go
// Format: acc://{partitionID}.acme/synthetic
// Example: acc://bvn-Apollo.acme/synthetic
func ConstructSyntheticLedgerURL(partitionID string) *url.URL {
    return url.MustParse(fmt.Sprintf("acc://%s.acme/synthetic", partitionID))
}
```

This format is used in:
- `heal_synth.go`
- Functions that access synthetic ledgers

### Sequence Chain URL Pattern

Sequence chain operations use a specific URL format:

```go
// Format: acc://{partitionID}.acme/synthetic/synthetic-sequence-{targetPartitionID}
// Example: acc://bvn-Apollo.acme/synthetic/synthetic-sequence-Directory
func ConstructSequenceChainURL(partitionID, targetPartitionID string) *url.URL {
    return url.MustParse(fmt.Sprintf("acc://%s.acme/synthetic/synthetic-sequence-%s", partitionID, targetPartitionID))
}
```

This format is used in:
- `heal_synth.go`
- Functions that access sequence chains

## Implementation Guidelines

1. **Identify URL Format Requirements**: Before working with a component, identify which URL format it requires
2. **Use Helper Functions**: Create helper functions for URL construction to ensure consistency
3. **Document URL Format**: Clearly document which URL format is required by each function
4. **Validate URLs**: Validate URL formats before making API calls
5. **Respect Existing Formats**: Do NOT attempt to standardize URL formats - this will break compatibility with the network

## Example Implementation

```go
// Helper functions for URL construction
func ConstructSequenceURL(partitionID string) *url.URL {
    return url.MustParse(fmt.Sprintf("acc://bvn-%s.acme", partitionID))
}

func ConstructAnchorURL(baseURL, partitionID string) *url.URL {
    return url.MustParse(fmt.Sprintf("%s/anchors/%s", baseURL, partitionID))
}

// Function that uses the correct URL format based on the operation type
func ConstructAppropriateURL(partitionID, operationType string) *url.URL {
    switch operationType {
    case "sequence":
        return ConstructSequenceURL(partitionID)
    case "anchor":
        return ConstructAnchorURL("acc://dn.acme", partitionID)
    default:
        return nil
    }
}
```

## Common Pitfalls

1. **Using the Wrong URL Format**: Using the sequence URL format for anchor healing operations or vice versa
2. **Attempting to Standardize URL Formats**: Trying to use a single URL format for all operations
3. **Hardcoding URL Formats**: Hardcoding URL formats instead of using helper functions
4. **Not Validating URLs**: Not validating URL formats before making API calls

## Best Practices

1. **Use Helper Functions**: Create helper functions for URL construction to ensure consistency
2. **Document URL Format Requirements**: Clearly document which URL format is required by each function
3. **Validate URLs**: Validate URL formats before making API calls
4. **Respect Existing Formats**: Each component must continue using the URL format it was designed for
5. **Test Against the Network**: Any changes to URL handling must be carefully tested against the actual network

## Related Documentation

- [URL Construction and Validation](../concepts/url-construction.md)
- [URL Construction Differences](../concepts/url-construction-differences.md)
- [URL Access Methods](../url_access_methods.md)
