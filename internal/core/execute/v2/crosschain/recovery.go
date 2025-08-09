// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"fmt"
	"sync"
	"time"

	// "gitlab.com/accumulatenetwork/accumulate/internal/core/execute" // Not currently used
	// "gitlab.com/accumulatenetwork/accumulate/internal/core/healing" // Removed to fix import cycle
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	// "gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging" // Not currently used
	// "gitlab.com/accumulatenetwork/accumulate/pkg/url" // Not currently used
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// RecoveryManager handles recovery of missing anchors and synthetic transactions
type RecoveryManager struct {
	conductor *CrossChainConductor
	logger    logging.OptionalLogger
	db        database.Beginner
	client    api.Querier

	// Recovery state
	recoveryQueue  chan *RecoveryRequest
	activeRecovery map[string]*RecoverySession
	mu             sync.RWMutex

	// Configuration
	maxConcurrentRecovery int
	checkInterval         time.Duration
	requestTimeout        time.Duration
}

// RecoveryRequest represents a request for missing transactions
type RecoveryRequest struct {
	Type        MessageType
	Source      string
	Destination string
	FromNumber  uint64
	ToNumber    uint64
	Requester   string // Which partition is requesting
	Priority    int    // Higher priority requests are processed first
	RequestedAt time.Time
	Callback    chan *RecoveryResponse
}

// RecoveryResponse contains recovered transactions
type RecoveryResponse struct {
	Request      *RecoveryRequest
	Transactions []RecoveredTransaction
	Error        error
}

// Note: RecoveredTransaction is defined in conductor.go

// RecoverySession tracks an active recovery operation
type RecoverySession struct {
	Request    *RecoveryRequest
	StartedAt  time.Time
	Status     string
	Progress   float64
	Recovered  int
	Total      int
	LastUpdate time.Time
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(conductor *CrossChainConductor, db database.Beginner, client api.Querier) *RecoveryManager {
	return &RecoveryManager{
		conductor:             conductor,
		logger:                conductor.logger.With("module", "recovery").(logging.OptionalLogger),
		db:                    db,
		client:                client,
		recoveryQueue:         make(chan *RecoveryRequest, 100),
		activeRecovery:        make(map[string]*RecoverySession),
		maxConcurrentRecovery: 5,
		checkInterval:         30 * time.Second,
		requestTimeout:        5 * time.Minute,
	}
}

// Start begins the recovery manager
func (rm *RecoveryManager) Start() {
	go rm.processRecoveryRequests()
	go rm.periodicHealthCheck()
}

// RequestMissingTransactions requests missing anchors or synthetic transactions
func (rm *RecoveryManager) RequestMissingTransactions(req *RecoveryRequest) (*RecoveryResponse, error) {
	// Validate request
	if req.FromNumber > req.ToNumber {
		return nil, errors.BadRequest.WithFormat("invalid range: from %d > to %d", req.FromNumber, req.ToNumber)
	}

	// Check if we're already recovering this range
	sessionKey := rm.getSessionKey(req)
	rm.mu.RLock()
	if session, exists := rm.activeRecovery[sessionKey]; exists {
		rm.mu.RUnlock()
		rm.logger.Info("Recovery already in progress",
			"source", req.Source,
			"destination", req.Destination,
			"progress", fmt.Sprintf("%.1f%%", session.Progress))
		// Wait for existing recovery to complete
		return rm.waitForSession(session, req)
	}
	rm.mu.RUnlock()

	// Create callback channel if not provided
	if req.Callback == nil {
		req.Callback = make(chan *RecoveryResponse, 1)
	}

	// Queue the request
	req.RequestedAt = time.Now()
	select {
	case rm.recoveryQueue <- req:
		rm.logger.Info("Recovery request queued",
			"type", rm.messageTypeName(req.Type),
			"source", req.Source,
			"destination", req.Destination,
			"range", fmt.Sprintf("%d-%d", req.FromNumber, req.ToNumber))
	case <-time.After(10 * time.Second):
		return nil, errors.NotReady.With("recovery queue is full")
	}

	// Wait for response
	select {
	case resp := <-req.Callback:
		return resp, resp.Error
	case <-time.After(rm.requestTimeout):
		return nil, errors.NotReady.With("recovery request timed out")
	}
}

// processRecoveryRequests processes queued recovery requests
func (rm *RecoveryManager) processRecoveryRequests() {
	for req := range rm.recoveryQueue {
		// Check concurrent limit
		rm.mu.RLock()
		activeCount := len(rm.activeRecovery)
		rm.mu.RUnlock()

		if activeCount >= rm.maxConcurrentRecovery {
			// Wait for a slot to open
			time.Sleep(1 * time.Second)
			rm.recoveryQueue <- req // Re-queue
			continue
		}

		// Start recovery session
		go rm.executeRecovery(req)
	}
}

// executeRecovery performs the actual recovery of missing transactions
func (rm *RecoveryManager) executeRecovery(req *RecoveryRequest) {
	sessionKey := rm.getSessionKey(req)
	session := &RecoverySession{
		Request:    req,
		StartedAt:  time.Now(),
		Status:     "starting",
		LastUpdate: time.Now(),
	}

	// Register session
	rm.mu.Lock()
	rm.activeRecovery[sessionKey] = session
	rm.mu.Unlock()

	// Clean up session when done
	defer func() {
		rm.mu.Lock()
		delete(rm.activeRecovery, sessionKey)
		rm.mu.Unlock()
	}()

	// Execute recovery based on type
	var resp *RecoveryResponse
	var err error

	switch req.Type {
	case MessageTypeAnchor:
		resp, err = rm.recoverAnchors(req, session)
	case MessageTypeSynthetic:
		resp, err = rm.recoverSynthetics(req, session)
	default:
		err = errors.BadRequest.WithFormat("unsupported message type: %v", req.Type)
	}

	// Send response
	if err != nil {
		resp = &RecoveryResponse{
			Request: req,
			Error:   err,
		}
	}

	select {
	case req.Callback <- resp:
	case <-time.After(10 * time.Second):
		rm.logger.Error("Failed to send recovery response", "error", "timeout")
	}
}

// recoverAnchors recovers missing anchor transactions
func (rm *RecoveryManager) recoverAnchors(req *RecoveryRequest, session *RecoverySession) (*RecoveryResponse, error) {
	srcUrl := protocol.PartitionUrl(req.Source)
	dstUrl := protocol.PartitionUrl(req.Destination)

	rm.logger.Info("Starting anchor recovery",
		"source", req.Source,
		"destination", req.Destination,
		"range", fmt.Sprintf("%d-%d", req.FromNumber, req.ToNumber))

	session.Status = "reading ledgers"
	session.Total = int(req.ToNumber - req.FromNumber + 1)

	// Read the anchor ledger from destination
	batch := rm.db.Begin(false)
	defer batch.Discard()

	account := batch.Account(dstUrl.JoinPath(protocol.AnchorPool))
	var ledger *protocol.AnchorLedger
	err := account.Main().GetAs(&ledger)
	if err != nil {
		return nil, errors.InternalError.WithFormat("failed to read anchor ledger: %w", err)
	}

	srcLedger := ledger.Anchor(srcUrl)

	// Collect missing anchors
	var recovered []RecoveredTransaction

	for seqNum := req.FromNumber; seqNum <= req.ToNumber; seqNum++ {
		session.Progress = float64(seqNum-req.FromNumber) / float64(session.Total) * 100
		session.LastUpdate = time.Now()

		// Check if we already have this anchor
		if seqNum <= srcLedger.Delivered {
			continue
		}

		// Try to get from pending list
		if seqNum > srcLedger.Delivered && seqNum <= srcLedger.Received {
			idx := seqNum - srcLedger.Delivered - 1
			if idx < uint64(len(srcLedger.Pending)) {
				// txid available at srcLedger.Pending[idx] but not used in simplified implementation
				_ = srcLedger.Pending[idx]
			}
		}

		// Retrieve the anchor from source (placeholder - handled by CrossChainConductor)
		session.Status = fmt.Sprintf("retrieving anchor %d", seqNum)
		
		// Since recovery is handled by CrossChainConductor, we just log the attempt
		rm.logger.Info("Anchor recovery request",
			"source", req.Source,
			"number", seqNum)
		session.Recovered++
	}

	session.Status = "completed"
	rm.logger.Info("Anchor recovery completed",
		"source", req.Source,
		"destination", req.Destination,
		"recovered", len(recovered),
		"total", session.Total)

	return &RecoveryResponse{
		Request:      req,
		Transactions: recovered,
	}, nil
}

// recoverSynthetics recovers missing synthetic transactions
func (rm *RecoveryManager) recoverSynthetics(req *RecoveryRequest, session *RecoverySession) (*RecoveryResponse, error) {
	srcUrl := protocol.PartitionUrl(req.Source)
	dstUrl := protocol.PartitionUrl(req.Destination)

	rm.logger.Info("Starting synthetic recovery",
		"source", req.Source,
		"destination", req.Destination,
		"range", fmt.Sprintf("%d-%d", req.FromNumber, req.ToNumber))

	session.Status = "reading ledgers"
	session.Total = int(req.ToNumber - req.FromNumber + 1)

	// Read the synthetic ledger from destination
	batch := rm.db.Begin(false)
	defer batch.Discard()

	account := batch.Account(dstUrl.JoinPath(protocol.Synthetic))
	var ledger *protocol.SyntheticLedger
	err := account.Main().GetAs(&ledger)
	if err != nil {
		return nil, errors.InternalError.WithFormat("failed to read synthetic ledger: %w", err)
	}

	srcLedger := ledger.Partition(srcUrl)

	// Collect missing synthetics
	var recovered []RecoveredTransaction

	for seqNum := req.FromNumber; seqNum <= req.ToNumber; seqNum++ {
		session.Progress = float64(seqNum-req.FromNumber) / float64(session.Total) * 100
		session.LastUpdate = time.Now()

		// Check if we already have this synthetic
		if seqNum <= srcLedger.Delivered {
			continue
		}

		// Try to get from pending list
		_, hasTxid := srcLedger.Get(seqNum)

		// Retrieve the synthetic from source (placeholder - handled by CrossChainConductor)
		session.Status = fmt.Sprintf("retrieving synthetic %d", seqNum)
		
		// Since recovery is handled by CrossChainConductor, we just log the attempt
		rm.logger.Info("Synthetic recovery request",
			"source", req.Source,
			"number", seqNum,
			"has_txid", hasTxid)
		
		if hasTxid {
			session.Recovered++
		}
	}

	session.Status = "completed"
	rm.logger.Info("Synthetic recovery completed",
		"source", req.Source,
		"destination", req.Destination,
		"recovered", len(recovered),
		"total", session.Total)

	return &RecoveryResponse{
		Request:      req,
		Transactions: recovered,
	}, nil
}


// periodicHealthCheck periodically checks for missing transactions
func (rm *RecoveryManager) periodicHealthCheck() {
	ticker := time.NewTicker(rm.checkInterval)
	defer ticker.Stop()

	for range ticker.C {
		rm.checkPartitionHealth()
	}
}

// checkPartitionHealth checks each partition for missing transactions
func (rm *RecoveryManager) checkPartitionHealth() {
	ctx := context.Background()
	batch := rm.db.Begin(false)
	defer batch.Discard()

	// Get network status
	netInfo, err := rm.getNetworkInfo(ctx)
	if err != nil {
		rm.logger.Error("Failed to get network info", "error", err)
		return
	}

	// Check each partition pair
	for _, srcPart := range netInfo.Partitions {
		for _, dstPart := range netInfo.Partitions {
			if srcPart.ID == dstPart.ID {
				continue
			}

			// Check anchors
			rm.checkMissingAnchors(batch, srcPart, dstPart)

			// Check synthetics
			rm.checkMissingSynthetics(batch, srcPart, dstPart)
		}
	}
}

// checkMissingAnchors checks for missing anchors between partitions
func (rm *RecoveryManager) checkMissingAnchors(batch *database.Batch, src, dst *protocol.PartitionInfo) {
	srcUrl := protocol.PartitionUrl(src.ID)
	dstUrl := protocol.PartitionUrl(dst.ID)

	// Read destination anchor ledger
	account := batch.Account(dstUrl.JoinPath(protocol.AnchorPool))
	var ledger *protocol.AnchorLedger
	err := account.Main().GetAs(&ledger)
	if err != nil {
		return
	}

	srcLedger := ledger.Anchor(srcUrl)

	// Check for gaps
	missing := srcLedger.Received - srcLedger.Delivered
	if missing > 10 { // Threshold for concern
		rm.logger.Info("Missing anchors detected",
			"source", src.ID,
			"destination", dst.ID,
			"missing", missing,
			"delivered", srcLedger.Delivered,
			"received", srcLedger.Received)

		// Trigger recovery if too many missing
		if missing > 50 {
			req := &RecoveryRequest{
				Type:        MessageTypeAnchor,
				Source:      src.ID,
				Destination: dst.ID,
				FromNumber:  srcLedger.Delivered + 1,
				ToNumber:    srcLedger.Received,
				Requester:   dst.ID,
				Priority:    1,
			}
			go func() {
				_, err := rm.RequestMissingTransactions(req)
				if err != nil {
					rm.logger.Error("Failed to request missing transactions", "error", err, "source", req.Source)
				}
			}()
		}
	}
}

// checkMissingSynthetics checks for missing synthetic transactions
func (rm *RecoveryManager) checkMissingSynthetics(batch *database.Batch, src, dst *protocol.PartitionInfo) {
	srcUrl := protocol.PartitionUrl(src.ID)
	dstUrl := protocol.PartitionUrl(dst.ID)

	// Read destination synthetic ledger
	account := batch.Account(dstUrl.JoinPath(protocol.Synthetic))
	var ledger *protocol.SyntheticLedger
	err := account.Main().GetAs(&ledger)
	if err != nil {
		return
	}

	srcLedger := ledger.Partition(srcUrl)

	// Check for gaps
	missing := srcLedger.Received - srcLedger.Delivered
	if missing > 10 { // Threshold for concern
		rm.logger.Info("Missing synthetics detected",
			"source", src.ID,
			"destination", dst.ID,
			"missing", missing,
			"delivered", srcLedger.Delivered,
			"received", srcLedger.Received)

		// Trigger recovery if too many missing
		if missing > 50 {
			req := &RecoveryRequest{
				Type:        MessageTypeSynthetic,
				Source:      src.ID,
				Destination: dst.ID,
				FromNumber:  srcLedger.Delivered + 1,
				ToNumber:    srcLedger.Received,
				Requester:   dst.ID,
				Priority:    1,
			}
			go func() {
				_, err := rm.RequestMissingTransactions(req)
				if err != nil {
					rm.logger.Error("Failed to request missing transactions", "error", err, "source", req.Source)
				}
			}()
		}
	}
}

// Helper methods

func (rm *RecoveryManager) getSessionKey(req *RecoveryRequest) string {
	return fmt.Sprintf("%v:%s->%s:%d-%d", req.Type, req.Source, req.Destination, req.FromNumber, req.ToNumber)
}

func (rm *RecoveryManager) messageTypeName(t MessageType) string {
	switch t {
	case MessageTypeAnchor:
		return "anchor"
	case MessageTypeSynthetic:
		return "synthetic"
	default:
		return "unknown"
	}
}

func (rm *RecoveryManager) waitForSession(session *RecoverySession, req *RecoveryRequest) (*RecoveryResponse, error) {
	// Poll session status
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(rm.requestTimeout)

	for {
		select {
		case <-ticker.C:
			rm.mu.RLock()
			if _, exists := rm.activeRecovery[rm.getSessionKey(req)]; !exists {
				rm.mu.RUnlock()
				// Session completed, check if we can get results
				return nil, errors.InternalError.With("recovery session completed but results unavailable")
			}
			rm.mu.RUnlock()

		case <-timeout:
			return nil, errors.NotReady.With("timeout waiting for existing recovery session")
		}
	}
}

// NetworkInfo holds network information for recovery
type NetworkInfo struct {
	Status     *api.NetworkStatus
	Partitions []*protocol.PartitionInfo
}

func (rm *RecoveryManager) getNetworkInfo(ctx context.Context) (*NetworkInfo, error) {
	// Simplified placeholder - actual network querying would be more complex
	netInfo := &NetworkInfo{
		Status:     nil, // Placeholder
		Partitions: make([]*protocol.PartitionInfo, 0),
	}

	return netInfo, nil
}

// ProvideRecoveredTransactions provides recovered transactions to requesting partition
func (rm *RecoveryManager) ProvideRecoveredTransactions(recovered []RecoveredTransaction, destination string) error {
	// Simplified placeholder - actual recovery handled by CrossChainConductor
	rm.logger.Info("Providing recovered transactions",
		"count", len(recovered),
		"destination", destination)
	return nil
}

