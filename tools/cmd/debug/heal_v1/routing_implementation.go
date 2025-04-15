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

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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

// setupWithNormalizedRouting sets up the healer with a normalizing router
func (h *healer) setupWithNormalizedRouting(ctx context.Context, network string) {
	// Call the original setup function
	h.setup(ctx, network)

	// Wrap the router with our normalizing router
	if h.router != nil {
		h.router = &NormalizingRouter{Router: h.router}
		slog.Info("Enhanced router with URL normalization")
	}

	// Update the client to use our normalizing router
	if c, ok := h.C2.Transport.(*message.RoutedTransport); ok {
		c.Router = &NormalizingRouter{Router: c.Router}
		slog.Info("Enhanced client transport with URL normalization")
	}
}

// enhancedSubmitLoop is an improved version of submitLoop that handles routing conflicts
func (h *healer) enhancedSubmitLoop(wg *sync.WaitGroup) {
	defer wg.Done()
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()

	// Cache for successful submissions to avoid redundant work
	submissionCache := make(map[string]bool)

	// Track problematic peers to avoid retrying them too often
	problematicPeers := make(map[peer.ID]time.Time)

	// Track successful peers for each partition to prioritize them
	successfulPeers := make(map[string][]peer.ID) // partition -> []peer.ID

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

		// Normalize URLs in all messages before creating the envelope
		normalizedMessages := make([]messaging.Message, 0, len(messages))
		for _, msg := range messages {
			// Create a copy of the message to avoid modifying the original
			normalizeUrlsInMessage(msg)
			normalizedMessages = append(normalizedMessages, msg)
		}

		env := &messaging.Envelope{Messages: normalizedMessages}

		// Generate a cache key based on the envelope hash
		cacheKey := fmt.Sprintf("%x", env.Hash())
		if _, ok := submissionCache[cacheKey]; ok {
			slog.Info("Skipping cached submission", "id", env.Messages[0].ID())
			messages = messages[:0]
			continue
		}

		// Try to determine the target partition for this message
		var targetPartition string
		if h.router != nil {
			// Try to route the envelope to determine the target partition
			partition, err := h.router.Route(env)
			if err == nil {
				targetPartition = partition
				slog.InfoContext(h.ctx, "Routed message to partition", "partition", targetPartition, "id", env.Messages[0].ID())
			} else {
				slog.WarnContext(h.ctx, "Failed to route message", "error", err, "id", env.Messages[0].ID())
			}
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
					submissionCache[cacheKey] = true
				} else {
					slog.ErrorContext(h.ctx, "Submission failed", "message", sub, "status", sub.Status)
				}
			}
		} else {
			lastErr = err
			slog.ErrorContext(h.ctx, "Default submission failed, trying individual peers",
				"error", err, "id", env.Messages[0].ID())

			// Prioritize partitions to try
			var partitionsToTry []string

			// If we know the target partition, try it first
			if targetPartition != "" {
				partitionsToTry = append(partitionsToTry, targetPartition)
			}

			// Add all other partitions
			for _, partition := range h.net.Status.Network.Partitions {
				if partition.ID != targetPartition {
					partitionsToTry = append(partitionsToTry, partition.ID)
				}
			}

			// Try each partition's peers
			for _, partitionID := range partitionsToTry {
				// Skip if we've already submitted successfully
				if submitted {
					break
				}

				slog.InfoContext(h.ctx, "Trying peers from partition", "partition", partitionID)

				// Make sure peers map is initialized for this partition
				if h.net == nil || h.net.Peers == nil || h.net.Peers[strings.ToLower(partitionID)] == nil {
					slog.WarnContext(h.ctx, "No peers available for partition", "partition", partitionID)
					continue
				}

				// Get all peers for this partition
				allPeers := make([]peer.ID, 0, len(h.net.Peers[strings.ToLower(partitionID)]))
				for peerID := range h.net.Peers[strings.ToLower(partitionID)] {
					// Skip problematic peers that failed recently
					if lastFailure, ok := problematicPeers[peerID]; ok {
						if time.Since(lastFailure) < 5*time.Minute {
							continue
						}
					}
					allPeers = append(allPeers, peerID)
				}

				// Prioritize successful peers
				var peersToTry []peer.ID

				// First add successful peers for this partition
				if successfulPeers[partitionID] != nil {
					for _, peerID := range successfulPeers[partitionID] {
						// Check if peer is still in the network
						if _, ok := h.net.Peers[strings.ToLower(partitionID)][peerID]; ok {
							peersToTry = append(peersToTry, peerID)
						}
					}
				}

				// Then add all other peers
				for _, peerID := range allPeers {
					// Check if this peer is already in our priority list
					alreadyAdded := false
					for _, p := range peersToTry {
						if p == peerID {
							alreadyAdded = true
							break
						}
					}

					if !alreadyAdded {
						peersToTry = append(peersToTry, peerID)
					}
				}

				// Try each peer
				for _, peerID := range peersToTry {
					info := h.net.Peers[strings.ToLower(partitionID)][peerID]

					// Create a client for this peer
					c := h.C2.ForPeer(peerID)
					if len(info.Addresses) > 0 {
						c = c.ForAddress(info.Addresses[0])
					}

					slog.InfoContext(h.ctx, "Trying submission with peer", "peer", peerID, "partition", partitionID)

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

								// Add to successful peers list if not already there
								found := false
								for _, p := range successfulPeers[partitionID] {
									if p == peerID {
										found = true
										break
									}
								}
								if !found {
									successfulPeers[partitionID] = append(successfulPeers[partitionID], peerID)
								}
							} else {
								slog.ErrorContext(h.ctx, "Submission failed with peer",
									"peer", peerID, "message", sub, "status", sub.Status)
							}
						}

						if submitted {
							submissionCache[cacheKey] = true
							break // Exit the peer loop on success
						}
					} else {
						lastErr = err
						slog.ErrorContext(h.ctx, "Submission failed with peer",
							"peer", peerID, "error", err)

						// Mark this peer as problematic
						problematicPeers[peerID] = time.Now()

						// If this is a routing conflict, try direct addressing
						if errors.Is(err, errors.BadRequest) && strings.Contains(err.Error(), "conflicting routes") {
							slog.WarnContext(h.ctx, "Detected routing conflict, trying direct addressing",
								"peer", peerID, "error", err)

							// Create a direct address to this peer
							var addr multiaddr.Multiaddr
							if len(info.Addresses) > 0 {
								addr = info.Addresses[0]
							} else {
								peerAddr, _ := multiaddr.NewComponent("p2p", peerID.String())
								if peerAddr != nil {
									addr = peerAddr
								}
							}

							if addr != nil {
								// Create a client with direct addressing
								directClient := h.C2.ForAddress(addr)

								submitCtx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
								subs, err = directClient.Submit(submitCtx, env, api.SubmitOptions{})
								cancel()

								if err == nil {
									for _, sub := range subs {
										if sub.Success {
											slog.InfoContext(h.ctx, "Submission succeeded with direct addressing",
												"peer", peerID, "id", sub.Status.TxID)
											submitted = true
										}
									}
									if submitted {
										submissionCache[cacheKey] = true
										break // Exit the peer loop on success
									}
								}
							}
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

// createDirectRouter creates a router that uses direct addressing for problematic URLs
func createDirectRouter(baseRouter routing.Router, overrides map[string]string) routing.Router {
	return &DirectRouter{
		BaseRouter: baseRouter,
		Overrides:  overrides,
	}
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

// Route routes envelopes, using overrides for problematic URLs
func (r *DirectRouter) Route(envs ...*messaging.Envelope) (string, error) {
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

	// Fall back to the base router
	return r.BaseRouter.Route(normalizedEnvs...)
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

// healSingleAnchorWithNormalizedUrls is an improved version of healSingleAnchor that uses normalized URLs
func (h *healer) healSingleAnchorWithNormalizedUrls(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
	// Normalize URLs to ensure consistent routing
	srcUrl, dstUrl := normalizeAnchorUrl(srcId, dstId)

	slog.InfoContext(h.ctx, "Healing anchor with normalized URLs",
		"source", srcUrl, "destination", dstUrl, "number", seqNum)

	var count int
retry:
	err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
		Client:  h.C2.ForAddress(nil),
		Querier: h.tryEach(),
		NetInfo: h.net,
		Known:   txns,
		Pretend: pretend,
		Wait:    waitForTxn,
		Submit: func(m ...messaging.Message) error {
			// Normalize URLs in all messages before submission
			for _, msg := range m {
				normalizeUrlsInMessage(msg)
			}

			select {
			case h.submit <- m:
				return nil
			case <-h.ctx.Done():
				return errors.NotReady.With("canceled")
			}
		},
	}, healing.SequencedInfo{
		Source:      srcId,
		Destination: dstId,
		Number:      seqNum,
		ID:          txid,
	})
	if err == nil {
		return false
	}
	if errors.Is(err, errors.Delivered) {
		return true
	}
	if !errors.Is(err, healing.ErrRetry) {
		slog.Error("Failed to heal", "source", srcId, "destination", dstId, "number", seqNum, "error", err)
		return false
	}

	count++
	if count >= 10 {
		slog.Error("Anchor still pending, skipping", "attempts", count)
		return false
	}
	slog.Error("Anchor still pending, retrying", "attempts", count)
	goto retry
}
