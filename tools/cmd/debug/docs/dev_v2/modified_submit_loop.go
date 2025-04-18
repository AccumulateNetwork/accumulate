// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

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
				for _, partition := range h.net.Status.Network.Partitions {
					// Skip if we've already submitted successfully
					if submitted {
						break
					}

					slog.InfoContext(h.ctx, "Trying peers from partition", "partition", partition.ID)

					// Make sure peers map is initialized for this partition
					if h.net == nil || h.net.Peers == nil || h.net.Peers[strings.ToLower(partition.ID)] == nil {
						slog.WarnContext(h.ctx, "No peers available for partition", "partition", partition.ID)
						continue
					}

					for peerID, info := range h.net.Peers[strings.ToLower(partition.ID)] {
						// Create a client for this peer
						c := h.C2.ForPeer(peerID)
						if len(info.Addresses) > 0 {
							c = c.ForAddress(info.Addresses[0])
						}

						slog.InfoContext(h.ctx, "Trying submission with peer", "peer", peerID, "partition", partition.ID)

						// Set a timeout for this specific submission attempt
						submitCtx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
						subs, err := c.Submit(submitCtx, env, api.SubmitOptions{})
						cancel()

						if err == nil {
							// Process successful submissions
							for _, sub := range subs {
								if sub.Success {
									slog.InfoContext(h.ctx, "Submission succeeded with peer",
										"peer", peerID, "id", sub.Status.TxID)
									submitted = true
								} else {
									slog.ErrorContext(h.ctx, "Submission failed with peer",
										"peer", peerID, "message", sub, "status", sub.Status)
								}
							}

							if submitted {
								submissionCache[cacheKey] = true
								break // Exit the peer loop on success
							}
						} else if errors.Is(err, errors.BadRequest) && strings.Contains(err.Error(), "conflicting routes") {
							// If routing conflict, try with normalized envelope
							slog.WarnContext(h.ctx, "Detected routing conflict with peer, trying normalized URLs",
								"peer", peerID, "error", err)

							// Create a new envelope with more aggressive URL normalization
							normalizedEnv := new(messaging.Envelope)
							for _, msg := range messages {
								// Apply more aggressive normalization
								switch m := msg.(type) {
								case *messaging.SequencedMessage:
									m.Destination = normalizeUrl(m.Destination)
								}
								normalizedEnv.Messages = append(normalizedEnv.Messages, msg)
							}

							submitCtx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
							subs, err = c.Submit(submitCtx, normalizedEnv, api.SubmitOptions{})
							cancel()

							if err == nil {
								for _, sub := range subs {
									if sub.Success {
										slog.InfoContext(h.ctx, "Submission succeeded with normalized URLs",
											"peer", peerID, "id", sub.Status.TxID)
										submitted = true
									}
								}
								if submitted {
									submissionCache[cacheKey] = true
									break // Exit the peer loop on success
								}
							}
						} else {
							lastErr = err
							slog.ErrorContext(h.ctx, "Submission failed with peer",
								"peer", peerID, "error", err)
						}
					}
				}
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
