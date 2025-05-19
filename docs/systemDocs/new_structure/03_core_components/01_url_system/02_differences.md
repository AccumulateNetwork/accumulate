# URL Construction Differences

```yaml
# AI-METADATA
document_type: concept
project: accumulate_network
component: debug_tools
version: current
```

## Overview

Different components of the Accumulate debug tools require different URL formats. This document explains these differences and provides guidance on how to maintain compatibility with existing code.

## URL Format Differences

### 1. Sequence Operations (in `sequence.go`)

Sequence operations use raw partition URLs for tracking:

```
acc://bvn-Apollo.acme
```

This format is used for:
- Tracking sequence chains
- Querying partition information
- Managing sequence operations

### 2. Anchor Healing (in `heal_anchor.go`)

Anchor healing operations append the partition ID to the anchor pool URL:

```
acc://dn.acme/anchors/Apollo
```

This format is used for:
- Healing anchor relationships
- Querying anchor pools
- Managing anchor operations

## Impact of Differences

These URL format differences can cause issues if not properly handled:

1. **Mismatched Queries**: Code might be looking for anchors at different URL paths
2. **Element Not Found Errors**: Queries might return "element does not exist" errors when checking the wrong URL format
3. **Broken Anchor Relationships**: Anchor relationships might not be properly maintained between partitions

## Resolution Approach

**IMPORTANT**: Do NOT attempt to standardize URL formats - this will break compatibility with the network.

Instead:

1. **Respect Existing Formats**: Each component must continue using the URL format it was designed for
2. **Use Appropriate Construction Methods**: Use the correct URL construction method for each component
3. **Document Format Requirements**: Clearly document which URL format is required by each component
4. **Test Against the Network**: Any changes to URL handling must be carefully tested against the actual network

## Implementation Examples

### Sequence URL Construction

```go
// Used by sequence operations
func ConstructSequenceChainURL(partitionID string) *url.URL {
    return url.MustParse(fmt.Sprintf("acc://bvn-%s.acme", partitionID))
}
```

### Anchor Pool URL Construction

```go
// Used by anchor healing
func ConstructAnchorPoolURL(baseURL, partitionID string) *url.URL {
    return url.MustParse(fmt.Sprintf("%s/anchors/%s", baseURL, partitionID))
}
```

## Best Practices

1. **Identify URL Format Requirements**: Before working with a component, identify which URL format it requires
2. **Use Consistent Construction Methods**: Use the same URL construction method throughout a component
3. **Add Defensive Checks**: Validate URL formats before making API calls
4. **Document URL Format Requirements**: Clearly document which URL format is required by each function
5. **Test Against the Network**: Always test URL handling against the actual network

## Related Documentation

- [URL Construction and Validation](./url-construction.md): Authoritative guide to URL formats, construction, and validation
- [URL Access Methods](../url_access_methods.md): Guide to different methods for accessing URLs
- [Network Initialization Design](./network-initialization/design.md): Comprehensive design document for network initialization
