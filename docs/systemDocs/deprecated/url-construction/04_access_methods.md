---
title: "Accumulate URL Access Methods"
description: "Guide to accessing different URL types in the Accumulate network"
date: "2025-04-25"
tags: ["url", "api", "synthetic-ledger", "sequence-chain", "access-methods"]
categories: ["development", "api", "documentation"]
ai_index: true
ai_keywords: ["url access methods", "synthetic ledger", "sequence chain", "Main() method", "Query method", "ChainQuery", "heal synth command"]
---

# Accumulate URL Access Methods

## Overview {#overview}

This document explains the different methods required to access different types of URLs in the Accumulate network. Understanding these differences is critical for properly implementing functions that interact with synthetic ledgers and sequence chains.

## URL Types and Access Methods {#url-types}

The Accumulate network has different types of URLs that require different access methods:

### 1. Synthetic Ledger URLs {#synthetic-ledger-urls}

**Format Example:**
```
acc://bvn-Yutu.acme/synthetic
acc://bvn-Chandrayaan.acme/synthetic
acc://dn.acme/synthetic
```

**Correct Access Method:**
```go
batch.Account(url).Main().GetAs(&ledger)
```

**Result:** Successfully returns the synthetic ledger

### 2. Sequence Chain URLs {#sequence-chain-urls}

**Format Example:**
```
acc://bvn-Yutu.acme/synthetic/synthetic-sequence-Chandrayaan
acc://bvn-Yutu.acme/synthetic/synthetic-sequence-Directory
acc://dn.acme/synthetic/synthetic-sequence-Yutu
```

**Incorrect Access Method (Will Fail):**
```go
batch.Account(url).Main().GetAs(&ledger)
```

**Error Message:**
```
Account.acc://bvn-Yutu.acme/synthetic/synthetic-sequence-Chandrayaan.Main not found
```

**Correct Access Method:**
```go
client.Query(ctx, url, &api.ChainQuery{Name: "main"})
```

**Result:** Successfully returns a chain record

## Root Cause Analysis {#root-cause}

The root cause of many issues with the `heal synth` command is that it's using the wrong access method for sequence chain URLs. The `pullSynthLedger` function is using the `.Main()` method for all URLs, but this only works for synthetic ledger URLs, not for sequence chain URLs.

Sequence chain URLs must be accessed using the `Query` method with a `ChainQuery`, not the `.Main()` method. This explains the errors we've been seeing in the logs.

## Implementation Guidance {#implementation}

When implementing functions that need to access both synthetic ledger URLs and sequence chain URLs, you should:

1. Determine the URL type based on its format
2. Use the appropriate access method for each URL type

### Example Implementation {#example}

```go
func accessURL(client *jsonrpc.Client, urlStr string) (interface{}, error) {
    // Parse URL
    u, err := url.Parse(urlStr)
    if err != nil {
        return nil, fmt.Errorf("failed to parse URL %s: %v", urlStr, err)
    }
    
    // Create timeout context
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // Determine URL type and use appropriate access method
    if strings.Contains(urlStr, "/synthetic/synthetic-sequence-") {
        // This is a sequence chain URL - use Query with ChainQuery
        return client.Query(ctx, u, &api.ChainQuery{Name: "main"})
    } else if strings.HasSuffix(urlStr, "/synthetic") {
        // This is a synthetic ledger URL - use Main()
        batch := light.OpenDB(false)
        defer batch.Discard()
        
        var ledger *protocol.SyntheticLedger
        err = batch.Account(u).Main().GetAs(&ledger)
        return ledger, err
    } else {
        return nil, fmt.Errorf("unknown URL type: %s", urlStr)
    }
}
```

## Testing {#testing}

Always test your implementation with both types of URLs to ensure they are being accessed correctly. The tests should verify that:

1. Synthetic ledger URLs are successfully accessed using the `.Main()` method
2. Sequence chain URLs are successfully accessed using the `Query` method with a `ChainQuery`

## Common Pitfalls {#pitfalls}

1. **Using `.Main()` for all URLs:** This will fail for sequence chain URLs with the error "Account.{url}.Main not found"
2. **Using `Query` with a `ChainQuery` for all URLs:** This might work for some URLs but is not the standard way to access synthetic ledger URLs
3. **Not handling timeout:** Always use a timeout context to prevent hanging when accessing URLs

## Conclusion {#conclusion}

Understanding the different access methods required for different URL types is critical for properly implementing functions that interact with synthetic ledgers and sequence chains. By using the correct access method for each URL type, you can avoid common errors and ensure your code works correctly.
