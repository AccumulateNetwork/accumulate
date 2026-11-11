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

	// Request ID counter, started at a random value (#4470)
	nextReqID atomic.Uint64

	// Response suppression (#4057): requests are broadcast, so peers skip
	// answering what another peer's response has already answered. Per
	// request, whether a response completed it, by the rule the requester
	// stops reading answers by (completes), and otherwise the digests of
	// the certificates the responses carried that checked (#4597): a
	// response that carried junk, or part of the answer, suppresses
	// nothing it did not deliver. An entry holds at most what one response
	// carries, and the requests are kept in two generations, the current
	// and the previous, so forgetting is a generation at a time and never
	// everything at once.
	answered     map[requestKey]*answeredRequest
	answeredPrev map[requestKey]*answeredRequest
	answeredMu   sync.Mutex

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

	s := &CertSyncer{
		answered: map[requestKey]*answeredRequest{},
		config:   config,
		dag:      d,
		gossip:   g,
		pending:  pending,
		inFlight: make(map[types.CertificateDigest]*inFlightRequest),
	}
	// A node restarted under the same key must not reuse the IDs its peers
	// saw answered before it stopped (#4470). Half the range leaves room to
	// count up.
	s.nextReqID.Store(rand.Uint64() >> 1)
	return s
}

// requestKey names a request: each node numbers only its own requests, so
// suppression keyed on the ID alone skips node B's request k because node
// A's request k was answered (#4470).
type requestKey struct {
	requester string
	id        uint64
}

func keyOf(requester []byte, id uint64) requestKey {
	return requestKey{string(requester), id}
}

// maxAnsweredRequests is how many requests one generation of suppression
// remembers; past it the previous generation is forgotten.
const maxAnsweredRequests = 4096

// answeredRequest is what the responses seen to one request did: completed
// it, or delivered these certificates.
type answeredRequest struct {
	whole   bool
	digests map[types.CertificateDigest]struct{}
}

// completes reports whether a response completes its request: every
// certificate in it checked (good) and it reports nothing missing. The
// requester stops reading answers to the request on such a response
// (readSyncResponses), so a peer answers it no more (noteDelivered): the
// one rule on both sides, so they cannot drift apart (#4597).
func completes(good bool, resp *gossip.CertSyncResponse) bool {
	return good && len(resp.Missing) == 0
}

// noteDelivered records that a response to the request named by key
// completed it (whole), or delivered these certificates, at most
// MaxSyncCertificates per request: one requester and ID cannot be filled
// with the whole DAG. A request completed stays completed.
func (s *CertSyncer) noteDelivered(key requestKey, digests []types.CertificateDigest, whole bool) {
	if !whole && len(digests) == 0 {
		return
	}
	s.answeredMu.Lock()
	defer s.answeredMu.Unlock()
	have := s.answered[key]
	if have == nil {
		if len(s.answered) > maxAnsweredRequests {
			s.answeredPrev, s.answered = s.answered, map[requestKey]*answeredRequest{}
		}
		// A request still being answered moves to the current generation.
		if have = s.answeredPrev[key]; have == nil {
			have = &answeredRequest{}
		} else {
			delete(s.answeredPrev, key)
		}
		s.answered[key] = have
	}
	if whole || have.whole {
		have.whole, have.digests = true, nil
		return
	}
	if have.digests == nil {
		have.digests = map[types.CertificateDigest]struct{}{}
	}
	for _, d := range digests {
		if len(have.digests) >= gossip.MaxSyncCertificates {
			break
		}
		have.digests[d] = struct{}{}
	}
}

// withoutDelivered is certs less those a response to the request named by
// key has delivered, and nothing once a response completed the request.
func (s *CertSyncer) withoutDelivered(key requestKey, certs []*types.Certificate) []*types.Certificate {
	s.answeredMu.Lock()
	defer s.answeredMu.Unlock()
	var current, previous map[types.CertificateDigest]struct{}
	for _, have := range []*answeredRequest{s.answered[key], s.answeredPrev[key]} {
		if have == nil {
			continue
		}
		if have.whole {
			return nil
		}
		if current == nil {
			current = have.digests
		} else {
			previous = have.digests
		}
	}
	if len(current) == 0 && len(previous) == 0 {
		return certs
	}
	kept := make([]*types.Certificate, 0, len(certs))
	for _, cert := range certs {
		d := cert.Digest()
		if _, done := current[d]; done {
			continue
		}
		if _, done := previous[d]; done {
			continue
		}
		kept = append(kept, cert)
	}
	return kept
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

// ForgetInFlight drops every request in flight, so that the next ask for any
// of the digests goes out at once. A seed (Node.Rejoin, Primary.ForgetHeld)
// calls it: a certificate answered before the seed and held for want of
// parents does not check, so its digest stays in flight (#4597), and the
// seed that makes it acceptable without them forgets the held copy, not the
// request. Its next ask was deduplicated against the request the answer had
// already met (#4610).
func (s *CertSyncer) ForgetInFlight() {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	s.inFlight = make(map[types.CertificateDigest]*inFlightRequest)
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
	// requester with duplicates (#4057). Delay a beat, then answer with what
	// another peer's response has not already delivered.
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

	resp := s.answer(req)
	if resp == nil {
		return
	}

	// Skip if no gossip layer
	if s.gossip == nil {
		return
	}

	if err := s.gossip.BroadcastSyncResponse(s.ctx, resp); err != nil {
		slog.Warn("Failed to send sync response",
			"partition", s.config.Partition,
			"error", err, "certificates", len(resp.Certificates))
		return
	}
	s.served("answered")

	slog.Debug("Sent sync response",
		"certificates", len(resp.Certificates),
		"missing", len(resp.Missing),
		"rounds", len(req.Rounds),
		"requestID", req.RequestID)
}

// answer builds the response to req from what this node holds, less what a
// response to the same request has already delivered. It is nil, and the
// request counted empty or suppressed, when nothing is left to send.
//
// A request this node has seen completed, by a response in which every
// certificate checked and nothing was missing (completes), is not
// answered: the requester reads no more answers to it, so the answer
// would be a broadcast every node relays and the requester drops, the
// flood #4057 added suppression against. A request seen only partly or
// badly answered is answered with what no response delivered (#4597): a
// response counts only for the certificates in it that checked
// (handleSyncResponse), so a forged response suppresses nothing and a
// partial one suppresses only what it carried.
func (s *CertSyncer) answer(req *gossip.CertSyncRequest) *gossip.CertSyncResponse {
	key := keyOf(req.Requester, req.RequestID)
	var certs []*types.Certificate
	var missing []types.CertificateDigest
	held := 0

	// add keeps what no response has delivered, up to what one response
	// carries. What was delivered is taken out before the cap, or the
	// certificates past the cap would never be served (#4597).
	add := func(found []*types.Certificate) {
		held += len(found)
		for _, cert := range s.withoutDelivered(key, found) {
			if len(certs) >= gossip.MaxSyncCertificates {
				return
			}
			certs = append(certs, cert)
		}
	}

	var found []*types.Certificate
	for _, digest := range req.Digests {
		if cert := s.dag.GetByDigest(digest); cert != nil {
			found = append(found, cert)
		} else {
			missing = append(missing, digest)
		}
	}
	add(found)

	// Serve whole rounds for round catch-up (#4057)
	for _, round := range req.Rounds {
		if len(certs) >= gossip.MaxSyncCertificates {
			break
		}
		add(s.dag.GetRound(round))
	}

	// Only respond if we have certificates to send
	if held == 0 {
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
		return nil
	}
	if len(certs) == 0 {
		s.served("suppressed")
		return nil
	}

	return &gossip.CertSyncResponse{
		Certificates: certs,
		RequestID:    req.RequestID,
		Requester:    req.Requester,
		Missing:      missing,
	}
}

// handleSyncResponses processes incoming sync responses from peers.
func (s *CertSyncer) handleSyncResponses() {
	defer s.wg.Done()

	// Skip if no gossip layer
	if s.gossip == nil {
		<-s.ctx.Done()
		return
	}

	s.readSyncResponses(s.gossip.SubscribeSyncResponses())
}

// readSyncResponses handles the responses as they arrive, one complete
// answer to each request: a request answered with nothing reported missing
// is done, and later answers to it are dropped unread, whoever asked. A
// partial answer leaves the request open for the next.
//
// Responses are broadcast, so every node receives the answers to every
// node's requests, and handles them one at a time. Suppression keeps a peer
// from answering a request it has seen answered, but under load it has not
// seen the first answer yet when its own delay runs out. A joining
// follower's requests were answered by 8 of its 12 peers at the median,
// each answer up to 100 certificates. At the rate a loaded CI runner
// handled responses (3 to 29 a second), its own answers waited 24 s at the
// median behind copies of answers it already had, its queue overflowed and
// dropped the next ones, and it climbed toward the frontier more slowly than
// the frontier moved (#4610).
func (s *CertSyncer) readSyncResponses(responses <-chan *gossip.CertSyncResponse) {
	complete := map[requestKey]struct{}{}
	for {
		select {
		case <-s.ctx.Done():
			return

		case resp, ok := <-responses:
			if !ok {
				return
			}
			if resp == nil || len(resp.Certificates) == 0 {
				continue
			}
			key := keyOf(resp.Requester, resp.RequestID)
			if _, done := complete[key]; done {
				s.window.repeatedAnswer()
				continue
			}
			if completes(s.handleSyncResponse(resp), resp) {
				if len(complete) > 4096 {
					complete = map[requestKey]struct{}{}
				}
				complete[key] = struct{}{}
			}
		}
	}
}

// handleSyncResponse processes a single sync response. It reports whether
// every certificate in it checked. A response is unsigned and anyone can
// send one, so it completes a request only once every certificate in it
// checks and it reports nothing missing (completes): a response with a
// certificate that does not check leaves the digests it carried in flight
// to be retried. A response that completes the request is recorded as
// answering it whole, and this peer answers it no more. One that does not
// is recorded as having delivered the certificates in it that checked, and
// only those (#4597), so a forged response suppresses nothing, and a
// partial one only what it carried, leaving the rest to this peer's own
// answer.
func (s *CertSyncer) handleSyncResponse(resp *gossip.CertSyncResponse) bool {
	if resp == nil || len(resp.Certificates) == 0 {
		return false
	}

	s.responsesRecv.Add(1)
	metrics.CertSyncResponsesReceivedTotal.WithLabelValues(s.config.Partition).Inc()
	s.window.response()

	// What this response delivered: the certificates in it that checked.
	// Only these suppress this node's own answer to the request (#4597).
	good := true
	var delivered []types.CertificateDigest
	for _, cert := range resp.Certificates {
		if cert == nil {
			continue
		}

		if !s.checks(cert) {
			good = false
			continue
		}

		digest := cert.Digest()

		// Remove from in-flight
		s.inFlightMu.Lock()
		delete(s.inFlight, digest)
		s.inFlightMu.Unlock()

		delivered = append(delivered, digest)
		s.certificatesRecv.Add(1)
	}

	s.noteDelivered(keyOf(resp.Requester, resp.RequestID), delivered, completes(good, resp))

	if !good {
		slog.Debug("Sync response with certificates that do not check",
			"certificates", len(resp.Certificates),
			"requestID", resp.RequestID)
		return false
	}

	slog.Debug("Received sync response",
		"certificates", len(resp.Certificates),
		"requestID", resp.RequestID)
	return true
}

// checks hands a received certificate to the receiver and reports whether
// it checked: it was verified and inserted, or the receiver holds it in
// the DAG and the copy is the held certificate byte for byte (the
// receiver's duplicate rule, receiveCertificate). Nothing else counts: the
// receiver reports a certificate waiting for its parents before verifying
// it (#4597). Without a receiver nothing checks.
func (s *CertSyncer) checks(cert *types.Certificate) bool {
	if s.onCertReceived == nil {
		return false
	}
	outcome := s.onCertReceived(cert)
	metrics.CertSyncCertificatesReceivedTotal.WithLabelValues(s.config.Partition, string(outcome)).Inc()
	s.window.certificate(outcome)
	return outcome == CertInserted || outcome == CertDuplicate
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
	if w.repeated > 0 {
		args = append(args, "repeatedAnswers", w.repeated)
	}
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
	args = append(args, "verified", w.verifications)
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
	// repeated counts answers dropped unread because their request had
	// already been answered in full (#4610).
	repeated int
	// verifications counts received certificates verified against the
	// committee, from gossip as well as sync. It is reported with the rest
	// but does not by itself make the line worth writing (#4469).
	verifications int
}

func (c syncWindowCounts) isEmpty() bool {
	return c.roundRequests == 0 && c.digestRequests == 0 && c.responses == 0 &&
		c.answered == 0 && c.suppressed == 0 && c.empty == 0 && c.repeated == 0
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

func (w *syncWindow) repeatedAnswer() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.repeated++
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

func (w *syncWindow) verification() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.verifications++
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
