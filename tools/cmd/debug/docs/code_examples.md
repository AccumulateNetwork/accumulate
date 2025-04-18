# Accumulate Healing Code Examples

```yaml
# AI-METADATA
document_type: code_examples
project: accumulate_network
component: healing_implementation
version: current
authors:
  - accumulate_team
last_updated: 2023-11-15

# Core Concepts and Patterns
key_concepts:
  - url_normalization:
      description: "Standardizing URL formats across the codebase to prevent routing conflicts"
      importance: critical
      related_patterns: [url_standardization, partition_url_construction]
  - stateless_design:
      description: "New implementation is completely stateless with no persistence between runs"
      importance: high
      contrast: "Previous versions used a persistent database"
  - minimal_caching:
      description: "Very limited caching only for rejected transactions to avoid regeneration"
      importance: medium
      contrast: "Previous versions had extensive query caching"
  - no_retry_mechanism:
      description: "No automatic retry with backoff, requires manual restart of failed operations"
      importance: high
      contrast: "Previous versions had automatic retry with configurable backoff"
  - enhanced_submission_process:
      description: "Improved submission process with URL normalization and multi-peer attempts"
      importance: high
  - routing_implementation:
      description: "Custom routing implementation that handles URL normalization and direct routing"
      importance: high

# Code Examples Included
code_examples:
  - normalizeUrl:
      purpose: "Convert between different URL formats to ensure consistent routing"
      pattern: url_standardization
      importance: critical
  - normalizeUrlsInMessage:
      purpose: "Apply URL normalization to all URLs in a message"
      pattern: message_normalization
  - submitLoop:
      purpose: "Handle message submission with routing conflict resolution"
      pattern: enhanced_submission
  - NormalizingRouter:
      purpose: "Router implementation that normalizes URLs before routing"
      pattern: routing_implementation
  - DirectRouter:
      purpose: "Router implementation that uses direct routing for problematic URLs"
      pattern: routing_implementation
  - normalizeAnchorUrl:
      purpose: "Standardize anchor URLs to use the partition URL format"
      pattern: url_standardization
      importance: critical
  - ExtractHostFromMultiaddr:
      purpose: "Extract host from multiaddr string"
      pattern: host_extraction
      importance: high
  - ExtractHostFromURL:
      purpose: "Extract host from URL string"
      pattern: host_extraction
      importance: high
  - LookupValidatorHost:
      purpose: "Look up known host for validator ID"
      pattern: validator_lookup
      importance: high

# Technical Details
language: go
dependencies:
  - github.com/multiformats/go-multiaddr: "^0.8.0"
  - gitlab.com/accumulatenetwork/accumulate/protocol: "latest"
  - github.com/cometbft/cometbft/p2p: "^0.37.0"
  - github.com/spf13/cobra: "^1.6.0"

# Related Documentation
related_files:
  - dev_v3/DEVELOPMENT_PLAN.md:
      relationship: "Contains detailed development plan and architecture decisions"
  - dev_v4/peer_discovery_analysis.md:
      relationship: "Analysis of peer discovery mechanisms and host extraction"
  - index.md:
      relationship: "Main documentation index with overview of all components"

# Known Issues
known_issues:
  - url_construction_differences:
      description: "Different parts of the codebase construct URLs in different formats"
      severity: high
      resolution: "Standardize on sequence.go format (acc://bvn-Apollo.acme)"
  - multiaddr_parsing_failures:
      description: "Some multiaddr formats with p2p components fail to parse"
      severity: medium
      workaround: "Use fallback chain with URL parsing and validator mapping"
```

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS document (that follows) will not be deleted.

This document contains code examples that demonstrate key concepts in the Accumulate healing process. These examples are provided for reference and are not meant to be compiled directly. They illustrate important patterns and techniques used in the healing implementation.

## Table of Contents

1. [URL Normalization](#url-normalization)
2. [Enhanced Submission Process](#enhanced-submission-process)
3. [Routing Implementation](#routing-implementation)
4. [Stateless Design](#stateless-design)

## URL Normalization {#url-normalization}

> **Related Topics:**
> - [URL Construction in Development Plan](dev_v3/DEVELOPMENT_PLAN.md)
> - [Peer Discovery Analysis](dev_v4/peer_discovery_analysis.md)
> - [Network Status URL Construction](dev_v3/network_status.md#url-construction)
> - [Caching System](dev_v3/DEVELOPMENT_PLAN.md#caching-system-implementation)

URL normalization is critical for ensuring consistent routing in the Accumulate network. Different parts of the codebase may construct URLs in different formats, which can lead to routing conflicts and "element does not exist" errors.

**Important Issue**: There is a fundamental difference in how URLs are constructed between different parts of the codebase:
- Some code uses raw partition URLs (e.g., `acc://bvn-Apollo.acme`)
- Other code appends the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

This discrepancy can cause anchor healing to fail because the code might be looking for anchors at different URL paths, leading to "element does not exist" errors.

### Key Concepts

- Standardizing on one URL format (the format from sequence.go)
- Converting between different URL formats
- Applying normalization to all URLs in messages

### Implementation Example

```go
// @function normalizeUrl
// @description Converts between different URL formats to ensure consistent routing
// @param u *url.URL - The URL to normalize
// @return *url.URL - The normalized URL using the format from sequence.go
// @example Input: acc://dn.acme/anchors/Apollo, Output: acc://bvn-Apollo.acme
// @related_concept url_standardization
// @importance critical - Inconsistent URL formats cause routing conflicts
func normalizeUrl(u *url.URL) *url.URL {
	// @check Return nil for nil input to avoid nil pointer dereference
	if u == nil {
		return nil
	}

	// @check Check if this is an anchor pool URL with a partition path
	// @pattern Convert anchor pool URL to partition URL
	if strings.EqualFold(u.Authority, protocol.Directory) && strings.HasPrefix(u.Path, "/anchors/") {
		parts := strings.Split(u.Path, "/")
		if len(parts) >= 3 {
			// @extract Extract the partition ID from the path
			partitionID := parts[2]
			// @convert Create a partition URL using the extracted ID
			return protocol.PartitionUrl(partitionID)
		}
	}

	// @fallback Return the original URL if it's not an anchor pool URL
	return u
}

// normalizeUrlsInMessage applies URL normalization to all URLs in a message
// This helps prevent routing conflicts by ensuring consistent URL formats
func normalizeUrlsInMessage(msg interface{}) {
	switch m := msg.(type) {
	case interface{ GetDestination() *url.URL }:
		if dest := m.GetDestination(); dest != nil {
			// Use type assertion to access the unexported field
			// This is a bit hacky but necessary to modify the URL
			if setter, ok := m.(interface{ SetDestination(*url.URL) }); ok {
				setter.SetDestination(normalizeUrl(dest))
			}
		}
	case interface{ GetUrl() *url.URL }:
		if u := m.GetUrl(); u != nil {
			// Use type assertion to access the unexported field
			if setter, ok := m.(interface{ SetUrl(*url.URL) }); ok {
				setter.SetUrl(normalizeUrl(u))
			}
		}
	}
}
```

### Usage Notes

- Apply URL normalization before submitting messages to the network
- Use the same normalization approach consistently throughout your code
- Be aware of the different URL formats used in different parts of the codebase

## Enhanced Submission Process {#enhanced-submission-process}

The submission process needs to handle routing conflicts and other issues that may arise during message submission. This enhanced implementation includes:

### Key Concepts

- Normalizing URLs before submission
- Caching successful submissions to avoid redundant work
- Trying multiple peers when submission fails
- Handling routing conflicts with more aggressive normalization

### Implementation Example

```go
// submitLoop is a modified version of the original function that handles routing conflicts
// by normalizing URLs before submission. This prevents "cannot route message" errors
// caused by inconsistent URL construction.
func (h *healer) submitLoop(wg *sync.WaitGroup) {
	defer wg.Done()
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()

	// Cache for successful submissions to avoid redundant work
	submissionCache := make(map[string]bool)

	var messages []messaging.Message
	var stop bool
	for !stop {
		select {
		case <-h.ctx.Done():
			stop = true
		case msg := <-h.submit:
			messages = append(messages, msg...)
			if len(messages) < 50 {
				continue
			}
		case <-t.C:
		}
		if len(messages) == 0 {
			continue
		}

		// Create envelope with normalized URLs to avoid routing conflicts
		env := new(messaging.Envelope)
		for _, msg := range messages {
			// Normalize URLs in the message to prevent routing conflicts
			normalizeUrlsInMessage(msg)
			env.Messages = append(env.Messages, msg)
		}

		// Generate a cache key based on the envelope hash
		cacheKey := fmt.Sprintf("%x", env.Hash())
		if _, ok := submissionCache[cacheKey]; ok {
			slog.Info("Skipping cached submission", "id", env.Messages[0].ID())
			messages = messages[:0]
			continue
		}

		// Try to submit using multiple peers
		var submitted bool
		var lastErr error

		// First try with the default client (for backward compatibility)
		subs, err := h.C2.Submit(h.ctx, env, api.SubmitOptions{})
		if err == nil {
			// Process successful submissions
			for _, sub := range subs {
				if sub.Success {
					slog.InfoContext(h.ctx, "Submission succeeded", "id", sub.Status.TxID)
					submitted = true
				} else {
					slog.ErrorContext(h.ctx, "Submission failed", "message", sub, "status", sub.Status)
				}
			}
			// Cache successful submission
			if submitted {
				submissionCache[cacheKey] = true
			}
		} else {
			// Check specifically for routing conflicts
			if errors.Is(err, errors.BadRequest) && strings.Contains(err.Error(), "conflicting routes") {
				slog.WarnContext(h.ctx, "Detected routing conflict, attempting with more aggressive URL normalization", 
					"id", env.Messages[0].ID(), "error", err)
				
				// Create a new envelope with more aggressive URL normalization
				normalizedEnv := new(messaging.Envelope)
				for _, msg := range messages {
					// Apply more aggressive normalization for routing conflicts
					switch m := msg.(type) {
					case *messaging.SequencedMessage:
						// Ensure destination is normalized
						m.Destination = normalizeUrl(m.Destination)
					}
					normalizedEnv.Messages = append(normalizedEnv.Messages, msg)
				}
				
				// Try submission with normalized envelope
				subs, err = h.C2.Submit(h.ctx, normalizedEnv, api.SubmitOptions{})
				if err == nil {
					for _, sub := range subs {
						if sub.Success {
							slog.InfoContext(h.ctx, "Submission succeeded with normalized URLs", "id", sub.Status.TxID)
							submitted = true
						}
					}
					if submitted {
						submissionCache[cacheKey] = true
					}
				} else {
					lastErr = err
					slog.ErrorContext(h.ctx, "Normalized submission still failed", "error", err)
				}
			} else {
				lastErr = err
				slog.ErrorContext(h.ctx, "Default submission failed, trying individual peers", 
					"error", err, "id", env.Messages[0].ID())
			}

			// If still not submitted, try each partition's peers
			if !submitted {
				// Try peers from each partition (code simplified for brevity)
				// See full implementation for details on peer selection and retry logic
			}
		}
		
		// Clear messages if submitted successfully, otherwise keep for retry
		if submitted {
			messages = messages[:0]
		} else {
			// If we tried all peers and still failed, log the error
			slog.ErrorContext(h.ctx, "Submission failed on all peers", 
				"error", lastErr, "id", env.Messages[0].ID())
		}
	}
}
```

### Usage Notes

- The submission process should handle routing conflicts gracefully
- Caching successful submissions prevents redundant work
- Trying multiple peers increases the chance of successful submission
- Logging submission results helps with debugging

## Routing Implementation {#routing-implementation}

The routing implementation ensures that messages are routed to the correct partition. This example shows how to enhance the routing system with URL normalization and direct routing for problematic URLs.

### Key Concepts

- Wrapping the existing router with normalization functionality
- Creating direct routing overrides for problematic URLs
- Normalizing anchor URLs for consistent routing

### Implementation Example

```go
// NormalizingRouter wraps a Router and normalizes URLs before routing
type NormalizingRouter struct {
	Router routing.Router
}

// RouteAccount normalizes the URL before routing to ensure consistent routing
func (r *NormalizingRouter) RouteAccount(u *url.URL) (string, error) {
	normalized := normalizeUrl(u)
	return r.Router.RouteAccount(normalized)
}

// Route normalizes all URLs in the envelopes before routing
func (r *NormalizingRouter) Route(envs ...*messaging.Envelope) (string, error) {
	// Normalize URLs in all envelopes
	normalizedEnvs := make([]*messaging.Envelope, len(envs))
	for i, env := range envs {
		normalizedEnv := new(messaging.Envelope)
		normalizedEnv.Signatures = env.Signatures
		for _, msg := range env.Messages {
			normalizeUrlsInMessage(msg)
			normalizedEnv.Messages = append(normalizedEnv.Messages, msg)
		}
		normalizedEnvs[i] = normalizedEnv
	}

	return r.Router.Route(normalizedEnvs...)
}

// DirectRouter is a router that uses direct addressing for problematic URLs
type DirectRouter struct {
	BaseRouter routing.Router
	Overrides  map[string]string // URL string -> partition ID
}

// RouteAccount routes an account URL, using overrides for problematic URLs
func (r *DirectRouter) RouteAccount(u *url.URL) (string, error) {
	// Check if this URL has an override
	if partition, ok := r.Overrides[u.String()]; ok {
		return partition, nil
	}

	// Normalize the URL before routing
	normalized := normalizeUrl(u)

	// Check if the normalized URL has an override
	if partition, ok := r.Overrides[normalized.String()]; ok {
		return partition, nil
	}

	// Fall back to the base router
	return r.BaseRouter.RouteAccount(normalized)
}

// normalizeAnchorUrl ensures consistent URL format for anchors
// This function standardizes URLs to use the format from sequence.go (e.g., acc://bvn-Apollo.acme)
// rather than the anchor pool URL format (e.g., acc://dn.acme/anchors/Apollo).
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
// @related_concept url_construction_differences
// @see Development Plan: URL Construction Differences
func normalizeAnchorUrl(srcId, dstId string) (*url.URL, *url.URL) {
	// @step Convert source partition ID to standardized partition URL
	// @pattern partition_url_construction
	// @implementation Uses sequence.go format (acc://bvn-<partition>.acme)
	srcUrl := protocol.PartitionUrl(srcId)
	
	// @step Convert destination partition ID to standardized partition URL
	// @pattern partition_url_construction
	dstUrl := protocol.PartitionUrl(dstId)

	// @step Return both normalized URLs
	return srcUrl, dstUrl
}
```

### Usage Notes

- Use the NormalizingRouter to wrap the existing router
- Create direct routing overrides for URLs that are known to cause problems
- Always normalize anchor URLs before using them in the healing process

## Best Practices

1. **Consistent URL Construction**: Use the same URL construction method throughout your code. Standardize on the format from sequence.go (e.g., `acc://bvn-Apollo.acme`).

2. **Normalize Before Submission**: Always normalize URLs before submitting messages to the network.

3. **Handle Routing Conflicts**: Implement proper error handling for routing conflicts, including more aggressive normalization when needed.

4. **Cache Successful Submissions**: Use a cache to avoid redundant submissions of the same message.

5. **Try Multiple Peers**: When submission fails, try multiple peers to increase the chance of success.

6. **Log Submission Results**: Log detailed information about submission attempts and results for debugging.

7. **Use Direct Routing for Problematic URLs**: Maintain a list of problematic URLs and use direct routing for them.

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS document (above) will not be deleted.

## Stateless Design {#stateless-design}

> **Related Topics:**
> - [Development Plan](dev_v3/DEVELOPMENT_PLAN.md#caching-system-implementation)
> - [Peer Discovery](dev_v4/peer_discovery_analysis.md)

The new healing implementation (v3+) uses a **stateless design** with very limited caching, unlike previous versions. This is a fundamental architectural change that developers should be aware of.


#### Old Implementation (v1/v2)

```go
// @component HealingState
// @description Old implementation maintained state between runs (v1/v2)
// @deprecated This approach is no longer used in v3+
// @design_pattern stateful_with_caching
type HealingState struct {
    // @field DB
    // @description Persistent database to track healing progress across runs
    // @purpose Maintain state between executions
    DB *database.DB
    
    // @field QueryCache
    // @description Extensive caching of query results indexed by URL and query type
    // @purpose Avoid repeated queries for the same data, especially for account and chain queries
    // @key_structure URL + QueryType
    QueryCache map[string]interface{}
    
    // @field ProblemNodes
    // @description Tracking of problematic nodes to avoid querying them
    // @purpose Improve reliability by avoiding nodes that consistently fail
    // @key_structure NodeAddress -> []QueryTypes
    ProblemNodes map[string][]string
    
    // @field RetryQueue
    // @description Queue for automatic retries of failed operations
    // @purpose Implement automatic retry with backoff
    RetryQueue []*RetryItem
}

// @function submitWithRetry
// @description Example of retry logic in old implementation (v1/v2)
// @deprecated This approach is no longer used in v3+
// @param ctx context.Context - Context for the operation
// @param tx *protocol.Transaction - Transaction to submit
// @return error - Error if all retries fail
// @design_pattern automatic_retry_with_backoff
func (h *Healer) submitWithRetry(ctx context.Context, tx *protocol.Transaction) error {
    // @loop Retry loop with configurable max attempts
    for i := 0; i < h.maxRetries; i++ {
        // @attempt Try to submit the transaction
        err := h.submit(ctx, tx)
        if err == nil {
            // @success Return immediately on success
            return nil
        }
        
        // @backoff Exponential backoff between retry attempts
        // @pattern automatic_retry
        time.Sleep(h.retryBackoff * time.Duration(i))
    }
    // @failure Return error after exhausting all retry attempts
    return errors.New("max retries exceeded")
}

#### New Implementation (v3+)

```go
// @component Healer
// @description New implementation is stateless (v3+)
// @design_pattern stateless
type Healer struct {
    // @field rejectedTxs
    // @description Very limited caching only for rejected transactions
    // @purpose Avoid regenerating the same transaction
    // @key_structure TransactionHash -> bool
    // No persistent database
    // No extensive caching
    // No tracking of problematic nodes
    // No automatic retry mechanism
    
    // Very limited caching only for rejected transactions
    rejectedTxs map[string]bool
}

// Example of transaction submission in new implementation
func (h *Healer) submit(ctx context.Context, tx *protocol.Transaction) error {
    // Check if this exact transaction was previously rejected
    txHash := tx.GetHash().String()
    if h.rejectedTxs[txHash] {
        return errors.New("transaction previously rejected")
    }
    
    // Single attempt submission with no automatic retry
    err := h.client.Submit(ctx, tx)
    if err != nil {
        // Mark as rejected to avoid regenerating the same transaction
        h.rejectedTxs[txHash] = true
        return err
    }
    
    return nil
}
```

### Implementation Considerations

1. **No State Persistence:**
   - Each run of the healing utility is completely independent
   - No information is carried over between executions
   - Results must be manually tracked by the operator

2. **Manual Retry Process:**
   - Failed operations must be manually restarted
   - No automatic retry with backoff
   - Operators must monitor and manage the healing process

3. **Minimal Memory Usage:**
   - The stateless design uses significantly less memory
   - No large caches or databases are maintained
   - Only rejected transactions are tracked to avoid regeneration
