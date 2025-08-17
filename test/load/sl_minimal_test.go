//go:build !testnet
// +build !testnet

package load_test

import (
	"fmt"
	"testing"
	"time"
)

// TestMinimalLoadWithTiming demonstrates the TPS calculation including settlement
func TestMinimalLoadWithTiming(t *testing.T) {
	numTxs := 20000
	batchDelay := 1 * time.Second
	batchSize := 1000
	
	t.Logf("=== LOAD TEST CONFIGURATION ===")
	t.Logf("Total transactions: %d", numTxs)
	t.Logf("Batch delay: %v every %d transactions", batchDelay, batchSize)
	
	// Start timing for end-to-end measurement
	endToEndStart := time.Now()
	
	// Simulate sending phase
	t.Log("=== SENDING PHASE ===")
	sendStart := time.Now()
	
	successCount := 0
	failCount := 0
	
	for i := 0; i < numTxs; i++ {
		// Simulate transaction send (immediate success/fail)
		if i%3 == 0 { // Simulate 66% success rate
			successCount++
		} else {
			failCount++
		}
		
		// Apply batch delay
		if (i+1)%batchSize == 0 && i < numTxs-1 {
			t.Logf("Batch %d complete, delaying %v...", (i+1)/batchSize, batchDelay)
			time.Sleep(batchDelay)
		}
	}
	
	sendDuration := time.Since(sendStart)
	sendTPS := float64(successCount) / sendDuration.Seconds()
	
	t.Logf("Sent %d transactions in %v", numTxs, sendDuration)
	t.Logf("Success: %d, Failed: %d", successCount, failCount)
	t.Logf("Send phase TPS: %.2f transactions/second", sendTPS)
	
	// Simulate settlement phase
	t.Log("=== SETTLEMENT PHASE ===")
	settlementStart := time.Now()
	
	// Simulate settlement time (based on network characteristics)
	// For 20k transactions, settlement might take 3-5 minutes
	settlementTime := 30 * time.Second // Shortened for demo
	t.Logf("Simulating settlement for %v...", settlementTime)
	time.Sleep(settlementTime)
	
	settlementDuration := time.Since(settlementStart)
	
	// Calculate end-to-end metrics
	endToEndDuration := time.Since(endToEndStart)
	endToEndTPS := float64(successCount) / endToEndDuration.Seconds()
	
	// Display comprehensive timing report
	t.Log("\n=== TIMING SUMMARY ===")
	t.Logf("Send phase duration: %v", sendDuration)
	t.Logf("Settlement phase duration: %v", settlementDuration)
	t.Logf("Total end-to-end duration: %v", endToEndDuration)
	
	t.Log("\n=== TPS COMPARISON ===")
	t.Logf("Send-only TPS: %.2f tx/s", sendTPS)
	t.Logf("End-to-end TPS (including settlement): %.2f tx/s", endToEndTPS)
	t.Logf("TPS reduction factor: %.2fx", sendTPS/endToEndTPS)
	
	// Calculate effective throughput
	expectedBatchDelays := (numTxs / batchSize) - 1
	if expectedBatchDelays < 0 {
		expectedBatchDelays = 0
	}
	totalBatchDelayTime := time.Duration(expectedBatchDelays) * batchDelay
	
	t.Log("\n=== BATCH DELAY ANALYSIS ===")
	t.Logf("Number of batch delays: %d", expectedBatchDelays)
	t.Logf("Total batch delay time: %v", totalBatchDelayTime)
	t.Logf("Send time without delays: %v", sendDuration-totalBatchDelayTime)
	
	// Show what the TPS would be without delays
	if sendDuration > totalBatchDelayTime {
		rawSendTime := (sendDuration - totalBatchDelayTime).Seconds()
		rawTPS := float64(successCount) / rawSendTime
		t.Logf("TPS without batch delays: %.2f tx/s", rawTPS)
	}
}

// Helper to format duration nicely
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.2f seconds", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := d.Seconds() - float64(minutes*60)
	return fmt.Sprintf("%d minutes %.2f seconds", minutes, seconds)
}