package results

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// MetricStats represents statistical summary of a metric
type MetricStats struct {
	MetricName string
	Count      int64
	Min        float64
	Max        float64
	Avg        float64
	StdDev     float64
}

// ComparisonResult represents a comparison between two test runs
type ComparisonResult struct {
	Metric      string
	Run1Value   float64
	Run2Value   float64
	Difference  float64
	PercentDiff float64
}

// GetMetricStats retrieves statistical summary for a metric in a test run
func (d *Database) GetMetricStats(runID int64, metricName string) (*MetricStats, error) {
	var stats MetricStats
	stats.MetricName = metricName

	var variance float64
	err := d.db.QueryRow(`
		SELECT
			COUNT(*),
			MIN(value),
			MAX(value),
			AVG(value),
			COALESCE(
				AVG(value * value) - AVG(value) * AVG(value),
				0
			) as variance
		FROM metrics_timeseries
		WHERE run_id = ? AND metric_name = ?
	`, runID, metricName).Scan(&stats.Count, &stats.Min, &stats.Max, &stats.Avg, &variance)
	if err != nil {
		return nil, fmt.Errorf("failed to get metric stats: %w", err)
	}

	// Compute standard deviation from variance
	stats.StdDev = math.Sqrt(variance)

	return &stats, nil
}

// GetMetricTimeSeries retrieves time-series data for a metric
func (d *Database) GetMetricTimeSeries(runID int64, metricName string) ([]time.Time, []float64, error) {
	rows, err := d.db.Query(`
		SELECT timestamp, value
		FROM metrics_timeseries
		WHERE run_id = ? AND metric_name = ?
		ORDER BY timestamp
	`, runID, metricName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query time series: %w", err)
	}
	defer rows.Close()

	var timestamps []time.Time
	var values []float64

	for rows.Next() {
		var ts time.Time
		var val float64
		if err := rows.Scan(&ts, &val); err != nil {
			return nil, nil, fmt.Errorf("failed to scan row: %w", err)
		}
		timestamps = append(timestamps, ts)
		values = append(values, val)
	}

	return timestamps, values, nil
}

// CompareMetrics compares average metric values between two test runs
func (d *Database) CompareMetrics(runID1, runID2 int64) ([]*ComparisonResult, error) {
	query := `
		SELECT
			m1.metric_name,
			AVG(m1.value) as avg1,
			AVG(m2.value) as avg2
		FROM metrics_timeseries m1
		INNER JOIN metrics_timeseries m2
			ON m1.metric_name = m2.metric_name
		WHERE m1.run_id = ? AND m2.run_id = ?
		GROUP BY m1.metric_name
		ORDER BY m1.metric_name
	`

	rows, err := d.db.Query(query, runID1, runID2)
	if err != nil {
		return nil, fmt.Errorf("failed to compare metrics: %w", err)
	}
	defer rows.Close()

	var results []*ComparisonResult
	for rows.Next() {
		var result ComparisonResult
		if err := rows.Scan(&result.Metric, &result.Run1Value, &result.Run2Value); err != nil {
			return nil, fmt.Errorf("failed to scan comparison: %w", err)
		}

		result.Difference = result.Run2Value - result.Run1Value
		if result.Run1Value != 0 {
			result.PercentDiff = (result.Difference / result.Run1Value) * 100
		}

		results = append(results, &result)
	}

	return results, nil
}

// GetErrorSummary retrieves error summary for a test run
func (d *Database) GetErrorSummary(runID int64) (map[string]int64, error) {
	rows, err := d.db.Query(`
		SELECT operation_type, COUNT(*)
		FROM test_errors
		WHERE run_id = ?
		GROUP BY operation_type
		ORDER BY COUNT(*) DESC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get error summary: %w", err)
	}
	defer rows.Close()

	summary := make(map[string]int64)
	for rows.Next() {
		var opType sql.NullString
		var count int64
		if err := rows.Scan(&opType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan error summary: %w", err)
		}
		key := "unknown"
		if opType.Valid {
			key = opType.String
		}
		summary[key] = count
	}

	return summary, nil
}

// GetNodeHealthSummary retrieves node health summary for a test run
func (d *Database) GetNodeHealthSummary(runID int64) (map[string]map[string]int64, error) {
	rows, err := d.db.Query(`
		SELECT node_id, status, COUNT(*)
		FROM node_health
		WHERE run_id = ?
		GROUP BY node_id, status
		ORDER BY node_id, status
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node health summary: %w", err)
	}
	defer rows.Close()

	summary := make(map[string]map[string]int64)
	for rows.Next() {
		var nodeID, status string
		var count int64
		if err := rows.Scan(&nodeID, &status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan node health: %w", err)
		}

		if summary[nodeID] == nil {
			summary[nodeID] = make(map[string]int64)
		}
		summary[nodeID][status] = count
	}

	return summary, nil
}

// FindSimilarRuns finds test runs with similar configurations
func (d *Database) FindSimilarRuns(configHash string, limit int) ([]*TestRun, error) {
	return d.ListTestRuns("", "", nil, nil, limit)
}

// GetRunsByCommit gets all test runs for a specific commit
func (d *Database) GetRunsByCommit(commitHash string) ([]*TestRun, error) {
	return d.ListTestRuns(commitHash, "", nil, nil, 0)
}

// GetRunsByDateRange gets test runs within a date range
func (d *Database) GetRunsByDateRange(startDate, endDate time.Time) ([]*TestRun, error) {
	return d.ListTestRuns("", "", &startDate, &endDate, 0)
}

// GetConfig retrieves a test configuration by hash
func (d *Database) GetConfig(configHash string) (*Config, error) {
	var parametersJSON string
	err := d.db.QueryRow(`
		SELECT parameters_json
		FROM test_configs
		WHERE config_hash = ?
	`, configHash).Scan(&parametersJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	var config Config
	if err := json.Unmarshal([]byte(parametersJSON), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// GetRecentMetrics gets the most recent N metrics for a run
func (d *Database) GetRecentMetrics(runID int64, metricName string, limit int) ([]*MetricRecord, error) {
	rows, err := d.db.Query(`
		SELECT run_id, timestamp, metric_name, value, unit, labels_json
		FROM metrics_timeseries
		WHERE run_id = ? AND metric_name = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, runID, metricName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent metrics: %w", err)
	}
	defer rows.Close()

	var metrics []*MetricRecord
	for rows.Next() {
		var metric MetricRecord
		var unit sql.NullString
		var labelsJSON sql.NullString

		err := rows.Scan(&metric.RunID, &metric.Timestamp, &metric.MetricName,
			&metric.Value, &unit, &labelsJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric: %w", err)
		}

		if unit.Valid {
			metric.Unit = unit.String
		}
		if labelsJSON.Valid && labelsJSON.String != "" {
			if err := json.Unmarshal([]byte(labelsJSON.String), &metric.Labels); err != nil {
				return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
			}
		}

		metrics = append(metrics, &metric)
	}

	return metrics, nil
}

// GetErrorsInTimeRange gets errors within a specific time range
func (d *Database) GetErrorsInTimeRange(runID int64, startTime, endTime time.Time) ([]*ErrorRecord, error) {
	rows, err := d.db.Query(`
		SELECT run_id, timestamp, operation_type, error_message, error_details_json
		FROM test_errors
		WHERE run_id = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp
	`, runID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query errors: %w", err)
	}
	defer rows.Close()

	var errors []*ErrorRecord
	for rows.Next() {
		var errRecord ErrorRecord
		var opType sql.NullString
		var detailsJSON sql.NullString

		err := rows.Scan(&errRecord.RunID, &errRecord.Timestamp, &opType,
			&errRecord.ErrorMessage, &detailsJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan error: %w", err)
		}

		if opType.Valid {
			errRecord.OperationType = opType.String
		}
		if detailsJSON.Valid && detailsJSON.String != "" {
			if err := json.Unmarshal([]byte(detailsJSON.String), &errRecord.ErrorDetails); err != nil {
				return nil, fmt.Errorf("failed to unmarshal error details: %w", err)
			}
		}

		errors = append(errors, &errRecord)
	}

	return errors, nil
}
