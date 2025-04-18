# ADI URL Addressing in Accumulate

```yaml
# AI-METADATA
document_type: concept_documentation
project: accumulate_network
component: url_addressing
version: current
authors:
  - accumulate_team
last_updated: 2023-11-15

# Core Concepts
key_concepts:
  - url_addressing:
      description: "URL-based addressing scheme for all Accumulate resources"
      importance: critical
      related_patterns: [hierarchical_addressing, human_readable_identifiers]
  - adi_urls:
      description: "URL format for Accumulate Digital Identities"
      importance: critical
      related_patterns: [identity_namespaces, domain_hierarchy]
  - account_urls:
      description: "URL format for accounts within an ADI"
      importance: high
      related_patterns: [resource_addressing, path_structure]
  - url_normalization:
      description: "Process of standardizing URL formats to ensure consistent routing"
      importance: high
      related_patterns: [routing_consistency, partition_addressing]

# Related Components
related_components:
  - adi:
      relationship: "Uses URLs as primary identifiers"
      importance: critical
  - routing:
      relationship: "Uses URLs to route transactions to correct partitions"
      importance: high
  - transaction:
      relationship: "Uses URLs to identify sources and destinations"
      importance: high

# Technical Details
language: go
dependencies:
  - gitlab.com/accumulatenetwork/accumulate/protocol
  - gitlab.com/accumulatenetwork/accumulate/pkg/url

# Implementation References
implementation_files:
  - /pkg/url/url.go:
      contains: "URL parsing and formatting"
  - /protocol/url.go:
      contains: "Protocol-specific URL handling"
```

## Overview

Accumulate uses a URL-based addressing scheme for all resources in the network. This document explains how URLs are structured, particularly in relation to Accumulate Digital Identities (ADIs), and how they are used for routing and addressing.

## URL Structure

<!-- @concept url_structure -->
<!-- @importance critical -->
<!-- @description Basic structure of Accumulate URLs -->

Accumulate URLs follow this general structure:

```
acc://<identity>/<path>?<query>#<fragment>
```

Where:
- `acc://` is the protocol identifier
- `<identity>` is the ADI or account identifier
- `<path>` is an optional path to a sub-resource
- `<query>` is an optional query string
- `<fragment>` is an optional fragment identifier

## ADI URL Formats

<!-- @concept adi_url_formats -->
<!-- @importance critical -->
<!-- @description Different formats of ADI URLs -->

### Root ADI URLs

Root ADI URLs follow this format:

```
acc://<adi-name>.<domain>
```

Examples:
- `acc://factom.acme`
- `acc://enterprise.acme`
- `acc://user.acme`

### Sub-ADI URLs

Sub-ADIs are represented as subdomains:

```
acc://<sub-adi>.<parent-adi>.<domain>
```

Examples:
- `acc://finance.enterprise.acme`
- `acc://hr.enterprise.acme`
- `acc://project1.department.enterprise.acme`

### ADI Resource URLs

Resources within an ADI are addressed using paths:

```
acc://<adi-name>.<domain>/<resource-type>/<resource-id>
```

Examples:
- `acc://factom.acme/book` (Key book)
- `acc://factom.acme/book/1` (Key page)
- `acc://factom.acme/tokens` (Token account)
- `acc://factom.acme/data` (Data account)

## URL Implementation

<!-- @struct URL -->
<!-- @description Go implementation of Accumulate URLs -->
<!-- @purpose Provides parsing and formatting for Accumulate URLs -->

```go
// @struct URL
// @description Go implementation of Accumulate URLs
// @purpose Provides parsing and formatting for Accumulate URLs
type URL struct {
    // @field Scheme
    // @description URL scheme, always "acc" for Accumulate
    // @constant "acc"
    Scheme string

    // @field Authority
    // @description The identity portion of the URL
    // @example "factom.acme"
    Authority string

    // @field Path
    // @description Optional path to a sub-resource
    // @example "/book/1"
    Path string

    // @field Query
    // @description Optional query parameters
    // @example "?amount=100"
    Query url.Values

    // @field Fragment
    // @description Optional fragment identifier
    // @example "#section1"
    Fragment string
}
```

## URL Parsing and Formatting

<!-- @process url_parsing -->
<!-- @importance high -->
<!-- @description How URLs are parsed and formatted -->

### Parsing URLs

```go
// @function ParseUrl
// @description Parse a string into an Accumulate URL
// @param urlStr string - The URL string to parse
// @return *URL - The parsed URL
// @return error - Error if parsing fails
// @example Input: "acc://factom.acme/book/1", Output: URL{Scheme: "acc", Authority: "factom.acme", Path: "/book/1"}
func ParseUrl(urlStr string) (*URL, error) {
    // Parse the URL using standard Go URL parsing
    stdUrl, err := url.Parse(urlStr)
    if err != nil {
        return nil, fmt.Errorf("invalid URL: %w", err)
    }
    
    // Validate the scheme
    if stdUrl.Scheme != "acc" {
        return nil, fmt.Errorf("invalid scheme: %s", stdUrl.Scheme)
    }
    
    // Create and return the Accumulate URL
    return &URL{
        Scheme:    stdUrl.Scheme,
        Authority: stdUrl.Host,
        Path:      stdUrl.Path,
        Query:     stdUrl.Query(),
        Fragment:  stdUrl.Fragment,
    }, nil
}
```

### Formatting URLs

```go
// @function FormatUrl
// @description Format an Accumulate URL as a string
// @param baseUrl *URL - The base URL
// @param pathElements ...string - Path elements to append
// @return *URL - The formatted URL
// @example Input: URL{Authority: "factom.acme"}, "book", "1", Output: "acc://factom.acme/book/1"
func FormatUrl(baseUrl *URL, pathElements ...string) *URL {
    // Create a copy of the base URL
    newUrl := &URL{
        Scheme:    baseUrl.Scheme,
        Authority: baseUrl.Authority,
        Path:      baseUrl.Path,
        Query:     baseUrl.Query,
        Fragment:  baseUrl.Fragment,
    }
    
    // Append path elements
    for _, element := range pathElements {
        if newUrl.Path == "" || newUrl.Path == "/" {
            newUrl.Path = "/" + element
        } else {
            newUrl.Path = newUrl.Path + "/" + element
        }
    }
    
    return newUrl
}
```

## URL Normalization

<!-- @concept url_normalization -->
<!-- @importance high -->
<!-- @description Process of standardizing URL formats -->

URL normalization is critical for ensuring consistent routing in the Accumulate network. Different parts of the codebase may construct URLs in different formats, which can lead to routing conflicts.

### Partition URL Normalization

A key normalization case involves converting between different URL formats for partition references:

```go
// @function normalizeUrl
// @description Converts between different URL formats to ensure consistent routing
// @param u *url.URL - The URL to normalize
// @return *url.URL - The normalized URL using the format from sequence.go
// @example Input: acc://dn.acme/anchors/Apollo, Output: acc://bvn-Apollo.acme
// @related_concept url_standardization
// @importance critical - Inconsistent URL formats cause routing conflicts
func normalizeUrl(u *url.URL) *url.URL {
    // Check if this is an anchor pool URL with a partition path
    if strings.EqualFold(u.Authority, protocol.Directory) && strings.HasPrefix(u.Path, "/anchors/") {
        parts := strings.Split(u.Path, "/")
        if len(parts) >= 3 {
            // Extract the partition ID from the path
            partitionID := parts[2]
            // Create a partition URL using the extracted ID
            return protocol.PartitionUrl(partitionID)
        }
    }

    // Return the original URL if it's not an anchor pool URL
    return u
}
```

### Anchor URL Normalization

Another important case is normalizing anchor URLs:

```go
// @function normalizeAnchorUrl
// @description Standardizes anchor URLs to use the partition URL format from sequence.go
// @param srcId string - Source partition ID (e.g., "Apollo")
// @param dstId string - Destination partition ID (e.g., "BVN1")
// @return *url.URL - Normalized source URL (e.g., acc://bvn-Apollo.acme)
// @return *url.URL - Normalized destination URL (e.g., acc://bvn-BVN1.acme)
// @example Input: "Apollo", "BVN1", Output: acc://bvn-Apollo.acme, acc://bvn-BVN1.acme
// @pattern url_standardization
// @pattern anchor_url_normalization
// @importance critical - Ensures consistent URL format for anchor operations
func normalizeAnchorUrl(srcId, dstId string) (*url.URL, *url.URL) {
    // Convert source partition ID to standardized partition URL
    srcUrl := protocol.PartitionUrl(srcId)
    
    // Convert destination partition ID to standardized partition URL
    dstUrl := protocol.PartitionUrl(dstId)

    // Return both normalized URLs
    return srcUrl, dstUrl
}
```

## URL Routing

<!-- @concept url_routing -->
<!-- @importance high -->
<!-- @description How URLs are used for routing transactions -->

URLs play a critical role in routing transactions to the correct partitions in the Accumulate network.

> **API Version Note**: URL routing has evolved across API versions. V1 had basic routing capabilities with limited partition awareness. V2 improved routing with better partition handling, while V3 provides comprehensive routing with optimized partition selection and stateless design. See [ADI API Version Comparison](adi_api_versions.md) for details.

When a URL is used in the Accumulate network, the following routing process occurs:

### Basic Routing Process

1. **URL Parsing**: The transaction's source and destination URLs are parsed
2. **URL Normalization**: URLs are normalized to ensure consistent routing
3. **Partition Determination**: The appropriate partition is determined based on the URL
4. **Transaction Routing**: The transaction is routed to the correct partition

### Routing Implementation

```go
// @struct NormalizingRouter
// @description Router that normalizes URLs before routing
// @purpose Ensures consistent routing despite URL format differences
type NormalizingRouter struct {
    // @field Router
    // @description The underlying router implementation
    // @embedded Used for actual routing after normalization
    Router protocol.Router
}

// @function RouteAccount
// @description Routes an account URL to the appropriate partition
// @param u *url.URL - The account URL to route
// @return string - The partition ID
// @return error - Error if routing fails
// @example Input: acc://factom.acme/book, Output: "BVN1"
func (r *NormalizingRouter) RouteAccount(u *url.URL) (string, error) {
    // Normalize the URL before routing
    normalized := normalizeUrl(u)
    return r.Router.RouteAccount(normalized)
}
```

## URL Types and Special Cases

<!-- @concept url_types -->
<!-- @importance medium -->
<!-- @description Different types of URLs in the Accumulate system -->

### Lite Account URLs

Lite accounts (which exist outside of ADIs) have a special URL format:

```
acc://ACME/<public-key-hash>
```

Example:
- `acc://ACME/4cd6e309a084c8a4d5fe43d221447c3f7163a678b9c3b25b1b8d8eadfd9348b1`

### Directory Network URLs

The Directory Network (DN) has special URL formats:

```
acc://dn.acme
```

With various system paths:
- `acc://dn.acme/anchors/<partition-id>`
- `acc://dn.acme/operators`
- `acc://dn.acme/network`

### Partition URLs

Partition URLs follow this format:

```
acc://bvn-<partition-id>.acme
```

Examples:
- `acc://bvn-Apollo.acme`
- `acc://bvn-BVN1.acme`

## URL Construction Best Practices

<!-- @concept url_construction_best_practices -->
<!-- @importance high -->
<!-- @description Best practices for constructing and working with URLs -->

1. **Use Standard Functions**: Always use the standard URL construction functions
   ```go
   // Create an ADI URL
   adiUrl := protocol.AccountUrl("factom.acme")
   
   // Create a resource URL
   bookUrl := protocol.FormatUrl(adiUrl, "book")
   ```

2. **Normalize Before Routing**: Always normalize URLs before routing
   ```go
   normalizedUrl := normalizeUrl(url)
   partition, err := router.RouteAccount(normalizedUrl)
   ```

3. **Consistent Format**: Standardize on one URL format throughout your code
   ```go
   // Prefer this format for partition URLs
   partitionUrl := protocol.PartitionUrl("Apollo")
   // Instead of this format
   // partitionUrl := protocol.FormatUrl(protocol.DnUrl(), "anchors", "Apollo")
   ```

4. **Validate URLs**: Always validate URLs before using them
   ```go
   if !protocol.IsValidAdiUrl(url) {
       return fmt.Errorf("invalid ADI URL: %v", url)
   }
   ```

## Common URL Operations

<!-- @concept common_url_operations -->
<!-- @importance medium -->
<!-- @description Common operations performed on URLs -->

### Getting the ADI Name

```go
// @function GetAdiName
// @description Extract the ADI name from an ADI URL
// @param u *url.URL - The ADI URL
// @return string - The ADI name
// @example Input: acc://factom.acme/book, Output: "factom"
func GetAdiName(u *url.URL) string {
    parts := strings.Split(u.Authority, ".")
    return parts[0]
}
```

### Checking URL Types

```go
// @function IsAdiUrl
// @description Check if a URL is an ADI URL
// @param u *url.URL - The URL to check
// @return bool - True if the URL is an ADI URL
// @example Input: acc://factom.acme, Output: true
func IsAdiUrl(u *url.URL) bool {
    return strings.Contains(u.Authority, ".") && !strings.HasPrefix(u.Authority, "bvn-")
}

// @function IsPartitionUrl
// @description Check if a URL is a partition URL
// @param u *url.URL - The URL to check
// @return bool - True if the URL is a partition URL
// @example Input: acc://bvn-Apollo.acme, Output: true
func IsPartitionUrl(u *url.URL) bool {
    return strings.HasPrefix(u.Authority, "bvn-")
}
```

### Creating Sub-ADI URLs

```go
// @function SubAdiUrl
// @description Create a sub-ADI URL
// @param parentUrl *url.URL - The parent ADI URL
// @param name string - The sub-ADI name
// @return *url.URL - The sub-ADI URL
// @example Input: acc://factom.acme, "finance", Output: acc://finance.factom.acme
func SubAdiUrl(parentUrl *url.URL, name string) *url.URL {
    return &url.URL{
        Scheme:    parentUrl.Scheme,
        Authority: name + "." + parentUrl.Authority,
    }
}
```

## URL-Related Issues and Solutions

<!-- @concept url_related_issues -->
<!-- @importance high -->
<!-- @description Common issues with URLs and their solutions -->

### URL Construction Differences

**Issue**: Different parts of the codebase construct URLs in different formats.

**Example**:
- Some code uses raw partition URLs: `acc://bvn-Apollo.acme`
- Other code uses anchor pool URLs: `acc://dn.acme/anchors/Apollo`

**Solution**: Standardize on one approach (typically the partition URL format) and use URL normalization to convert between formats.

### URL Parsing Failures

**Issue**: URL parsing can fail if the URL format is unexpected.

**Solution**: Implement robust parsing with fallbacks:

```go
func parseUrlWithFallback(urlStr string) (*url.URL, error) {
    // Try standard parsing first
    u, err := url.Parse(urlStr)
    if err == nil {
        return u, nil
    }
    
    // Try adding the scheme if missing
    if !strings.Contains(urlStr, "://") {
        u, err = url.Parse("acc://" + urlStr)
        if err == nil {
            return u, nil
        }
    }
    
    // Return the original error if all attempts fail
    return nil, err
}
```

### URL Routing Conflicts

**Issue**: Different URL formats for the same resource can cause routing conflicts.

**Solution**: Implement a normalizing router that standardizes URLs before routing:

```go
type NormalizingRouter struct {
    BaseRouter protocol.Router
}

func (r *NormalizingRouter) RouteAccount(u *url.URL) (string, error) {
    normalized := normalizeUrl(u)
    return r.BaseRouter.RouteAccount(normalized)
}
```

## Related Documentation

- [Accumulate Digital Identities (ADIs)](adi_documentation.md)
- [ADI API Version Comparison](adi_api_versions.md) - Detailed comparison of URL handling across API versions
- [Routing Implementation](routing_implementation.md)
- [Transaction Processing](transaction_processing.md)
- [Network Architecture](network_architecture.md)
