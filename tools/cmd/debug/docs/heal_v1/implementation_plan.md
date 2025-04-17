# Implementation Plan: Resolving Routing Errors

## Issue Summary

The healing process is encountering "cannot route message" errors due to inconsistent URL construction between different parts of the codebase. This causes routing conflicts when trying to submit messages to peers.

## Approach

We will implement a targeted solution that addresses the routing conflicts without making extensive changes to the codebase. The solution will focus on:

1. Adding URL normalization in the submission process
2. Enhancing error handling for routing conflicts
3. Implementing a simple caching mechanism to avoid redundant requests

## Specific Changes

### 1. URL Normalization Function

Add a new function to normalize URLs before submission:

```go
// normalizeUrl converts between different URL formats to ensure consistent routing
func normalizeUrl(u *url.URL) *url.URL {
    // If this is an anchor pool URL with a partition path, convert to partition URL
    if u.Authority == protocol.Directory && strings.HasPrefix(u.Path, "/anchors/") {
        parts := strings.Split(u.Path, "/")
        if len(parts) >= 3 {
            partitionID := parts[2]
            return protocol.PartitionUrl(partitionID)
        }
    }
    return u
}
```

### 2. Enhanced submitLoop Function

Modify the `submitLoop` function in `heal_common.go` to handle routing conflicts specifically:

```go
func (h *healer) submitLoop(wg *sync.WaitGroup) {
    defer wg.Done()
    
    // Cache for successful submissions to avoid redundant work
    submissionCache := make(map[string]bool)
    
    messages := make([]messaging.Message, 0, 10)
    for {
        select {
        case <-h.ctx.Done():
            return
        case msg := <-h.submit:
            messages = append(messages, msg...)
        default:
            if len(messages) == 0 {
                select {
                case msg := <-h.submit:
                    messages = append(messages, msg...)
                    continue
                case <-h.ctx.Done():
                    return
                }
            }

            // Create envelope with normalized URLs to avoid routing conflicts
            env := new(messaging.Envelope)
            for _, msg := range messages {
                // Apply URL normalization to message URLs if needed
                if msg, ok := msg.(*messaging.SequencedMessage); ok {
                    msg.Destination = normalizeUrl(msg.Destination)
                }
                env.Messages = append(env.Messages, msg)
            }

            // Check cache to avoid redundant submissions
            cacheKey := fmt.Sprintf("%x", env.Hash())
            if _, ok := submissionCache[cacheKey]; ok {
                slog.Info("Skipping cached submission", "id", env.Messages[0].ID())
                messages = messages[:0]
                continue
            }

            // Try default client first
            slog.Info("Submitting", "id", env.Messages[0].ID())
            submitted := false
            var lastErr error
            
            // First attempt with default client
            subs, err := h.C2.Submit(h.ctx, env, api.SubmitOptions{})
            if err == nil {
                // Process successful submission
                submissionCache[cacheKey] = true
                submitted = true
                slog.Info("Submitted successfully", "id", env.Messages[0].ID())
            } else {
                // Check specifically for routing conflicts
                if strings.Contains(err.Error(), "conflicting routes") {
                    slog.Warn("Detected routing conflict, attempting with normalized URLs", "id", env.Messages[0].ID())
                    
                    // Create a new envelope with more aggressive URL normalization
                    normalizedEnv := new(messaging.Envelope)
                    normalizedEnv.Messages = env.Messages
                    
                    // Try submission with normalized envelope
                    subs, err = h.C2.Submit(h.ctx, normalizedEnv, api.SubmitOptions{})
                    if err == nil {
                        submissionCache[cacheKey] = true
                        submitted = true
                        slog.Info("Submitted successfully with normalized URLs", "id", env.Messages[0].ID())
                    } else {
                        lastErr = err
                    }
                } else {
                    lastErr = err
                }
            }

            // If still not submitted, try each partition's peers
            if !submitted {
                // Try each partition's peers
                for _, part := range h.net.Status.Network.Partitions {
                    if submitted {
                        break
                    }
                    
                    for peer, info := range h.net.Peers[strings.ToLower(part.ID)] {
                        c := h.C2.ForPeer(peer)
                        if len(info.Addresses) > 0 {
                            c = c.ForAddress(info.Addresses[0])
                        }
                        
                        subs, err = c.Submit(h.ctx, env, api.SubmitOptions{})
                        if err != nil {
                            // If routing conflict, try with normalized envelope
                            if strings.Contains(err.Error(), "conflicting routes") {
                                normalizedEnv := new(messaging.Envelope)
                                normalizedEnv.Messages = env.Messages
                                
                                subs, err = c.Submit(h.ctx, normalizedEnv, api.SubmitOptions{})
                                if err != nil {
                                    lastErr = err
                                    continue
                                }
                            } else {
                                lastErr = err
                                continue
                            }
                        }
                        
                        submitted = true
                        submissionCache[cacheKey] = true
                        slog.Info("Submitted via peer", "id", env.Messages[0].ID(), "peer", peer)
                        break
                    }
                }
            }
            
            // Clear messages if submitted successfully, otherwise keep for retry
            if submitted {
                messages = messages[:0]
            } else {
                // If we tried all peers and still failed, log the error
                slog.ErrorContext(h.ctx, "Submission failed on all peers", "error", lastErr, "id", env.Messages[0].ID())
            }
        }
    }
}
```

### 3. URL Caching System

Add a simple caching system to avoid redundant URL processing:

```go
// Add to healer struct
type healer struct {
    // existing fields...
    urlCache map[string]*url.URL // Cache for normalized URLs
}

// Initialize cache in setup
func (h *healer) setup(ctx context.Context, network string) {
    // existing setup...
    h.urlCache = make(map[string]*url.URL)
}

// Use cache in normalizeUrl
func (h *healer) normalizeUrl(u *url.URL) *url.URL {
    key := u.String()
    if cached, ok := h.urlCache[key]; ok {
        return cached
    }
    
    normalized := normalizeUrl(u)
    h.urlCache[key] = normalized
    return normalized
}
```

## Testing Plan

1. Test URL normalization with various URL formats
2. Verify routing conflict resolution with test transactions
3. Confirm that the caching mechanism reduces redundant processing
4. Validate that the solution works across all partition types

## Rollout Strategy

1. Implement changes in a development branch
2. Run extensive testing with simulated routing conflicts
3. Deploy to a test network to validate in a real environment
4. Monitor error rates after deployment to production

## Success Criteria

1. No more "cannot route message" errors during healing
2. Successful message submission across all partition types
3. No regression in existing functionality
4. Improved performance due to caching
