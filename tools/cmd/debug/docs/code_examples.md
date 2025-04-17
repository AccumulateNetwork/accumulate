# Accumulate Healing Code Examples

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS document (that follows) will not be deleted.

This document contains code examples that demonstrate key concepts in the Accumulate healing process. These examples are provided for reference and are not meant to be compiled directly. They illustrate important patterns and techniques used in the healing implementation.

## Table of Contents

1. [URL Normalization](#url-normalization)
2. [Enhanced Submission Process](#enhanced-submission-process)
3. [Routing Implementation](#routing-implementation)

## URL Normalization {#url-normalization}

URL normalization is critical for ensuring consistent routing in the Accumulate network. Different parts of the codebase may construct URLs in different formats, which can lead to routing conflicts and "element does not exist" errors.

### Key Concepts

- Standardizing on one URL format (the format from sequence.go)
- Converting between different URL formats
- Applying normalization to all URLs in messages

### Implementation Example

```go
// normalizeUrl converts between different URL formats to ensure consistent routing.
// This function standardizes URLs to use the format from sequence.go (e.g., acc://bvn-Apollo.acme)
// rather than the anchor pool URL format (e.g., acc://dn.acme/anchors/Apollo).
func normalizeUrl(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}

	// If this is an anchor pool URL with a partition path, convert to partition URL
	if strings.EqualFold(u.Authority, protocol.Directory) && strings.HasPrefix(u.Path, "/anchors/") {
		parts := strings.Split(u.Path, "/")
		if len(parts) >= 3 {
			partitionID := parts[2]
			return protocol.PartitionUrl(partitionID)
		}
	}

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
func normalizeAnchorUrl(srcId, dstId string) (*url.URL, *url.URL) {
	// Standardize on the partition URL format used in sequence.go
	srcUrl := protocol.PartitionUrl(srcId)
	dstUrl := protocol.PartitionUrl(dstId)

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
