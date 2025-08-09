package main

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Enhanced visual demonstration with automatic partition failures
func main() {
	fmt.Println("================================================================================")
	fmt.Println("           VISUAL LAG AND CATCH-UP DEMONSTRATION")
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println("This demo automatically shows:")
	fmt.Println("  • Partition failures causing lag to build up")
	fmt.Println("  • Visual progress bars showing lag amount")
	fmt.Println("  • Catch-up rates when partitions recover")
	fmt.Println("  • Real-time metrics and statistics")
	fmt.Println()
	fmt.Println("Starting demonstration...")
	time.Sleep(2 * time.Second)
	
	demo := NewLagDemo()
	demo.Run()
}

type LagDemo struct {
	partitions map[string]*PartitionInfo
	mu         sync.RWMutex
	
	globalSeq  int64
	startTime  time.Time
	ctx        context.Context
	cancel     context.CancelFunc
}

type PartitionInfo struct {
	Name              string
	IsHealthy         bool
	IsPaused          bool  // Partition is paused (not processing)
	
	// Sequences
	LastSent          int64
	LastProcessed     int64
	Lag               int64
	MaxLag            int64
	
	// Timing
	DownTime          time.Time
	RecoveryStartTime time.Time
	CatchUpDuration   time.Duration
	
	// Metrics
	TotalSent         int64
	TotalProcessed    int64
	TotalDropped      int64
	
	// Catch-up tracking
	CatchUpRate       float64
	CatchUpStartSeq   int64
	CatchUpStartTime  time.Time
}

func NewLagDemo() *LagDemo {
	ctx, cancel := context.WithCancel(context.Background())
	
	demo := &LagDemo{
		partitions: make(map[string]*PartitionInfo),
		startTime:  time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}
	
	// Initialize partitions
	for _, name := range []string{"BVN0", "BVN1", "BVN2", "Directory"} {
		demo.partitions[name] = &PartitionInfo{
			Name:      name,
			IsHealthy: true,
		}
	}
	
	return demo
}

func (demo *LagDemo) Run() {
	// Start transaction generator
	go demo.generateTransactions()
	
	// Start partition processors
	go demo.processPartitions()
	
	// Start failure simulator
	go demo.simulateFailures()
	
	// Display loop
	demo.displayLoop()
}

func (demo *LagDemo) generateTransactions() {
	ticker := time.NewTicker(25 * time.Millisecond) // 40 tx/sec total
	defer ticker.Stop()
	
	partitionNames := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	
	for {
		select {
		case <-demo.ctx.Done():
			return
		case <-ticker.C:
			seq := atomic.AddInt64(&demo.globalSeq, 1)
			
			// Send to each partition
			for _, name := range partitionNames {
				demo.mu.Lock()
				partition := demo.partitions[name]
				
				partition.TotalSent++
				partition.LastSent = seq
				
				if partition.IsPaused {
					// Partition is down - transaction would be dropped
					partition.TotalDropped++
					partition.Lag = partition.LastSent - partition.LastProcessed
					if partition.Lag > partition.MaxLag {
						partition.MaxLag = partition.Lag
					}
				} else {
					// Partition is up - will be processed
					partition.Lag = partition.LastSent - partition.LastProcessed
				}
				
				demo.mu.Unlock()
			}
		}
	}
}

func (demo *LagDemo) processPartitions() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-demo.ctx.Done():
			return
		case <-ticker.C:
			demo.mu.Lock()
			
			for _, partition := range demo.partitions {
				if !partition.IsPaused && partition.IsHealthy {
					// Process transactions
					toProcess := int64(1)
					
					// If catching up, process faster
					if partition.Lag > 10 {
						toProcess = int64(math.Min(10, float64(partition.Lag)))
						
						// Track catch-up rate
						if partition.CatchUpStartTime.IsZero() {
							partition.CatchUpStartTime = time.Now()
							partition.CatchUpStartSeq = partition.LastProcessed
						}
					} else if !partition.CatchUpStartTime.IsZero() {
						// Finished catching up
						duration := time.Since(partition.CatchUpStartTime).Seconds()
						caught := partition.LastProcessed - partition.CatchUpStartSeq
						if duration > 0 {
							partition.CatchUpRate = float64(caught) / duration
						}
						partition.CatchUpDuration = time.Since(partition.CatchUpStartTime)
						partition.CatchUpStartTime = time.Time{}
					}
					
					// Process transactions
					if partition.LastProcessed < partition.LastSent {
						partition.LastProcessed = min(partition.LastProcessed+toProcess, partition.LastSent)
						partition.TotalProcessed += toProcess
						partition.Lag = partition.LastSent - partition.LastProcessed
					}
				}
			}
			
			demo.mu.Unlock()
		}
	}
}

func (demo *LagDemo) simulateFailures() {
	// Automated failure scenario
	scenarios := []struct {
		delay    time.Duration
		action   func()
		message  string
	}{
		{
			delay: 5 * time.Second,
			action: func() {
				demo.pausePartition("BVN1")
			},
			message: "💥 BVN1 FAILED - Lag will start building",
		},
		{
			delay: 10 * time.Second,
			action: func() {
				demo.resumePartition("BVN1")
			},
			message: "🔄 BVN1 RECOVERED - Starting catch-up",
		},
		{
			delay: 5 * time.Second,
			action: func() {
				demo.pausePartition("BVN2")
				demo.pausePartition("Directory")
			},
			message: "💥 MULTIPLE FAILURES - BVN2 and Directory down",
		},
		{
			delay: 12 * time.Second,
			action: func() {
				demo.resumePartition("BVN2")
				demo.resumePartition("Directory")
			},
			message: "✅ ALL PARTITIONS RECOVERED - Watch catch-up rates",
		},
		{
			delay: 8 * time.Second,
			action: func() {
				// Cascade failure
				demo.pausePartition("BVN0")
				time.Sleep(500 * time.Millisecond)
				demo.pausePartition("BVN1")
				time.Sleep(500 * time.Millisecond)
				demo.pausePartition("BVN2")
			},
			message: "💥 CASCADE FAILURE - Partitions failing in sequence",
		},
		{
			delay: 15 * time.Second,
			action: func() {
				demo.resumePartition("BVN0")
				demo.resumePartition("BVN1")
				demo.resumePartition("BVN2")
			},
			message: "🚀 MASS RECOVERY - All BVNs coming back online",
		},
	}
	
	for _, scenario := range scenarios {
		select {
		case <-demo.ctx.Done():
			return
		case <-time.After(scenario.delay):
			fmt.Printf("\n%s\n", scenario.message)
			scenario.action()
		}
	}
}

func (demo *LagDemo) pausePartition(name string) {
	demo.mu.Lock()
	defer demo.mu.Unlock()
	
	if partition, exists := demo.partitions[name]; exists {
		partition.IsPaused = true
		partition.IsHealthy = false
		partition.DownTime = time.Now()
	}
}

func (demo *LagDemo) resumePartition(name string) {
	demo.mu.Lock()
	defer demo.mu.Unlock()
	
	if partition, exists := demo.partitions[name]; exists {
		partition.IsPaused = false
		partition.IsHealthy = true
		partition.RecoveryStartTime = time.Now()
		partition.CatchUpStartTime = time.Time{} // Reset for new catch-up tracking
	}
}

func (demo *LagDemo) displayLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	// Run for 60 seconds
	timeout := time.After(60 * time.Second)
	
	for {
		select {
		case <-timeout:
			demo.printFinalReport()
			demo.cancel()
			return
		case <-ticker.C:
			demo.updateDisplay()
		}
	}
}

func (demo *LagDemo) updateDisplay() {
	// Clear screen
	fmt.Print("\033[H\033[2J")
	
	// Header
	fmt.Println("================================================================================")
	fmt.Println("                LAG BUILD-UP AND CATCH-UP VISUALIZATION")
	fmt.Println("================================================================================")
	fmt.Printf("Running: %s | Global Sequence: %d | Time: %s\n",
		time.Since(demo.startTime).Round(time.Second),
		atomic.LoadInt64(&demo.globalSeq),
		time.Now().Format("15:04:05"))
	fmt.Println()
	
	// Partition table
	fmt.Println("┌──────────┬─────────┬──────────┬───────────┬─────┬──────┬─────────────────────────┐")
	fmt.Println("│ Partition│ Status  │   Sent   │ Processed │ Lag │ Max  │ Lag Visualization       │")
	fmt.Println("├──────────┼─────────┼──────────┼───────────┼─────┼──────┼─────────────────────────┤")
	
	demo.mu.RLock()
	defer demo.mu.RUnlock()
	
	totalLag := int64(0)
	totalDropped := int64(0)
	downCount := 0
	
	for _, name := range []string{"BVN0", "BVN1", "BVN2", "Directory"} {
		p := demo.partitions[name]
		
		status := "🟢 UP   "
		if p.IsPaused {
			status = "🔴 DOWN "
			downCount++
		} else if p.Lag > 10 {
			status = "🟡 CATCH"
		}
		
		// Create visual lag bar
		lagBar := demo.createLagBar(p.Lag, 100)
		
		fmt.Printf("│ %-8s │ %s │ %8d │ %9d │ %3d │ %4d │ %s │\n",
			p.Name,
			status,
			p.TotalSent,
			p.TotalProcessed,
			p.Lag,
			p.MaxLag,
			lagBar)
		
		totalLag += p.Lag
		totalDropped += p.TotalDropped
	}
	
	fmt.Println("└──────────┴─────────┴──────────┴───────────┴─────┴──────┴─────────────────────────┘")
	
	// Statistics
	fmt.Println()
	fmt.Println("📊 STATISTICS")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("  Total Lag: %d sequences | Dropped: %d transactions | Down: %d partitions\n",
		totalLag, totalDropped, downCount)
	
	// Catch-up status
	fmt.Println()
	fmt.Println("🚀 CATCH-UP STATUS")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	
	catchingUp := false
	for _, name := range []string{"BVN0", "BVN1", "BVN2", "Directory"} {
		p := demo.partitions[name]
		
		if !p.CatchUpStartTime.IsZero() {
			catchingUp = true
			elapsed := time.Since(p.CatchUpStartTime).Seconds()
			rate := float64(0)
			if elapsed > 0 {
				caught := p.LastProcessed - p.CatchUpStartSeq
				rate = float64(caught) / elapsed
			}
			
			eta := "calculating..."
			if rate > 0 {
				remaining := float64(p.Lag) / rate
				eta = fmt.Sprintf("%.1fs", remaining)
			}
			
			fmt.Printf("  %s catching up: %d behind, Rate: %.1f tx/s, ETA: %s\n",
				p.Name, p.Lag, rate, eta)
		}
		
		if p.CatchUpRate > 0 && p.CatchUpStartTime.IsZero() {
			fmt.Printf("  %s caught up: Rate was %.1f tx/s, Duration: %s\n",
				p.Name, p.CatchUpRate, p.CatchUpDuration.Round(time.Millisecond))
		}
		
		if p.IsPaused && !p.DownTime.IsZero() {
			fmt.Printf("  %s has been down for %s (lag: %d, max: %d)\n",
				p.Name, time.Since(p.DownTime).Round(time.Second), p.Lag, p.MaxLag)
		}
	}
	
	if !catchingUp && downCount == 0 && totalLag == 0 {
		fmt.Println("  ✅ All partitions are in sync")
	}
	
	// Instructions
	fmt.Println()
	fmt.Println("💡 WATCH FOR:")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	fmt.Println("  • Lag building when partitions go down (red)")
	fmt.Println("  • Fast catch-up when partitions recover (yellow)")
	fmt.Println("  • Catch-up rates showing recovery speed")
	fmt.Println("  • Maximum lag reached during outages")
}

func (demo *LagDemo) createLagBar(lag, max int64) string {
	if lag <= 0 {
		return "│                       │"
	}
	
	barWidth := 23
	fillRatio := float64(lag) / float64(max)
	if fillRatio > 1 {
		fillRatio = 1
	}
	
	filled := int(fillRatio * float64(barWidth))
	
	// Color based on severity
	bar := "│"
	for i := 0; i < barWidth; i++ {
		if i < filled {
			if lag > 50 {
				bar += "█" // Heavy lag
			} else if lag > 20 {
				bar += "▓" // Medium lag
			} else {
				bar += "▒" // Light lag
			}
		} else {
			bar += " "
		}
	}
	bar += "│"
	
	return bar
}

func (demo *LagDemo) printFinalReport() {
	fmt.Print("\033[H\033[2J")
	fmt.Println("================================================================================")
	fmt.Println("                           DEMONSTRATION COMPLETE")
	fmt.Println("================================================================================")
	fmt.Println()
	
	demo.mu.RLock()
	defer demo.mu.RUnlock()
	
	fmt.Println("📋 FINAL REPORT")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	
	for _, name := range []string{"BVN0", "BVN1", "BVN2", "Directory"} {
		p := demo.partitions[name]
		
		efficiency := float64(0)
		if p.TotalSent > 0 {
			efficiency = float64(p.TotalProcessed) / float64(p.TotalSent) * 100
		}
		
		fmt.Printf("\n%s:\n", p.Name)
		fmt.Printf("  Total Sent:      %d\n", p.TotalSent)
		fmt.Printf("  Total Processed: %d\n", p.TotalProcessed)
		fmt.Printf("  Total Dropped:   %d\n", p.TotalDropped)
		fmt.Printf("  Max Lag:         %d sequences\n", p.MaxLag)
		fmt.Printf("  Efficiency:      %.1f%%\n", efficiency)
		
		if p.CatchUpRate > 0 {
			fmt.Printf("  Best Catch-up:   %.1f tx/s\n", p.CatchUpRate)
		}
	}
	
	fmt.Println()
	fmt.Println("✅ KEY INSIGHTS:")
	fmt.Println("  • Partitions accumulate lag when down")
	fmt.Println("  • Catch-up happens automatically when recovered")
	fmt.Println("  • System tracks and visualizes lag in real-time")
	fmt.Println("  • Recovery rates depend on lag amount")
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}