package results

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDatabase(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Test config
	config := &Config{
		TransactionsPerSecond: 100,
		Duration:              5 * time.Second,
		NumClients:            10,
		TransactionTypes:      []string{"SendTokens", "CreateAccount"},
		NetworkTopology:       "3-partition",
		NumValidators:         9,
		NumPartitions:         3,
	}

	configHash, err := db.SaveConfig(config)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Test starting a run
	runID, err := db.StartTestRun("abc123", "main", configHash, "Test run")
	if err != nil {
		t.Fatalf("failed to start test run: %v", err)
	}

	// Test saving metrics
	metrics := []*MetricRecord{
		{
			RunID:      runID,
			Timestamp:  time.Now(),
			MetricName: "tps",
			Value:      95.5,
			Unit:       "transactions/second",
		},
		{
			RunID:      runID,
			Timestamp:  time.Now().Add(1 * time.Second),
			MetricName: "tps",
			Value:      98.2,
			Unit:       "transactions/second",
		},
		{
			RunID:      runID,
			Timestamp:  time.Now().Add(2 * time.Second),
			MetricName: "latency",
			Value:      150.5,
			Unit:       "ms",
			Labels:     map[string]string{"percentile": "p50"},
		},
	}

	if err := db.SaveMetricBatch(metrics); err != nil {
		t.Fatalf("failed to save metrics: %v", err)
	}

	// Test saving errors
	errRecord := &ErrorRecord{
		RunID:         runID,
		Timestamp:     time.Now(),
		OperationType: "SendTokens",
		ErrorMessage:  "insufficient balance",
		ErrorDetails:  map[string]interface{}{"account": "acc://test"},
	}

	if err := db.SaveError(errRecord); err != nil {
		t.Fatalf("failed to save error: %v", err)
	}

	// Test saving node health
	health := &NodeHealthRecord{
		RunID:       runID,
		Timestamp:   time.Now(),
		NodeID:      "validator-0",
		PartitionID: "BVN0",
		Status:      "healthy",
		Resources: map[string]interface{}{
			"cpu_percent":    25.5,
			"memory_percent": 45.2,
			"disk_gb":        100.5,
		},
	}

	if err := db.SaveNodeHealth(health); err != nil {
		t.Fatalf("failed to save node health: %v", err)
	}

	// Test updating run
	if err := db.UpdateTestRun(runID, "completed", 1000, 5); err != nil {
		t.Fatalf("failed to update test run: %v", err)
	}

	// Test retrieving run
	run, err := db.GetTestRun(runID)
	if err != nil {
		t.Fatalf("failed to get test run: %v", err)
	}

	if run.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", run.Status)
	}
	if run.TotalTransactions != 1000 {
		t.Errorf("expected 1000 transactions, got %d", run.TotalTransactions)
	}

	// Test metric stats
	stats, err := db.GetMetricStats(runID, "tps")
	if err != nil {
		t.Fatalf("failed to get metric stats: %v", err)
	}

	if stats.Count != 2 {
		t.Errorf("expected 2 metrics, got %d", stats.Count)
	}

	// Test error summary
	errSummary, err := db.GetErrorSummary(runID)
	if err != nil {
		t.Fatalf("failed to get error summary: %v", err)
	}

	if count, ok := errSummary["SendTokens"]; !ok || count != 1 {
		t.Errorf("expected 1 SendTokens error, got %d", count)
	}

	// Test node health summary
	healthSummary, err := db.GetNodeHealthSummary(runID)
	if err != nil {
		t.Fatalf("failed to get node health summary: %v", err)
	}

	if _, ok := healthSummary["validator-0"]; !ok {
		t.Error("expected validator-0 in health summary")
	}

	// Test listing runs
	runs, err := db.ListTestRuns("", "", nil, nil, 10)
	if err != nil {
		t.Fatalf("failed to list test runs: %v", err)
	}

	if len(runs) != 1 {
		t.Errorf("expected 1 run, got %d", len(runs))
	}
}

func TestConfigHash(t *testing.T) {
	config1 := &Config{
		TransactionsPerSecond: 100,
		Duration:              5 * time.Second,
	}

	config2 := &Config{
		TransactionsPerSecond: 100,
		Duration:              5 * time.Second,
	}

	hash1, err := ComputeConfigHash(config1)
	if err != nil {
		t.Fatalf("failed to compute hash1: %v", err)
	}

	hash2, err := ComputeConfigHash(config2)
	if err != nil {
		t.Fatalf("failed to compute hash2: %v", err)
	}

	if hash1 != hash2 {
		t.Error("identical configs should have same hash")
	}

	config3 := &Config{
		TransactionsPerSecond: 200,
		Duration:              5 * time.Second,
	}

	hash3, err := ComputeConfigHash(config3)
	if err != nil {
		t.Fatalf("failed to compute hash3: %v", err)
	}

	if hash1 == hash3 {
		t.Error("different configs should have different hash")
	}
}

func TestDeleteOldRuns(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	config := &Config{
		TransactionsPerSecond: 100,
	}
	configHash, _ := db.SaveConfig(config)

	// Create an old run (manually set start_time)
	db.db.Exec(`
		INSERT INTO test_runs (commit_hash, branch, config_hash, start_time, status)
		VALUES (?, ?, ?, ?, ?)
	`, "old123", "main", configHash, time.Now().AddDate(0, 0, -31), "completed")

	// Create a recent run
	_, _ = db.StartTestRun("new123", "main", configHash, "Recent run")

	// Delete runs older than 30 days
	deleted, err := db.DeleteOldRuns(30)
	if err != nil {
		t.Fatalf("failed to delete old runs: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted run, got %d", deleted)
	}

	// Check that recent run still exists
	runs, _ := db.ListTestRuns("", "", nil, nil, 10)
	if len(runs) != 1 {
		t.Errorf("expected 1 remaining run, got %d", len(runs))
	}
}
