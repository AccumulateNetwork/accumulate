// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Default configuration values for certificate syncing.
const (
	// DefaultSyncBatchInterval is the time to collect missing digests before sending a request.
	DefaultSyncBatchInterval = 50 * time.Millisecond

	// DefaultSyncDeduplicationInterval is the minimum time between requesting the same digest.
	DefaultSyncDeduplicationInterval = 5 * time.Second

	// DefaultSyncRetryTimeout is the time to wait before retrying a request.
	DefaultSyncRetryTimeout = 10 * time.Second

	// DefaultSyncMaxRetries is the maximum number of retry attempts.
	DefaultSyncMaxRetries = 3

	// DefaultSyncJitterMax is the maximum random jitter added to requests.
	DefaultSyncJitterMax = 100 * time.Millisecond
)

// CertSyncerConfig holds configuration for the CertSyncer.
type CertSyncerConfig struct {
	// BatchInterval is the time to collect missing digests before sending a request.
	BatchInterval time.Duration

	// DeduplicationInterval is the minimum time between requesting the same digest.
	DeduplicationInterval time.Duration

	// RetryTimeout is the time to wait before retrying a request.
	RetryTimeout time.Duration

	// MaxRetries is the maximum number of retry attempts per digest.
	MaxRetries int

	// JitterMax is the maximum random jitter added to batch requests.
	JitterMax time.Duration

	// PublicKey is this node's public key for identifying ourselves in requests.
	PublicKey ed25519.PublicKey
}

// applyDefaults fills in default values for unset configuration fields.
func (c *CertSyncerConfig) applyDefaults() {
	if c.BatchInterval <= 0 {
		c.BatchInterval = DefaultSyncBatchInterval
	}
	if c.DeduplicationInterval <= 0 {
		c.DeduplicationInterval = DefaultSyncDeduplicationInterval
	}
	if c.RetryTimeout <= 0 {
		c.RetryTimeout = DefaultSyncRetryTimeout
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = DefaultSyncMaxRetries
	}
	if c.JitterMax <= 0 {
		c.JitterMax = DefaultSyncJitterMax
	}
}

// CertSyncer handles requesting missing certificates from peers.
// It implements batching, deduplication, and retry logic.
type CertSyncer struct {
	config  CertSyncerConfig
	dag     *dag.DAG
	gossip  *gossip.GossipLayer
	pending *PendingCertificates

	// Track in-flight requests to avoid duplicates
	inFlight   map[types.CertificateDigest]*inFlightRequest
	inFlightMu sync.Mutex

	// Batch pending digest requests
	batchQueue   []types.CertificateDigest
	batchQueueMu sync.Mutex
	batchTimer   *time.Timer

	// Request ID counter
	nextReqID atomic.Uint64

	// Callback for received certificates
	onCertReceived func(*types.Certificate)

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	requestsSent     atomic.Uint64
	responsesRecv    atomic.Uint64
	certificatesRecv atomic.Uint64
}

// inFlightRequest tracks a pending sync request.
type inFlightRequest struct {
	digest    types.CertificateDigest
	requestID uint64
	sentAt    time.Time
	retries   int
}

// NewCertSyncer creates a new CertSyncer.
func NewCertSyncer(
	config CertSyncerConfig,
	d *dag.DAG,
	g *gossip.GossipLayer,
	pending *PendingCertificates,
) *CertSyncer {
	config.applyDefaults()

	return &CertSyncer{
		config:   config,
		dag:      d,
		gossip:   g,
		pending:  pending,
		inFlight: make(map[types.CertificateDigest]*inFlightRequest),
	}
}

// SetCertReceivedCallback sets the callback to invoke when a certificate is received.
func (s *CertSyncer) SetCertReceivedCallback(cb func(*types.Certificate)) {
	s.onCertReceived = cb
}

// Start begins the certificate syncer's main loop.
func (s *CertSyncer) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Start handlers
	s.wg.Add(2)
	go s.handleSyncRequests()
	go s.handleSyncResponses()

	// Start retry loop
	s.wg.Add(1)
	go s.retryLoop()

	slog.Info("CertSyncer started")
	return nil
}

// Stop stops the certificate syncer.
func (s *CertSyncer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	slog.Info("CertSyncer stopped",
		"requestsSent", s.requestsSent.Load(),
		"responsesRecv", s.responsesRecv.Load(),
		"certificatesRecv", s.certificatesRecv.Load())
}

// RequestMissing queues a request for missing certificates.
// Requests are batched and deduplicated before being sent.
func (s *CertSyncer) RequestMissing(digests []types.CertificateDigest) {
	if len(digests) == 0 {
		return
	}

	s.batchQueueMu.Lock()
	defer s.batchQueueMu.Unlock()

	now := time.Now()

	for _, digest := range digests {
		// Check if we already have it in the DAG
		if s.dag.Contains(digest) {
			continue
		}

		// Check deduplication
		s.inFlightMu.Lock()
		if req, exists := s.inFlight[digest]; exists {
			// Check if we should retry
			if now.Sub(req.sentAt) < s.config.DeduplicationInterval {
				s.inFlightMu.Unlock()
				continue
			}
			// Too many retries
			if req.retries >= s.config.MaxRetries {
				s.inFlightMu.Unlock()
				continue
			}
		}
		s.inFlightMu.Unlock()

		// Add to batch queue
		s.batchQueue = append(s.batchQueue, digest)
	}

	// Start batch timer if not already running
	if len(s.batchQueue) > 0 && s.batchTimer == nil {
		// Add jitter to prevent thundering herd
		jitter := time.Duration(rand.Int64N(int64(s.config.JitterMax)))
		s.batchTimer = time.AfterFunc(s.config.BatchInterval+jitter, s.sendBatchRequest)
	}
}

// sendBatchRequest sends a batch of certificate sync requests.
func (s *CertSyncer) sendBatchRequest() {
	s.batchQueueMu.Lock()
	if len(s.batchQueue) == 0 {
		s.batchTimer = nil
		s.batchQueueMu.Unlock()
		return
	}

	// Get digests to request
	digests := s.batchQueue
	s.batchQueue = nil
	s.batchTimer = nil
	s.batchQueueMu.Unlock()

	// Filter digests we still need
	now := time.Now()
	var toRequest []types.CertificateDigest

	s.inFlightMu.Lock()
	for _, digest := range digests {
		// Skip if we now have it
		if s.dag.Contains(digest) {
			continue
		}

		// Skip if recently requested
		if req, exists := s.inFlight[digest]; exists {
			if now.Sub(req.sentAt) < s.config.DeduplicationInterval {
				continue
			}
			// Update retry count
			req.retries++
			req.sentAt = now
			req.requestID = s.nextReqID.Add(1)
			toRequest = append(toRequest, digest)
		} else {
			// New request
			reqID := s.nextReqID.Add(1)
			s.inFlight[digest] = &inFlightRequest{
				digest:    digest,
				requestID: reqID,
				sentAt:    now,
				retries:   0,
			}
			toRequest = append(toRequest, digest)
		}
	}
	s.inFlightMu.Unlock()

	if len(toRequest) == 0 {
		return
	}

	// Create and send request
	req := &gossip.CertSyncRequest{
		Digests:   toRequest,
		Requester: s.config.PublicKey,
		RequestID: s.nextReqID.Add(1),
	}

	slog.Info("Requesting missing certificates",
		"count", len(toRequest))

	// Skip if no gossip layer available
	if s.gossip == nil {
		return
	}

	if err := s.gossip.BroadcastSyncRequest(s.ctx, req); err != nil {
		slog.Warn("Failed to broadcast sync request",
			"error", err)
		return
	}

	s.requestsSent.Add(1)
}

// handleSyncRequests processes incoming sync requests from peers.
func (s *CertSyncer) handleSyncRequests() {
	defer s.wg.Done()

	// Skip if no gossip layer
	if s.gossip == nil {
		<-s.ctx.Done()
		return
	}

	requests := s.gossip.SubscribeSyncRequests()

	for {
		select {
		case <-s.ctx.Done():
			return

		case req, ok := <-requests:
			if !ok {
				return
			}
			s.handleSyncRequest(req)
		}
	}
}

// handleSyncRequest processes a single sync request.
func (s *CertSyncer) handleSyncRequest(req *gossip.CertSyncRequest) {
	if req == nil || len(req.Digests) == 0 {
		return
	}

	// Build response
	var certs []*types.Certificate
	var missing []types.CertificateDigest

	for _, digest := range req.Digests {
		cert := s.dag.GetByDigest(digest)
		if cert != nil {
			certs = append(certs, cert)
			// Limit response size
			if len(certs) >= gossip.MaxSyncCertificates {
				break
			}
		} else {
			missing = append(missing, digest)
		}
	}

	// Only respond if we have certificates to send
	if len(certs) == 0 {
		return
	}

	resp := &gossip.CertSyncResponse{
		Certificates: certs,
		RequestID:    req.RequestID,
		Missing:      missing,
	}

	// Skip if no gossip layer
	if s.gossip == nil {
		return
	}

	if err := s.gossip.BroadcastSyncResponse(s.ctx, resp); err != nil {
		slog.Debug("Failed to send sync response",
			"error", err)
		return
	}

	slog.Debug("Sent sync response",
		"certificates", len(certs),
		"missing", len(missing),
		"requestID", req.RequestID)
}

// handleSyncResponses processes incoming sync responses from peers.
func (s *CertSyncer) handleSyncResponses() {
	defer s.wg.Done()

	// Skip if no gossip layer
	if s.gossip == nil {
		<-s.ctx.Done()
		return
	}

	responses := s.gossip.SubscribeSyncResponses()

	for {
		select {
		case <-s.ctx.Done():
			return

		case resp, ok := <-responses:
			if !ok {
				return
			}
			s.handleSyncResponse(resp)
		}
	}
}

// handleSyncResponse processes a single sync response.
func (s *CertSyncer) handleSyncResponse(resp *gossip.CertSyncResponse) {
	if resp == nil || len(resp.Certificates) == 0 {
		return
	}

	s.responsesRecv.Add(1)

	for _, cert := range resp.Certificates {
		if cert == nil {
			continue
		}

		digest := cert.Digest()

		// Remove from in-flight
		s.inFlightMu.Lock()
		delete(s.inFlight, digest)
		s.inFlightMu.Unlock()

		// Invoke callback if set
		if s.onCertReceived != nil {
			s.onCertReceived(cert)
		}

		s.certificatesRecv.Add(1)
	}

	slog.Debug("Received sync response",
		"certificates", len(resp.Certificates),
		"requestID", resp.RequestID)
}

// retryLoop periodically checks for timed-out requests and retries them.
func (s *CertSyncer) retryLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.RetryTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return

		case <-ticker.C:
			s.checkRetries()
		}
	}
}

// checkRetries finds timed-out requests and queues them for retry.
func (s *CertSyncer) checkRetries() {
	now := time.Now()
	var toRetry []types.CertificateDigest

	s.inFlightMu.Lock()
	for digest, req := range s.inFlight {
		// Check if we now have it
		if s.dag.Contains(digest) {
			delete(s.inFlight, digest)
			continue
		}

		// Check if timed out
		if now.Sub(req.sentAt) < s.config.RetryTimeout {
			continue
		}

		// Check retry limit
		if req.retries >= s.config.MaxRetries {
			slog.Debug("Giving up on certificate sync",
				"digest", digest.String(),
				"retries", req.retries)
			delete(s.inFlight, digest)
			continue
		}

		toRetry = append(toRetry, digest)
	}
	s.inFlightMu.Unlock()

	// Queue for retry
	if len(toRetry) > 0 {
		slog.Debug("Retrying certificate sync",
			"count", len(toRetry))
		s.RequestMissing(toRetry)
	}
}

// Metrics returns syncer metrics.
func (s *CertSyncer) Metrics() (requestsSent, responsesRecv, certificatesRecv uint64) {
	return s.requestsSent.Load(), s.responsesRecv.Load(), s.certificatesRecv.Load()
}

// InFlightCount returns the number of in-flight sync requests.
func (s *CertSyncer) InFlightCount() int {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	return len(s.inFlight)
}
