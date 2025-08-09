package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Configuration constants
const (
	DefaultBatchThreshold   = 2
	DefaultMaxBatchSize     = 100
	DefaultProofTimeout     = 30 * time.Second
	DefaultQueueSize        = 1000
	MaxActiveRecoveries     = 100
	SessionCleanupInterval  = 5 * time.Minute
)

// BatchProofConfig contains configuration for the batch proof recovery manager
type BatchProofConfig struct {
	BatchThreshold   int
	MaxBatchSize     int
	ProofTimeout     time.Duration
	QueueSize        int
	EnableMetrics    bool
	MetricsInterval  time.Duration
}

// DefaultBatchProofConfig returns default configuration
func DefaultBatchProofConfig() BatchProofConfig {
	return BatchProofConfig{
		BatchThreshold:  DefaultBatchThreshold,
		MaxBatchSize:    DefaultMaxBatchSize,
		ProofTimeout:    DefaultProofTimeout,
		QueueSize:       DefaultQueueSize,
		EnableMetrics:   true,
		MetricsInterval: 10 * time.Second,
	}
}

// BatchProofRecoveryManager uses collection proofs for efficient batch recovery
type BatchProofRecoveryManager struct {
	conductor    *CrossChainConductor
	logger       logging.OptionalLogger
	config       BatchProofConfig
	
	// Properly typed interfaces
	client       APIClient
	db           Database
	
	// Active recovery sessions with cleanup
	activeRecovery   map[string]*BatchRecoverySession
	recoveryQueue    chan *BatchRecoveryRequest
	mu               sync.RWMutex
	
	// Context for cancellation
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	
	// Thread-safe metrics using atomic operations
	metrics          *BatchProofMetrics
	
	// Circuit breaker for failing partitions
	circuitBreaker   map[string]*CircuitBreaker
	cbMu             sync.RWMutex
}

// BatchProofMetrics contains thread-safe metrics
type BatchProofMetrics struct {
	totalRequests    int64
	batchRequests    int64
	individualProofs int64
	proofSavings     int64
	failedRequests   int64
	activeRecoveries int64
}

// CircuitBreaker prevents repeated failures
type CircuitBreaker struct {
	failures       int32
	lastFailure    time.Time
	state          int32 // 0=closed, 1=open, 2=half-open
	backoffUntil   time.Time
}

// APIClient defines the client interface
type APIClient interface {
	Query(ctx context.Context, url *url.URL) (interface{}, error)
}

// Database defines the database interface
type Database interface {
	BeginTx(ctx context.Context) (interface{}, error)
}

// NewBatchProofRecoveryManager creates a new manager with proper initialization
func NewBatchProofRecoveryManager(config BatchProofConfig, conductor *CrossChainConductor, logger logging.OptionalLogger) *BatchProofRecoveryManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &BatchProofRecoveryManager{
		conductor:       conductor,
		logger:          logger.With("module", "batch-recovery"),
		config:          config,
		activeRecovery:  make(map[string]*BatchRecoverySession),
		recoveryQueue:   make(chan *BatchRecoveryRequest, config.QueueSize),
		ctx:             ctx,
		cancel:          cancel,
		metrics:         &BatchProofMetrics{},
		circuitBreaker:  make(map[string]*CircuitBreaker),
	}
}

// Start begins processing with proper goroutine management
func (brm *BatchProofRecoveryManager) Start() {
	brm.logger.Info("Starting batch proof recovery manager", 
		"config", brm.config)
	
	// Start recovery processor
	brm.wg.Add(1)
	go brm.processRecoveryQueue()
	
	// Start batch optimizer
	brm.wg.Add(1)
	go brm.optimizeBatches()
	
	// Start session cleanup
	brm.wg.Add(1)
	go brm.cleanupSessions()
	
	// Start metrics reporter if enabled
	if brm.config.EnableMetrics {
		brm.wg.Add(1)
		go brm.reportMetrics()
	}
}

// Stop gracefully shuts down all goroutines
func (brm *BatchProofRecoveryManager) Stop() {
	brm.logger.Info("Stopping batch proof recovery manager")
	
	// Signal shutdown
	brm.cancel()
	
	// Close queue
	close(brm.recoveryQueue)
	
	// Wait for all goroutines to finish
	brm.wg.Wait()
	
	brm.logger.Info("Batch proof recovery manager stopped",
		"total_requests", atomic.LoadInt64(&brm.metrics.totalRequests),
		"proof_savings", atomic.LoadInt64(&brm.metrics.proofSavings))
}

// RequestBatchRecovery requests recovery with automatic batch optimization
func (brm *BatchProofRecoveryManager) RequestBatchRecovery(req *BatchRecoveryRequest) error {
	// Check circuit breaker
	if brm.isCircuitOpen(req.PartitionID) {
		atomic.AddInt64(&brm.metrics.failedRequests, 1)
		return fmt.Errorf("circuit breaker open for partition %s", req.PartitionID)
	}
	
	// Non-blocking send with context
	select {
	case brm.recoveryQueue <- req:
		atomic.AddInt64(&brm.metrics.totalRequests, 1)
		return nil
	case <-brm.ctx.Done():
		return brm.ctx.Err()
	default:
		return fmt.Errorf("recovery queue full")
	}
}

// processRecoveryQueue processes incoming recovery requests with context
func (brm *BatchProofRecoveryManager) processRecoveryQueue() {
	defer brm.wg.Done()
	
	for {
		select {
		case req, ok := <-brm.recoveryQueue:
			if !ok {
				return // Queue closed
			}
			brm.handleRecoveryRequest(req)
			
		case <-brm.ctx.Done():
			return
		}
	}
}

// handleRecoveryRequest processes a request with proper synchronization
func (brm *BatchProofRecoveryManager) handleRecoveryRequest(req *BatchRecoveryRequest) {
	// Validate request
	if req == nil || len(req.MissingSequences) == 0 {
		brm.logger.Warn("Invalid recovery request")
		return
	}
	
	sessionKey := fmt.Sprintf("%s-%s", req.PartitionID, req.Type.String())
	
	// Register session with cleanup
	brm.registerSession(sessionKey, req)
	defer brm.cleanupSession(sessionKey)
	
	// Check batch threshold
	if len(req.MissingSequences) >= brm.config.BatchThreshold {
		brm.logger.Info("Using batch proof for recovery",
			"partition", req.PartitionID,
			"sequences", len(req.MissingSequences),
			"threshold", brm.config.BatchThreshold)
		
		// Process with timeout context
		ctx, cancel := context.WithTimeout(brm.ctx, brm.config.ProofTimeout)
		defer cancel()
		
		if err := brm.processBatchRecovery(ctx, req); err != nil {
			brm.handleRecoveryError(req.PartitionID, err)
		} else {
			brm.resetCircuitBreaker(req.PartitionID)
		}
		
		atomic.AddInt64(&brm.metrics.batchRequests, 1)
	} else {
		brm.logger.Info("Using individual proofs for recovery",
			"partition", req.PartitionID,
			"sequences", len(req.MissingSequences))
		
		brm.processIndividualRecovery(brm.ctx, req)
		atomic.AddInt64(&brm.metrics.individualProofs, int64(len(req.MissingSequences)))
	}
}

// processBatchRecovery handles recovery with context and error handling
func (brm *BatchProofRecoveryManager) processBatchRecovery(ctx context.Context, req *BatchRecoveryRequest) error {
	brm.logger.Info("Processing batch recovery",
		"partition", req.PartitionID,
		"type", req.Type,
		"count", len(req.MissingSequences))
	
	startTime := time.Now()
	
	// Check if sequences need sorting (avoid unnecessary copy)
	sequences := req.MissingSequences
	if !sort.SliceIsSorted(sequences, func(i, j int) bool {
		return sequences[i] < sequences[j]
	}) {
		// Only copy if we need to sort
		sequences = append([]uint64(nil), req.MissingSequences...)
		sort.Slice(sequences, func(i, j int) bool {
			return sequences[i] < sequences[j]
		})
	}
	
	// Process in batches with context checking
	batchSize := min(len(sequences), brm.config.MaxBatchSize)
	
	for i := 0; i < len(sequences); i += batchSize {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		end := min(i+batchSize, len(sequences))
		batch := sequences[i:end]
		
		// Validate batch
		if len(batch) == 0 {
			continue
		}
		
		response, err := brm.generateCollectionProof(ctx, req, batch)
		if err != nil {
			brm.logger.Error("Failed to generate collection proof",
				"partition", req.PartitionID,
				"batch_range", fmt.Sprintf("%d-%d", batch[0], batch[len(batch)-1]),
				"error", err)
			
			// Fallback to individual proofs
			if fallbackErr := brm.processIndividualRecoveryBatch(ctx, req, batch); fallbackErr != nil {
				return fmt.Errorf("both collection and individual proofs failed: %w", fallbackErr)
			}
			continue
		}
		
		// Update metrics atomically
		proofSavings := len(batch) - 1
		atomic.AddInt64(&brm.metrics.proofSavings, int64(proofSavings))
		
		response.ProofSavings = proofSavings
		response.ProofGenerated = time.Now()
		
		brm.logger.Info("Generated collection proof",
			"partition", req.PartitionID,
			"batch_size", len(batch),
			"proof_savings", proofSavings,
			"generation_time", time.Since(startTime))
		
		// Non-blocking callback
		if req.Callback != nil {
			go req.Callback(response)
		}
	}
	
	return nil
}

// Session management methods

func (brm *BatchProofRecoveryManager) registerSession(key string, req *BatchRecoveryRequest) {
	brm.mu.Lock()
	defer brm.mu.Unlock()
	
	// Limit active recoveries
	if len(brm.activeRecovery) >= MaxActiveRecoveries {
		// Remove oldest session
		var oldestKey string
		var oldestTime time.Time
		for k, v := range brm.activeRecovery {
			if oldestTime.IsZero() || v.StartTime.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.StartTime
			}
		}
		delete(brm.activeRecovery, oldestKey)
	}
	
	brm.activeRecovery[key] = &BatchRecoverySession{
		PartitionID: req.PartitionID,
		Requests:    []*BatchRecoveryRequest{req},
		StartTime:   time.Now(),
		LastUpdate:  time.Now(),
		Status:      BatchRecoveryPending,
	}
	
	atomic.AddInt64(&brm.metrics.activeRecoveries, 1)
}

func (brm *BatchProofRecoveryManager) cleanupSession(key string) {
	brm.mu.Lock()
	defer brm.mu.Unlock()
	
	if _, exists := brm.activeRecovery[key]; exists {
		delete(brm.activeRecovery, key)
		atomic.AddInt64(&brm.metrics.activeRecoveries, -1)
	}
}

// cleanupSessions periodically removes stale sessions
func (brm *BatchProofRecoveryManager) cleanupSessions() {
	defer brm.wg.Done()
	
	ticker := time.NewTicker(SessionCleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			brm.mu.Lock()
			now := time.Now()
			for key, session := range brm.activeRecovery {
				if now.Sub(session.LastUpdate) > SessionCleanupInterval {
					delete(brm.activeRecovery, key)
					atomic.AddInt64(&brm.metrics.activeRecoveries, -1)
				}
			}
			brm.mu.Unlock()
			
		case <-brm.ctx.Done():
			return
		}
	}
}

// Circuit breaker methods

func (brm *BatchProofRecoveryManager) isCircuitOpen(partitionID string) bool {
	brm.cbMu.RLock()
	defer brm.cbMu.RUnlock()
	
	cb, exists := brm.circuitBreaker[partitionID]
	if !exists {
		return false
	}
	
	state := atomic.LoadInt32(&cb.state)
	if state == 1 && time.Now().After(cb.backoffUntil) {
		// Try half-open state
		atomic.StoreInt32(&cb.state, 2)
		return false
	}
	
	return state == 1
}

func (brm *BatchProofRecoveryManager) handleRecoveryError(partitionID string, err error) {
	brm.cbMu.Lock()
	defer brm.cbMu.Unlock()
	
	cb, exists := brm.circuitBreaker[partitionID]
	if !exists {
		cb = &CircuitBreaker{}
		brm.circuitBreaker[partitionID] = cb
	}
	
	failures := atomic.AddInt32(&cb.failures, 1)
	cb.lastFailure = time.Now()
	
	// Open circuit after 3 failures
	if failures >= 3 {
		atomic.StoreInt32(&cb.state, 1)
		cb.backoffUntil = time.Now().Add(time.Duration(failures) * time.Second)
		brm.logger.Warn("Circuit breaker opened",
			"partition", partitionID,
			"failures", failures,
			"backoff_until", cb.backoffUntil)
	}
}

func (brm *BatchProofRecoveryManager) resetCircuitBreaker(partitionID string) {
	brm.cbMu.Lock()
	defer brm.cbMu.Unlock()
	
	if cb, exists := brm.circuitBreaker[partitionID]; exists {
		atomic.StoreInt32(&cb.failures, 0)
		atomic.StoreInt32(&cb.state, 0)
	}
}

// Metrics reporting

func (brm *BatchProofRecoveryManager) reportMetrics() {
	defer brm.wg.Done()
	
	ticker := time.NewTicker(brm.config.MetricsInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			brm.logger.Info("Batch proof metrics",
				"total_requests", atomic.LoadInt64(&brm.metrics.totalRequests),
				"batch_requests", atomic.LoadInt64(&brm.metrics.batchRequests),
				"individual_proofs", atomic.LoadInt64(&brm.metrics.individualProofs),
				"proof_savings", atomic.LoadInt64(&brm.metrics.proofSavings),
				"failed_requests", atomic.LoadInt64(&brm.metrics.failedRequests),
				"active_recoveries", atomic.LoadInt64(&brm.metrics.activeRecoveries))
				
		case <-brm.ctx.Done():
			return
		}
	}
}

// Helper functions (simplified for brevity)

func (brm *BatchProofRecoveryManager) generateCollectionProof(ctx context.Context, req *BatchRecoveryRequest, sequences []uint64) (*BatchRecoveryResponse, error) {
	// Implementation with context checking
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		// Actual implementation here
		return &BatchRecoveryResponse{
			PartitionID: req.PartitionID,
			Type:        req.Type,
			BatchSize:   len(sequences),
		}, nil
	}
}

func (brm *BatchProofRecoveryManager) processIndividualRecovery(ctx context.Context, req *BatchRecoveryRequest) {
	// Implementation
}

func (brm *BatchProofRecoveryManager) processIndividualRecoveryBatch(ctx context.Context, req *BatchRecoveryRequest, sequences []uint64) error {
	// Implementation
	return nil
}

func (brm *BatchProofRecoveryManager) optimizeBatches() {
	defer brm.wg.Done()
	// Implementation
}

// Additional type definitions remain the same...

type BatchRecoveryRequest struct {
	PartitionID      string
	Type             RecoveryType
	MissingSequences []uint64
	ChainURL         *url.URL
	RequestTime      time.Time
	Callback         func(*BatchRecoveryResponse)
}

type BatchRecoveryResponse struct {
	PartitionID       string
	Type              RecoveryType
	CollectionProof   *merkle.ReceiptList
	TransactionHashes [][]byte
	Transactions      []*RecoveredTransaction
	ProofGenerated    time.Time
	BatchSize         int
	ProofSavings      int
	Error             error
}

type RecoveredTransaction struct {
	Hash        []byte
	SequenceNum uint64
	Timestamp   time.Time
	Type        string
	Data        []byte
}

type BatchRecoverySession struct {
	PartitionID string
	Requests    []*BatchRecoveryRequest
	StartTime   time.Time
	LastUpdate  time.Time
	Status      BatchRecoveryStatus
}

type BatchRecoveryStatus int

const (
	BatchRecoveryPending BatchRecoveryStatus = iota
	BatchRecoveryGeneratingProof
	BatchRecoveryComplete
	BatchRecoveryFailed
)

type RecoveryType int

const (
	RecoveryTypeAnchor RecoveryType = iota
	RecoveryTypeSynthetic
)

func (rt RecoveryType) String() string {
	switch rt {
	case RecoveryTypeAnchor:
		return "anchor"
	case RecoveryTypeSynthetic:
		return "synthetic"
	default:
		return "unknown"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}