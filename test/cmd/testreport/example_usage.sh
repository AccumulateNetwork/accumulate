#!/bin/bash
# Example usage of the testreport tool

set -e

# Build the tool
echo "Building testreport tool..."
go build -o testreport

# Create sample database and add test data
echo "Creating sample test data..."

DB_PATH="/tmp/test-reports/example.db"
rm -f "$DB_PATH"

# Create a simple Go program to populate sample data
cat > /tmp/populate_test_data.go << 'EOF'
package main

import (
	"fmt"
	"time"
	"gitlab.com/accumulatenetwork/accumulate/test/testresults"
)

func main() {
	db, err := testresults.NewDatabase("/tmp/test-reports/example.db")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Insert baseline run
	baseRun := &testresults.TestRun{
		CommitHash:    "abc123def456",
		Branch:        "main",
		StartTime:     time.Now().Add(-48 * time.Hour),
		EndTime:       time.Now().Add(-48 * time.Hour).Add(5 * time.Minute),
		Duration:      300,
		TargetTPS:     100,
		Concurrency:   25,
		NetworkConfig: `{"nodes": 3, "validators": 9}`,
		TestConfig:    `{"type": "load", "operation": "faucet"}`,
		TotalTx:       30000,
		TxPassed:      29800,
		TxFailed:      200,
		AverageTPS:    99.3,
		PeakTPS:       125.0,
		MinLatency:    15.0,
		MaxLatency:    450.0,
		AvgLatency:    52.3,
		P50Latency:    48.0,
		P95Latency:    105.0,
		P99Latency:    180.0,
		ErrorRate:     0.67,
		NodeCrashes:   0,
		NodeRestarts:  0,
		Notes:         "Baseline performance test",
		ResultsPath:   "/tmp/load_tester/run1",
	}

	id1, err := db.InsertTestRun(baseRun)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Inserted baseline run with ID: %d\n", id1)

	// Insert comparison run (with regression)
	compareRun := &testresults.TestRun{
		CommitHash:    "def456abc789",
		Branch:        "feature/new-consensus",
		StartTime:     time.Now().Add(-24 * time.Hour),
		EndTime:       time.Now().Add(-24 * time.Hour).Add(5 * time.Minute),
		Duration:      300,
		TargetTPS:     100,
		Concurrency:   25,
		NetworkConfig: `{"nodes": 3, "validators": 9}`,
		TestConfig:    `{"type": "load", "operation": "faucet"}`,
		TotalTx:       30000,
		TxPassed:      29200,
		TxFailed:      800,
		AverageTPS:    87.5,  // Regression: 11.9% decrease
		PeakTPS:       110.0,
		MinLatency:    18.0,
		MaxLatency:    650.0,
		AvgLatency:    62.8,  // Regression: 20% increase
		P50Latency:    56.0,
		P95Latency:    135.0, // Regression: 28.6% increase
		P99Latency:    250.0,
		ErrorRate:     2.67,  // Regression: error rate increased
		NodeCrashes:   1,     // Regression: node crash
		NodeRestarts:  2,
		Notes:         "Testing new consensus algorithm - performance degraded",
		ResultsPath:   "/tmp/load_tester/run2",
	}

	id2, err := db.InsertTestRun(compareRun)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Inserted comparison run with ID: %d\n", id2)

	// Insert trend data
	for i := 0; i < 5; i++ {
		trendRun := &testresults.TestRun{
			CommitHash:    fmt.Sprintf("trend%d", i),
			Branch:        "main",
			StartTime:     time.Now().Add(-time.Duration(5-i) * 24 * time.Hour),
			EndTime:       time.Now().Add(-time.Duration(5-i) * 24 * time.Hour).Add(5 * time.Minute),
			Duration:      300,
			TargetTPS:     100,
			Concurrency:   25,
			NetworkConfig: `{"nodes": 3}`,
			TestConfig:    `{"type": "load"}`,
			TotalTx:       30000,
			TxPassed:      29500 + i*100,
			TxFailed:      500 - i*100,
			AverageTPS:    95.0 + float64(i)*1.5,
			PeakTPS:       120.0 + float64(i)*2.0,
			MinLatency:    15.0,
			MaxLatency:    400.0 - float64(i)*10.0,
			AvgLatency:    55.0 - float64(i)*1.0,
			P50Latency:    50.0 - float64(i)*1.0,
			P95Latency:    110.0 - float64(i)*2.0,
			P99Latency:    190.0 - float64(i)*3.0,
			ErrorRate:     1.67 - float64(i)*0.3,
			NodeCrashes:   0,
			NodeRestarts:  0,
			Notes:         fmt.Sprintf("Trend analysis run %d", i),
			ResultsPath:   fmt.Sprintf("/tmp/load_tester/trend%d", i),
		}

		id, err := db.InsertTestRun(trendRun)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Inserted trend run %d with ID: %d\n", i, id)
	}

	fmt.Println("\nSample data populated successfully!")
}
EOF

# Run the data population script
cd /tmp/accumulate-worktrees/issue-3847
go run /tmp/populate_test_data.go

# Return to testreport directory
cd test/cmd/testreport

echo ""
echo "=== Example 1: Single Run Summary ==="
echo "Command: ./testreport summary -id 1 -db $DB_PATH"
echo ""
./testreport summary -id 1 -db "$DB_PATH"

echo ""
echo "=== Example 2: Comparison Report (Regression Detection) ==="
echo "Command: ./testreport compare -base 1 -compare 2 -db $DB_PATH"
echo ""
./testreport compare -base 1 -compare 2 -db "$DB_PATH"

echo ""
echo "=== Example 3: Trend Analysis ==="
echo "Command: ./testreport trend -metric avg_tps -days 7 -db $DB_PATH"
echo ""
./testreport trend -metric avg_tps -days 7 -db "$DB_PATH"

echo ""
echo "=== Example 4: Generate HTML Report ==="
REPORT_FILE="/tmp/test-reports/comparison.html"
./testreport compare -base 1 -compare 2 -db "$DB_PATH" -format html -output "$REPORT_FILE"
echo "HTML report generated: $REPORT_FILE"

echo ""
echo "=== Example 5: JSON Output ==="
echo "Command: ./testreport summary -id 1 -db $DB_PATH -format json"
echo ""
./testreport summary -id 1 -db "$DB_PATH" -format json | head -30

echo ""
echo "Examples completed successfully!"
echo "Database: $DB_PATH"
echo "HTML Report: $REPORT_FILE"
