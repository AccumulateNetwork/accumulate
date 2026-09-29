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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Default configuration values for certificate syncing.
const (
	// DefaultSyncBatchInterval is the time to collect missing digests before sending a request.
	DefaultSyncBatchInterval = 50 * time.Millisecond

	// DefaultSyncDeduplicationInterval is the minimum time between requesting the same digest.
	DefaultSyncDeduplicationInterval = 5 * time.Second

	// DefaultSyncRetryTimeout is the time to wait before retrying a request.
	// Increased from 10s to 30s to handle network congestion under sustained load.
	DefaultSyncRetryTimeout = 30 * time.Second

	// DefaultSyncMaxRetries is the maximum number of retry attempts.
	// Increased from 3 to 10 to be more persistent under sustained load.
	DefaultSyncMaxRetries = 10

	// DefaultSyncJitterMax is the maximum random jitter added to requests.
	DefaultSyncJitterMax = 100 * time.Millisecond

	// DefaultSyncPendingRescanInterval is how often to rescan pending certificates
	// and re-request their missing parents. This ensures recovery from dropped sync responses.
	DefaultSyncPendingRescanInterval = 15 * time.Second

	// DefaultSyncSummaryInterval is how often the catch-up summary is logged
	// at Info, when there was catch-up traffic in the interval.
	DefaultSyncSummaryInterval = 10 * time.Second
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

	// PendingRescanInterval is how often to rescan pending certificates and
	// re-request their missing parents. Set to 0 to disable.
	PendingRescanInterval time.Duration

	// PublicKey is this node's public key for identifying ourselves in requests.
	PublicKey ed25519.PublicKey

	// Partition labels the syncer's metrics and log lines.
	Partition string

	// SummaryInterval is how often the catch-up summary is logged.
	SummaryInterval time.Duration
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
	if c.PendingRescanInterval <= 0 {
		c.PendingRescanInterval = DefaultSyncPendingRescanInterval
	}
	if c.SummaryInterval <= 0 {
		c.SummaryInterval = DefaultSyncSummaryInterval
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

	// Response suppression (#4057): requests are broadcast, so peers skip
	// answering when another peer's response has already been observed
	answered   map[uint64]struct{}
	answeredMu sync.Mutex

	// Callback for received certificates; returns what inserting it did
	onCertReceived func(*types.Certificate) CertOutcome

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	requestsSent     atomic.Uint64
	responsesRecv    atomic.Uint64
	certificatesRecv atomic.Uint64

	// What happened on the catch-up path since the last summary line
	window syncWindow
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
		answered: map[uint64]struct{}{},
		config:   config,
		dag:      d,
		gossip:   g,
		pending:  pending,
		inFlight: make(map[types.CertificateDigest]*inFlightRequest),
	}
}

// SetCertReceivedCallback sets the callback to invoke when a certificate is
// received. It reports what inserting the certificate did.
func (s *CertSyncer) SetCertReceivedCallback(cb func(*types.Certificate) CertOutcome) {
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

	// Start the catch-up summary
	s.wg.Add(1)
	go s.summaryLoop()

	// Start pending rescan loop if configured
	if s.config.PendingRescanInterval > 0 {
		s.wg.Add(1)
		go s.pendingRescanLoop()
	}

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

// RequestRounds requests every certificate of the given rounds from peers.
// Used for round catch-up (#4057): a node that has fallen behind cannot know
// which digests it is missing, only which rounds. The caller is responsible
// for pacing.
func (s *CertSyncer) RequestRounds(rounds []types.Round) {
	if len(rounds) == 0 || s.gossip == nil {
		return
	}
	if len(rounds) > gossip.MaxSyncRounds {
		rounds = rounds[:gossip.MaxSyncRounds]
	}

	req := &gossip.CertSyncRequest{
		Rounds:    rounds,
		Requester: s.config.PublicKey,
		RequestID: s.nextReqID.Add(1),
	}
	if err := s.gossip.BroadcastSyncRequest(s.ctx, req); err != nil {
		slog.Warn("Failed to send round sync request",
			"partition", s.config.Partition,
			"error", err, "start", rounds[0], "end", rounds[len(rounds)-1])
		return
	}
	metrics.CertSyncRequestsSentTotal.WithLabelValues(s.config.Partition, "round").Inc()
	s.window.roundRequest(rounds[0], rounds[len(rounds)-1])
	slog.Debug("Requested certificates by round",
		"start", rounds[0], "end", rounds[len(rounds)-1], "count", len(rounds))
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

	slog.Debug("Requesting missing certificates",
		"count", len(toRequest))

	// Skip if no gossip layer available
	if s.gossip == nil {
		return
	}

	for _, chunk := range digestChunks(toRequest) {
		req := &gossip.CertSyncRequest{
			Digests:   chunk,
			Requester: s.config.PublicKey,
			RequestID: s.nextReqID.Add(1),
		}
		if err := s.gossip.BroadcastSyncRequest(s.ctx, req); err != nil {
			slog.Warn("Failed to broadcast sync request",
				"partition", s.config.Partition,
				"error", err)
			return
		}
		s.requestsSent.Add(1)
		metrics.CertSyncRequestsSentTotal.WithLabelValues(s.config.Partition, "digest").Inc()
		s.window.digestRequest(len(chunk))
	}
}

// digestChunks splits a digest request into requests a response can answer
// whole. A peer answers with at most MaxSyncCertificates certificates and
// silently leaves the rest, which then wait out the retry timeout (30 s)
// in the in-flight set; asking for more than MaxSyncDigests fails to encode
// and nothing is sent at all. A joining node's rescan asks for several
// hundred at once, and the parents it needs next were beyond the first
// hundred (#4469).
func digestChunks(digests []types.CertificateDigest) [][]types.CertificateDigest {
	var chunks [][]types.CertificateDigest
	for len(digests) > 0 {
		n := min(len(digests), gossip.MaxSyncCertificates)
		chunks = append(chunks, digests[:n])
		digests = digests[n:]
	}
	return chunks
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
	if req == nil || (len(req.Digests) == 0 && len(req.Rounds) == 0) {
		return
	}

	// Requests and responses are broadcast, so without suppression every
	// peer answers every request — an N-fold amplification that floods the
	// requester with duplicates (#4057). Delay a beat, then skip if another
	// peer's response for this request has already been observed.
	jitter := time.Duration(rand.Int64N(int64(200 * time.Millisecond)))
	if s.ctx != nil {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(jitter):
		}
	} else {
		time.Sleep(jitter)
	}
	s.answeredMu.Lock()
	_, answered := s.answered[req.RequestID]
	s.answeredMu.Unlock()
	if answered {
		s.served("suppressed")
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

	// Serve whole rounds for round catch-up (#4057)
	for _, round := range req.Rounds {
		for _, cert := range s.dag.GetRound(round) {
			if len(certs) >= gossip.MaxSyncCertificates {
				break
			}
			certs = append(certs, cert)
		}
	}

	// Only respond if we have certificates to send
	if len(certs) == 0 {
		s.served("empty")
		if len(req.Rounds) > 0 {
			// A round request that matches nothing means the requester is
			// asking for rounds we do not have — that is load-bearing
			// diagnosis for round catch-up (#4057), not noise.
			slog.Debug("Sync request matched nothing",
				"rounds", len(req.Rounds),
				"firstRound", req.Rounds[0],
				"lastRound", req.Rounds[len(req.Rounds)-1],
				"requestID", req.RequestID)
		}
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
		slog.Warn("Failed to send sync response",
			"partition", s.config.Partition,
			"error", err, "certificates", len(certs))
		return
	}
	s.served("answered")

	slog.Debug("Sent sync response",
		"certificates", len(certs),
		"missing", len(missing),
		"rounds", len(req.Rounds),
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

	// Note the response so we do not also answer this request (suppression)
	s.answeredMu.Lock()
	if len(s.answered) > 4096 {
		s.answered = make(map[uint64]struct{})
	}
	s.answered[resp.RequestID] = struct{}{}
	s.answeredMu.Unlock()

	s.responsesRecv.Add(1)
	metrics.CertSyncResponsesReceivedTotal.WithLabelValues(s.config.Partition).Inc()
	s.window.response()

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
			outcome := s.onCertReceived(cert)
			metrics.CertSyncCertificatesReceivedTotal.WithLabelValues(s.config.Partition, string(outcome)).Inc()
			s.window.certificate(outcome)
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

// pendingRescanLoop periodically scans pending certificates and re-requests
// their missing parents. This ensures recovery from dropped sync responses
// or situations where requests never got a response.
func (s *CertSyncer) pendingRescanLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.PendingRescanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return

		case <-ticker.C:
			s.rescanPendingCertificates()
		}
	}
}

// rescanPendingCertificates checks all pending certificates and re-requests
// any missing parents that are not currently being actively synced.
func (s *CertSyncer) rescanPendingCertificates() {
	if s.pending == nil {
		return
	}

	// Get all missing parent digests from pending certificates
	allMissing := s.pending.GetAllMissingParents()
	if len(allMissing) == 0 {
		return
	}

	// Filter to digests not in DAG and not recently requested
	var toRequest []types.CertificateDigest
	now := time.Now()

	s.inFlightMu.Lock()
	for _, digest := range allMissing {
		// Skip if we have it
		if s.dag.Contains(digest) {
			continue
		}

		// Check if actively being requested
		if req, exists := s.inFlight[digest]; exists {
			// Only skip if recently requested
			if now.Sub(req.sentAt) < s.config.DeduplicationInterval {
				continue
			}
		}

		toRequest = append(toRequest, digest)
	}
	s.inFlightMu.Unlock()

	if len(toRequest) > 0 {
		slog.Debug("Re-requesting missing certificates from pending rescan",
			"count", len(toRequest),
			"totalPending", s.pending.Size())
		s.RequestMissing(toRequest)
	}
}

// ClearInFlight removes a digest from the in-flight tracking.
// This should be called when a certificate is received via normal gossip
// (not through sync) to prevent stale in-flight entries.
func (s *CertSyncer) ClearInFlight(digest types.CertificateDigest) {
	s.inFlightMu.Lock()
	delete(s.inFlight, digest)
	s.inFlightMu.Unlock()
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

// served records how this node handled a peer's sync request.
func (s *CertSyncer) served(outcome string) {
	metrics.CertSyncRequestsServedTotal.WithLabelValues(s.config.Partition, outcome).Inc()
	s.window.served(outcome)
}

// summaryLoop logs the catch-up summary once per SummaryInterval.
func (s *CertSyncer) summaryLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.SummaryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.logSummary()
		}
	}
}

// logSummary writes one Info line for the interval just ended, if anything
// crossed the catch-up path. The line is what distinguishes "nobody
// answered" (round requests sent, no responses) from "everybody said no"
// (responses received, certificates refused) -- the two need different fixes
// and Debug logging hid both (#4469).
func (s *CertSyncer) logSummary() {
	w := s.window.take()
	if w.isEmpty() {
		return
	}
	args := []any{"partition", s.config.Partition}
	if w.roundRequests > 0 {
		args = append(args, "roundRequests", w.roundRequests, "roundsFrom", w.firstRound, "roundsTo", w.lastRound)
	}
	if w.digestRequests > 0 {
		args = append(args, "digestRequests", w.digestRequests, "digestsAsked", w.digestsAsked)
	}
	args = append(args, "responses", w.responses, "certificates", w.certificates)
	for _, o := range certOutcomes {
		if n := w.outcomes[o]; n > 0 {
			args = append(args, string(o), n)
		}
	}
	if w.lastRefusal != "" {
		args = append(args, "lastRefusal", w.lastRefusal)
	}
	if w.answered+w.suppressed+w.empty > 0 {
		args = append(args, "served", w.answered, "suppressed", w.suppressed, "servedEmpty", w.empty)
	}
	slog.Info("Certificate catch-up", args...)
}

// CertOutcome is what inserting a received certificate did.
type CertOutcome string

const (
	CertInserted     CertOutcome = "inserted"
	CertDuplicate    CertOutcome = "duplicate"
	CertBuffered     CertOutcome = "buffered"     // parents missing, held until they arrive
	CertDropped      CertOutcome = "dropped"      // parents missing and the buffer did not take it
	CertInvalid      CertOutcome = "invalid"      // failed verification against the committee
	CertEquivocation CertOutcome = "equivocation" // a different certificate for the same author and round
	CertFailed       CertOutcome = "failed"       // any other insert error
)

var certOutcomes = []CertOutcome{CertInserted, CertDuplicate, CertBuffered, CertDropped, CertInvalid, CertEquivocation, CertFailed}

// syncWindow accumulates the catch-up path's events between summary lines.
type syncWindow struct {
	mu sync.Mutex
	syncWindowCounts
}

type syncWindowCounts struct {
	roundRequests         int
	firstRound, lastRound types.Round
	digestRequests        int
	digestsAsked          int
	responses             int
	certificates          int
	outcomes              map[CertOutcome]int
	lastRefusal           string
	answered, suppressed  int
	empty                 int
}

func (c syncWindowCounts) isEmpty() bool {
	return c.roundRequests == 0 && c.digestRequests == 0 && c.responses == 0 &&
		c.answered == 0 && c.suppressed == 0 && c.empty == 0
}

func (w *syncWindow) take() syncWindowCounts {
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.syncWindowCounts
	w.syncWindowCounts = syncWindowCounts{}
	return c
}

func (w *syncWindow) roundRequest(first, last types.Round) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.roundRequests == 0 || first < w.firstRound {
		w.firstRound = first
	}
	if last > w.lastRound {
		w.lastRound = last
	}
	w.roundRequests++
}

func (w *syncWindow) digestRequest(n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.digestRequests++
	w.digestsAsked += n
}

func (w *syncWindow) response() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.responses++
}

func (w *syncWindow) certificate(o CertOutcome) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.certificates++
	if w.outcomes == nil {
		w.outcomes = map[CertOutcome]int{}
	}
	w.outcomes[o]++
	switch o {
	case CertDropped, CertInvalid, CertEquivocation, CertFailed:
		w.lastRefusal = string(o)
	}
}

func (w *syncWindow) served(outcome string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch outcome {
	case "answered":
		w.answered++
	case "suppressed":
		w.suppressed++
	case "empty":
		w.empty++
	}
}
