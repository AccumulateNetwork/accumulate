package results

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"
)

// Monitor collects and records node health data
type Monitor struct {
	db        *Database
	runID     int64
	interval  time.Duration
	stopChan  chan struct{}
	wg        sync.WaitGroup
	nodeID    string
	partition string
}

// NodeHealthCollector is an interface for collecting node health data
type NodeHealthCollector interface {
	GetNodeID() string
	GetPartitionID() string
	GetStatus() string
	GetResources() map[string]interface{}
}

// NewMonitor creates a new health monitor
func NewMonitor(db *Database, runID int64, nodeID, partition string, interval time.Duration) *Monitor {
	return &Monitor{
		db:        db,
		runID:     runID,
		interval:  interval,
		stopChan:  make(chan struct{}),
		nodeID:    nodeID,
		partition: partition,
	}
}

// Start begins monitoring and recording health data
func (m *Monitor) Start(collector NodeHealthCollector) {
	m.wg.Add(1)
	go m.monitorLoop(collector)
}

// Stop stops the monitoring
func (m *Monitor) Stop() {
	close(m.stopChan)
	m.wg.Wait()
}

// monitorLoop is the main monitoring loop
func (m *Monitor) monitorLoop(collector NodeHealthCollector) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.recordHealth(collector); err != nil {
				log.Printf("Error recording health data: %v", err)
			}
		case <-m.stopChan:
			return
		}
	}
}

// recordHealth records current health status
func (m *Monitor) recordHealth(collector NodeHealthCollector) error {
	health := &NodeHealthRecord{
		RunID:       m.runID,
		Timestamp:   time.Now(),
		NodeID:      collector.GetNodeID(),
		PartitionID: collector.GetPartitionID(),
		Status:      collector.GetStatus(),
		Resources:   collector.GetResources(),
	}

	return m.db.SaveNodeHealth(health)
}

// RecordHealthSnapshot records a single health snapshot
func (m *Monitor) RecordHealthSnapshot(nodeID, partitionID, status string, resources map[string]interface{}) error {
	health := &NodeHealthRecord{
		RunID:       m.runID,
		Timestamp:   time.Now(),
		NodeID:      nodeID,
		PartitionID: partitionID,
		Status:      status,
		Resources:   resources,
	}

	return m.db.SaveNodeHealth(health)
}

// GetRuntimeResources collects runtime resource information
func GetRuntimeResources() map[string]interface{} {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return map[string]interface{}{
		"goroutines":     runtime.NumGoroutine(),
		"alloc_mb":       float64(mem.Alloc) / 1024 / 1024,
		"total_alloc_mb": float64(mem.TotalAlloc) / 1024 / 1024,
		"sys_mb":         float64(mem.Sys) / 1024 / 1024,
		"num_gc":         mem.NumGC,
		"heap_objects":   mem.HeapObjects,
	}
}

// SimpleCollector is a basic implementation of NodeHealthCollector
type SimpleCollector struct {
	nodeID      string
	partitionID string
	statusFunc  func() string
}

// NewSimpleCollector creates a simple health collector
func NewSimpleCollector(nodeID, partitionID string, statusFunc func() string) *SimpleCollector {
	return &SimpleCollector{
		nodeID:      nodeID,
		partitionID: partitionID,
		statusFunc:  statusFunc,
	}
}

func (c *SimpleCollector) GetNodeID() string {
	return c.nodeID
}

func (c *SimpleCollector) GetPartitionID() string {
	return c.partitionID
}

func (c *SimpleCollector) GetStatus() string {
	if c.statusFunc != nil {
		return c.statusFunc()
	}
	return "healthy"
}

func (c *SimpleCollector) GetResources() map[string]interface{} {
	return GetRuntimeResources()
}

// BatchHealthRecorder records health data for multiple nodes efficiently
type BatchHealthRecorder struct {
	db      *Database
	runID   int64
	records []*NodeHealthRecord
	mu      sync.Mutex
}

// NewBatchHealthRecorder creates a new batch recorder
func NewBatchHealthRecorder(db *Database, runID int64) *BatchHealthRecorder {
	return &BatchHealthRecorder{
		db:      db,
		runID:   runID,
		records: make([]*NodeHealthRecord, 0),
	}
}

// Add adds a health record to the batch
func (b *BatchHealthRecorder) Add(nodeID, partitionID, status string, resources map[string]interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.records = append(b.records, &NodeHealthRecord{
		RunID:       b.runID,
		Timestamp:   time.Now(),
		NodeID:      nodeID,
		PartitionID: partitionID,
		Status:      status,
		Resources:   resources,
	})
}

// Flush writes all batched records to the database
func (b *BatchHealthRecorder) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.records) == 0 {
		return nil
	}

	tx, err := b.db.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, record := range b.records {
		if err := b.db.SaveNodeHealth(record); err != nil {
			return fmt.Errorf("failed to save health record: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	b.records = b.records[:0]
	return nil
}
