package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// PartitionHealthMonitor tracks the health status of partitions
type PartitionHealthMonitor struct {
	partitions map[string]*PartitionStatus
	mu         sync.RWMutex
	logger     logging.OptionalLogger

	// Configuration
	healthCheckInterval   time.Duration
	unhealthyThreshold    int // Number of failures before marking unhealthy
	recoveryCheckInterval time.Duration
	maxQueueSize          int // Maximum pending transactions per partition
}

// PartitionStatus tracks the status of a single partition
type PartitionStatus struct {
	ID               string
	State            PartitionState
	LastHealthCheck  time.Time
	LastSuccessful   time.Time
	ConsecutiveFails int32

	// Circuit breaker state
	CircuitState     CircuitState
	CircuitOpenTime  time.Time
	HalfOpenAttempts int32

	// Pending transactions while partition is down
	PendingQueue []*PendingTransaction
	QueueMu      sync.Mutex

	// Recovery state
	RecoveryInProgress bool
	LastSequenceAck    uint64 // Last acknowledged sequence number
	ExpectedSequence   uint64 // Next expected sequence number
}

// PartitionState represents the health state of a partition
type PartitionState int

const (
	PartitionHealthy PartitionState = iota
	PartitionDegraded
	PartitionUnhealthy
	PartitionDown
	PartitionRecovering
)

// CircuitState represents circuit breaker states
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Partition is down, rejecting requests
	CircuitHalfOpen                     // Testing if partition recovered
)

// PendingTransaction holds transactions that couldn't be delivered
type PendingTransaction struct {
	ID          string
	Type        MessageType
	Message     messaging.Message
	Destination *url.URL
	SequenceNum uint64
	Timestamp   time.Time
	RetryCount  int
	LastAttempt time.Time
}

// NewPartitionHealthMonitor creates a new health monitor
func NewPartitionHealthMonitor(logger logging.OptionalLogger) *PartitionHealthMonitor {
	return &PartitionHealthMonitor{
		partitions:            make(map[string]*PartitionStatus),
		logger:                logger,
		healthCheckInterval:   10 * time.Second,
		unhealthyThreshold:    3,
		recoveryCheckInterval: 30 * time.Second,
		maxQueueSize:          1000,
	}
}

// Start begins monitoring partitions
func (phm *PartitionHealthMonitor) Start(partitionIDs []string) {
	// Initialize partition status
	for _, id := range partitionIDs {
		phm.registerPartition(id)
	}

	// Start monitoring goroutines
	go phm.healthCheckLoop()
	go phm.recoveryCheckLoop()
	go phm.circuitBreakerManager()
}

// registerPartition initializes monitoring for a partition
func (phm *PartitionHealthMonitor) registerPartition(partitionID string) {
	phm.mu.Lock()
	defer phm.mu.Unlock()

	if _, exists := phm.partitions[partitionID]; !exists {
		phm.partitions[partitionID] = &PartitionStatus{
			ID:              partitionID,
			State:           PartitionHealthy,
			CircuitState:    CircuitClosed,
			LastHealthCheck: time.Now(),
			LastSuccessful:  time.Now(),
			PendingQueue:    make([]*PendingTransaction, 0),
		}
	}
}

// CanSendToPartition checks if we can send to a partition
func (phm *PartitionHealthMonitor) CanSendToPartition(partitionID string) (bool, error) {
	phm.mu.RLock()
	status, exists := phm.partitions[partitionID]
	phm.mu.RUnlock()

	if !exists {
		return false, errors.NotFound.WithFormat("partition %s not registered", partitionID)
	}

	// Check circuit breaker state
	switch status.CircuitState {
	case CircuitClosed:
		return true, nil

	case CircuitOpen:
		// Check if it's time to try half-open
		if time.Since(status.CircuitOpenTime) > 30*time.Second {
			phm.transitionToHalfOpen(partitionID)
			return true, nil // Allow one attempt
		}
		return false, errors.Unavailable.WithFormat("partition %s is down (circuit open)", partitionID)

	case CircuitHalfOpen:
		// Allow limited attempts in half-open state
		attempts := atomic.LoadInt32(&status.HalfOpenAttempts)
		if attempts < 3 {
			atomic.AddInt32(&status.HalfOpenAttempts, 1)
			return true, nil
		}
		return false, errors.Unavailable.WithFormat("partition %s is being tested (circuit half-open)", partitionID)
	}

	return false, errors.InternalError.With("unknown circuit state")
}

// RecordSuccess records a successful transaction to a partition
func (phm *PartitionHealthMonitor) RecordSuccess(partitionID string, sequenceNum uint64) {
	phm.mu.Lock()
	defer phm.mu.Unlock()

	status, exists := phm.partitions[partitionID]
	if !exists {
		return
	}

	status.LastSuccessful = time.Now()
	status.ConsecutiveFails = 0
	status.LastSequenceAck = sequenceNum

	// Update circuit breaker
	if status.CircuitState == CircuitHalfOpen {
		// Partition recovered, close circuit
		status.CircuitState = CircuitClosed
		status.State = PartitionHealthy
		phm.logger.Info("Partition recovered", "partition", partitionID)

		// Start draining pending queue
		go phm.drainPendingQueue(partitionID)
	}
}

// RecordFailure records a failed transaction to a partition
func (phm *PartitionHealthMonitor) RecordFailure(partitionID string, err error) {
	phm.mu.Lock()
	defer phm.mu.Unlock()

	status, exists := phm.partitions[partitionID]
	if !exists {
		return
	}

	atomic.AddInt32(&status.ConsecutiveFails, 1)
	fails := atomic.LoadInt32(&status.ConsecutiveFails)

	// Check if we should open the circuit
	if fails >= int32(phm.unhealthyThreshold) {
		if status.CircuitState != CircuitOpen {
			status.CircuitState = CircuitOpen
			status.CircuitOpenTime = time.Now()
			status.State = PartitionDown

			phm.logger.Warn("Partition marked as down",
				"partition", partitionID,
				"consecutive_failures", fails,
				"error", err)
		}
	} else if fails > 1 {
		status.State = PartitionDegraded
	}

	// If in half-open state and failed, go back to open
	if status.CircuitState == CircuitHalfOpen {
		status.CircuitState = CircuitOpen
		status.CircuitOpenTime = time.Now()
		status.HalfOpenAttempts = 0
		phm.logger.Info("Partition recovery failed, circuit reopened", "partition", partitionID)
	}
}

// QueueTransaction queues a transaction for a down partition
func (phm *PartitionHealthMonitor) QueueTransaction(partitionID string, tx *PendingTransaction) error {
	phm.mu.RLock()
	status, exists := phm.partitions[partitionID]
	phm.mu.RUnlock()

	if !exists {
		return errors.NotFound.WithFormat("partition %s not found", partitionID)
	}

	status.QueueMu.Lock()
	defer status.QueueMu.Unlock()

	// Check queue size limit
	if len(status.PendingQueue) >= phm.maxQueueSize {
		// Queue is full, we must drop the transaction
		phm.logger.Error("Pending queue full, dropping transaction",
			"partition", partitionID,
			"queue_size", len(status.PendingQueue),
			"transaction", tx.ID)
		return errors.Unavailable.With("partition queue is full")
	}

	status.PendingQueue = append(status.PendingQueue, tx)
	phm.logger.Info("Transaction queued for down partition",
		"partition", partitionID,
		"transaction", tx.ID,
		"queue_size", len(status.PendingQueue))

	return nil
}

// HandleOutOfOrderRequest handles when a partition requests missing transactions
func (phm *PartitionHealthMonitor) HandleOutOfOrderRequest(partitionID string, requestedSeq uint64) ([]*PendingTransaction, error) {
	phm.mu.RLock()
	status, exists := phm.partitions[partitionID]
	phm.mu.RUnlock()

	if !exists {
		return nil, errors.NotFound.WithFormat("partition %s not found", partitionID)
	}

	phm.logger.Info("Partition requesting out-of-order transactions",
		"partition", partitionID,
		"requested_sequence", requestedSeq,
		"last_ack", status.LastSequenceAck)

	// Mark partition as recovering
	status.State = PartitionRecovering
	status.RecoveryInProgress = true

	// Find transactions starting from requested sequence
	status.QueueMu.Lock()
	defer status.QueueMu.Unlock()

	var toSend []*PendingTransaction
	for _, tx := range status.PendingQueue {
		if tx.SequenceNum >= requestedSeq {
			toSend = append(toSend, tx)
		}
	}

	phm.logger.Info("Providing catch-up transactions",
		"partition", partitionID,
		"count", len(toSend),
		"from_sequence", requestedSeq)

	return toSend, nil
}

// healthCheckLoop periodically checks partition health
func (phm *PartitionHealthMonitor) healthCheckLoop() {
	ticker := time.NewTicker(phm.healthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		phm.mu.RLock()
		partitions := make([]*PartitionStatus, 0, len(phm.partitions))
		for _, status := range phm.partitions {
			partitions = append(partitions, status)
		}
		phm.mu.RUnlock()

		for _, status := range partitions {
			go phm.checkPartitionHealth(status)
		}
	}
}

// checkPartitionHealth performs a health check on a partition
func (phm *PartitionHealthMonitor) checkPartitionHealth(status *PartitionStatus) {
	// This would make an actual health check call to the partition
	// For now, we'll simulate based on state

	if status.State == PartitionDown {
		// Try to ping the partition
		if phm.pingPartition(status.ID) {
			// Partition might be recovering
			phm.transitionToHalfOpen(status.ID)
		}
	}

	status.LastHealthCheck = time.Now()
}

// pingPartition attempts to ping a partition (placeholder)
func (phm *PartitionHealthMonitor) pingPartition(partitionID string) bool {
	// In real implementation, this would make an actual network call
	// For simulation, we'll randomly recover partitions
	return time.Now().Unix()%10 == 0 // 10% chance of recovery
}

// transitionToHalfOpen transitions a partition to half-open state
func (phm *PartitionHealthMonitor) transitionToHalfOpen(partitionID string) {
	phm.mu.Lock()
	defer phm.mu.Unlock()

	status, exists := phm.partitions[partitionID]
	if !exists {
		return
	}

	if status.CircuitState == CircuitOpen {
		status.CircuitState = CircuitHalfOpen
		status.HalfOpenAttempts = 0
		phm.logger.Info("Testing partition recovery (circuit half-open)", "partition", partitionID)
	}
}

// recoveryCheckLoop checks for partitions that need recovery
func (phm *PartitionHealthMonitor) recoveryCheckLoop() {
	ticker := time.NewTicker(phm.recoveryCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		phm.mu.RLock()
		for partitionID, status := range phm.partitions {
			if status.State == PartitionRecovering && !status.RecoveryInProgress {
				go phm.attemptRecovery(partitionID)
			}
		}
		phm.mu.RUnlock()
	}
}

// attemptRecovery attempts to recover a partition
func (phm *PartitionHealthMonitor) attemptRecovery(partitionID string) {
	phm.mu.Lock()
	status := phm.partitions[partitionID]
	status.RecoveryInProgress = true
	phm.mu.Unlock()

	defer func() {
		phm.mu.Lock()
		status.RecoveryInProgress = false
		phm.mu.Unlock()
	}()

	phm.logger.Info("Attempting partition recovery", "partition", partitionID)

	// Try to send pending transactions
	err := phm.drainPendingQueue(partitionID)
	if err == nil {
		phm.mu.Lock()
		status.State = PartitionHealthy
		status.CircuitState = CircuitClosed
		phm.mu.Unlock()

		phm.logger.Info("Partition recovery successful", "partition", partitionID)
	} else {
		phm.logger.Warn("Partition recovery failed", "partition", partitionID, "error", err)
	}
}

// drainPendingQueue attempts to send all pending transactions
func (phm *PartitionHealthMonitor) drainPendingQueue(partitionID string) error {
	phm.mu.RLock()
	status, exists := phm.partitions[partitionID]
	phm.mu.RUnlock()

	if !exists {
		return errors.NotFound.WithFormat("partition %s not found", partitionID)
	}

	status.QueueMu.Lock()
	queue := status.PendingQueue
	status.PendingQueue = make([]*PendingTransaction, 0)
	status.QueueMu.Unlock()

	phm.logger.Info("Draining pending queue",
		"partition", partitionID,
		"queue_size", len(queue))

	// Sort by sequence number to maintain order
	// In production, implement proper sorting

	successCount := 0
	failCount := 0

	for _, tx := range queue {
		// Attempt to send transaction
		// This would use the actual dispatcher
		err := phm.sendPendingTransaction(tx)
		if err != nil {
			failCount++
			// Re-queue failed transaction
			status.QueueMu.Lock()
			status.PendingQueue = append(status.PendingQueue, tx)
			status.QueueMu.Unlock()
		} else {
			successCount++
		}
	}

	phm.logger.Info("Queue drain complete",
		"partition", partitionID,
		"success", successCount,
		"failed", failCount)

	if failCount > 0 {
		return errors.InternalError.WithFormat("failed to send %d transactions", failCount)
	}

	return nil
}

// sendPendingTransaction sends a pending transaction (placeholder)
func (phm *PartitionHealthMonitor) sendPendingTransaction(tx *PendingTransaction) error {
	// In real implementation, this would use the dispatcher
	// For now, simulate success
	return nil
}

// circuitBreakerManager manages circuit breaker states
func (phm *PartitionHealthMonitor) circuitBreakerManager() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		phm.mu.RLock()
		for partitionID, status := range phm.partitions {
			if status.CircuitState == CircuitOpen {
				// Check if it's time to try half-open
				if time.Since(status.CircuitOpenTime) > 30*time.Second {
					phm.transitionToHalfOpen(partitionID)
				}
			}
		}
		phm.mu.RUnlock()
	}
}

// GetPartitionStatus returns the current status of a partition
func (phm *PartitionHealthMonitor) GetPartitionStatus(partitionID string) (*PartitionStatus, error) {
	phm.mu.RLock()
	defer phm.mu.RUnlock()

	status, exists := phm.partitions[partitionID]
	if !exists {
		return nil, errors.NotFound.WithFormat("partition %s not found", partitionID)
	}

	return status, nil
}

// GetAllPartitionStatuses returns status of all partitions
func (phm *PartitionHealthMonitor) GetAllPartitionStatuses() map[string]*PartitionStatus {
	phm.mu.RLock()
	defer phm.mu.RUnlock()

	result := make(map[string]*PartitionStatus)
	for k, v := range phm.partitions {
		result[k] = v
	}

	return result
}

// String representations for debugging
func (ps PartitionState) String() string {
	switch ps {
	case PartitionHealthy:
		return "Healthy"
	case PartitionDegraded:
		return "Degraded"
	case PartitionUnhealthy:
		return "Unhealthy"
	case PartitionDown:
		return "Down"
	case PartitionRecovering:
		return "Recovering"
	default:
		return "Unknown"
	}
}

func (cs CircuitState) String() string {
	switch cs {
	case CircuitClosed:
		return "Closed"
	case CircuitOpen:
		return "Open"
	case CircuitHalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}
