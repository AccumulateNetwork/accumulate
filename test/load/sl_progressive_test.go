//go:build !testnet
// +build !testnet

package load_test

import (
	"testing"
	"time"
)

// Default configuration values
const (
	defaultTotalTxs      = 100000
	defaultReportInterval = 10000
	defaultBatchSize     = 1000
	defaultBatchDelayMs  = 1000
	defaultSettleTimeMs  = 5000
)

// Metrics tracks performance metrics for a segment
type Metrics struct {
	StartTime      time.Time
	EndTime        time.Time
	TxCount        int
	SuccessCount   int
	FailCount      int
	SendDuration   time.Duration
	SettleDuration time.Duration
	TotalDuration  time.Duration
	SendTPS        float64
	EndToEndTPS    float64
	CumulativeLag  time.Duration
}

func TestProgressiveLoadWithReporting(t *testing.T) {
	// Use default values (in a real test, these could come from env vars or test config)
	totalTxs := defaultTotalTxs
	reportInterval := 20000 // Report every 20k as requested
	batchSize := defaultBatchSize
	batchDelay := time.Duration(defaultBatchDelayMs) * time.Millisecond
	settleTime := time.Duration(defaultSettleTimeMs) * time.Millisecond
	
	t.Logf("=== PROGRESSIVE LOAD TEST CONFIGURATION ===")
	t.Logf("Total transactions: %d", totalTxs)
	t.Logf("Report interval: every %d transactions", reportInterval)
	t.Logf("Batch size: %d transactions", batchSize)
	t.Logf("Batch delay: %v", batchDelay)
	t.Logf("Settlement time per interval: %v", settleTime)
	t.Log("")
	
	// Overall test start time
	testStart := time.Now()
	
	// Track cumulative metrics
	var allMetrics []Metrics
	totalSuccess := 0
	totalFail := 0
	cumulativeLag := time.Duration(0)
	
	// Expected time without any delays (baseline)
	baselineTimePerTx := 100 * time.Microsecond // Assume 100μs per transaction in ideal conditions
	
	for txsSent := 0; txsSent < totalTxs; {
		// Determine how many transactions to send in this segment
		segmentSize := reportInterval
		if txsSent+segmentSize > totalTxs {
			segmentSize = totalTxs - txsSent
		}
		
		segmentStart := time.Now()
		expectedDuration := time.Duration(segmentSize) * baselineTimePerTx
		
		// Send transactions for this segment
		t.Logf("=== SEGMENT %d: Transactions %d - %d ===", 
			len(allMetrics)+1, txsSent+1, txsSent+segmentSize)
		
		sendStart := time.Now()
		segmentSuccess := 0
		segmentFail := 0
		
		for i := 0; i < segmentSize; i++ {
			// Simulate transaction send with 66% success rate
			if (txsSent+i)%3 != 0 {
				segmentSuccess++
				totalSuccess++
			} else {
				segmentFail++
				totalFail++
			}
			
			// Apply batch delay if needed
			if (txsSent+i+1)%batchSize == 0 && (txsSent+i+1) < totalTxs {
				time.Sleep(batchDelay)
			}
		}
		
		sendDuration := time.Since(sendStart)
		
		// Simulate settlement phase
		time.Sleep(settleTime)
		settleDuration := settleTime
		
		// Calculate metrics for this segment
		segmentEnd := time.Now()
		segmentDuration := segmentEnd.Sub(segmentStart)
		
		// Calculate lag (actual time vs expected baseline time)
		actualSendTime := sendDuration - (time.Duration(segmentSize/batchSize) * batchDelay)
		lag := actualSendTime - expectedDuration
		cumulativeLag += lag
		
		// Calculate TPS rates
		sendTPS := float64(segmentSuccess) / sendDuration.Seconds()
		endToEndTPS := float64(segmentSuccess) / segmentDuration.Seconds()
		
		// Store metrics
		metrics := Metrics{
			StartTime:      segmentStart,
			EndTime:        segmentEnd,
			TxCount:        segmentSize,
			SuccessCount:   segmentSuccess,
			FailCount:      segmentFail,
			SendDuration:   sendDuration,
			SettleDuration: settleDuration,
			TotalDuration:  segmentDuration,
			SendTPS:        sendTPS,
			EndToEndTPS:    endToEndTPS,
			CumulativeLag:  cumulativeLag,
		}
		allMetrics = append(allMetrics, metrics)
		
		// Print segment report
		printSegmentReport(t, len(allMetrics), metrics, txsSent+segmentSize, totalTxs)
		
		txsSent += segmentSize
	}
	
	// Print final summary
	testDuration := time.Since(testStart)
	printFinalSummary(t, allMetrics, totalSuccess, totalFail, testDuration)
}

func printSegmentReport(t *testing.T, segmentNum int, m Metrics, totalSent, totalTarget int) {
	t.Logf("Segment %d Report:", segmentNum)
	t.Logf("  Transactions: %d (Success: %d, Failed: %d)", 
		m.TxCount, m.SuccessCount, m.FailCount)
	t.Logf("  Send duration: %v", m.SendDuration)
	t.Logf("  Settlement duration: %v", m.SettleDuration)
	t.Logf("  Total segment duration: %v", m.TotalDuration)
	t.Logf("  Send TPS: %.2f tx/s", m.SendTPS)
	t.Logf("  End-to-end TPS: %.2f tx/s", m.EndToEndTPS)
	t.Logf("  Cumulative lag: %v", m.CumulativeLag)
	t.Logf("  Progress: %d / %d (%.1f%%)", 
		totalSent, totalTarget, float64(totalSent)/float64(totalTarget)*100)
	t.Log("")
}

func printFinalSummary(t *testing.T, metrics []Metrics, totalSuccess, totalFail int, totalDuration time.Duration) {
	t.Log("=== FINAL SUMMARY ===")
	t.Logf("Total transactions: %d", totalSuccess+totalFail)
	t.Logf("Total successful: %d (%.1f%%)", 
		totalSuccess, float64(totalSuccess)/float64(totalSuccess+totalFail)*100)
	t.Logf("Total failed: %d", totalFail)
	t.Logf("Total test duration: %v", totalDuration)
	
	// Calculate overall TPS
	overallTPS := float64(totalSuccess) / totalDuration.Seconds()
	t.Logf("Overall TPS: %.2f tx/s", overallTPS)
	
	// Calculate average metrics across segments
	if len(metrics) > 0 {
		var avgSendTPS, avgEndToEndTPS float64
		var totalSendTime, totalSettleTime time.Duration
		
		for _, m := range metrics {
			avgSendTPS += m.SendTPS
			avgEndToEndTPS += m.EndToEndTPS
			totalSendTime += m.SendDuration
			totalSettleTime += m.SettleDuration
		}
		
		avgSendTPS /= float64(len(metrics))
		avgEndToEndTPS /= float64(len(metrics))
		
		t.Log("\n=== PERFORMANCE ANALYSIS ===")
		t.Logf("Average send TPS: %.2f tx/s", avgSendTPS)
		t.Logf("Average end-to-end TPS: %.2f tx/s", avgEndToEndTPS)
		t.Logf("Total send time: %v", totalSendTime)
		t.Logf("Total settlement time: %v", totalSettleTime)
		t.Logf("Send time percentage: %.1f%%", 
			float64(totalSendTime)/float64(totalDuration)*100)
		t.Logf("Settlement time percentage: %.1f%%", 
			float64(totalSettleTime)/float64(totalDuration)*100)
		
		// Show TPS degradation over time if any
		if len(metrics) > 1 {
			firstSegmentTPS := metrics[0].SendTPS
			lastSegmentTPS := metrics[len(metrics)-1].SendTPS
			degradation := (firstSegmentTPS - lastSegmentTPS) / firstSegmentTPS * 100
			
			t.Log("\n=== TPS TREND ANALYSIS ===")
			t.Logf("First segment TPS: %.2f tx/s", firstSegmentTPS)
			t.Logf("Last segment TPS: %.2f tx/s", lastSegmentTPS)
			if degradation > 0 {
				t.Logf("TPS degradation: %.1f%%", degradation)
			} else {
				t.Logf("TPS improvement: %.1f%%", -degradation)
			}
		}
		
		// Final lag analysis
		if len(metrics) > 0 {
			finalLag := metrics[len(metrics)-1].CumulativeLag
			t.Log("\n=== LAG ANALYSIS ===")
			t.Logf("Final cumulative lag: %v", finalLag)
			t.Logf("Average lag per transaction: %v", 
				time.Duration(int64(finalLag)/int64(totalSuccess+totalFail)))
		}
	}
}

