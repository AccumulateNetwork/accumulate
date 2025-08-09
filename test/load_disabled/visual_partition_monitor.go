package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

// PartitionMonitor tracks lag and catch-up rates for each partition
type PartitionMonitor struct {
	partitions map[string]*PartitionState
	mu         sync.RWMutex
	logger     logging.OptionalLogger
	startTime  time.Time

	// Global sequence counter
	globalSequence int64
}

// PartitionState tracks the state of a single partition
type PartitionState struct {
	Name              string
	IsHealthy         bool
	LastSentSequence  int64 // Last sequence we tried to send
	LastAckedSequence int64 // Last sequence acknowledged by partition
	LagAmount         int64 // How many sequences behind
	DownSince         time.Time
	RecoveredAt       time.Time
	CatchUpStarted    time.Time
	CatchUpRate       float64 // Sequences per second during catch-up

	// Metrics
	TotalSent        int64
	TotalAcked       int64
	TotalDropped     int64
	ConsecutiveFails int32

	// Circuit breaker
	CircuitOpen     bool
	CircuitOpenTime time.Time
}

// VisualDashboard manages the display
type VisualDashboard struct {
	monitor     *PartitionMonitor
	dispatcher  *SmartDispatcher
	handler     *SmartPartitionHandler
	ctx         context.Context
	cancel      context.CancelFunc
	refreshRate time.Duration
}

// SmartDispatcher simulates network with lag tracking
type SmartDispatcher struct {
	partitionHealth map[string]bool
	partitionQueues map[string]*MessageQueue
	mu              sync.RWMutex
	networkLatency  time.Duration

	// Metrics
	totalSubmits   int64
	totalSuccesses int64
	totalFailures  int64
}

// MessageQueue simulates a partition's message queue
type MessageQueue struct {
	messages       []QueuedMessage
	mu             sync.Mutex
	processingRate time.Duration // How fast the partition processes messages
}

type QueuedMessage struct {
	Sequence  int64
	Timestamp time.Time
	Processed bool
}

// SmartPartitionHandler handles partitions with health awareness
type SmartPartitionHandler struct {
	monitor    *PartitionMonitor
	dispatcher *SmartDispatcher
	logger     logging.OptionalLogger

	// Configuration
	maxRetries       int
	retryDelay       time.Duration
	failureThreshold int
}

func main() {
	fmt.Println("================================================================================")
	fmt.Println("           VISUAL PARTITION LAG AND CATCH-UP MONITOR")
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println("This monitor shows:")
	fmt.Println("  • Real-time lag for each partition")
	fmt.Println("  • Catch-up rate when partitions recover")
	fmt.Println("  • Visual progress bars for lag")
	fmt.Println("  • Automatic pause/resume based on partition health")
	fmt.Println()
	fmt.Println("Controls:")
	fmt.Println("  Press 1-4 to toggle partition health (1=BVN0, 2=BVN1, 3=BVN2, 4=Directory)")
	fmt.Println("  Press 'c' to cause cascading failure")
	fmt.Println("  Press 'r' to recover all partitions")
	fmt.Println("  Press 'q' to quit")
	fmt.Println()
	fmt.Println("Starting in 3 seconds...")
	time.Sleep(3 * time.Second)

	// Initialize components
	monitor := NewPartitionMonitor()
	dispatcher := NewSmartDispatcher()
	handler := NewSmartPartitionHandler(monitor, dispatcher)

	// Create dashboard
	dashboard := NewVisualDashboard(monitor, dispatcher, handler)

	// Start the system
	dashboard.Start()
}

func NewPartitionMonitor() *PartitionMonitor {
	pm := &PartitionMonitor{
		partitions: make(map[string]*PartitionState),
		startTime:  time.Now(),
	}

	// Initialize partitions
	partitionNames := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	for _, name := range partitionNames {
		pm.partitions[name] = &PartitionState{
			Name:      name,
			IsHealthy: true,
		}
	}

	return pm
}

func NewSmartDispatcher() *SmartDispatcher {
	sd := &SmartDispatcher{
		partitionHealth: make(map[string]bool),
		partitionQueues: make(map[string]*MessageQueue),
		networkLatency:  5 * time.Millisecond,
	}

	// Initialize all partitions as healthy
	partitionNames := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	for _, name := range partitionNames {
		sd.partitionHealth[name] = true
		sd.partitionQueues[name] = &MessageQueue{
			processingRate: 10 * time.Millisecond,
		}
	}

	return sd
}

func NewSmartPartitionHandler(monitor *PartitionMonitor, dispatcher *SmartDispatcher) *SmartPartitionHandler {
	return &SmartPartitionHandler{
		monitor:          monitor,
		dispatcher:       dispatcher,
		maxRetries:       3,
		retryDelay:       100 * time.Millisecond,
		failureThreshold: 3,
	}
}

func NewVisualDashboard(monitor *PartitionMonitor, dispatcher *SmartDispatcher, handler *SmartPartitionHandler) *VisualDashboard {
	ctx, cancel := context.WithCancel(context.Background())
	return &VisualDashboard{
		monitor:     monitor,
		dispatcher:  dispatcher,
		handler:     handler,
		ctx:         ctx,
		cancel:      cancel,
		refreshRate: 500 * time.Millisecond,
	}
}

func (vd *VisualDashboard) Start() {
	// Start transaction generators
	vd.startTransactionGenerators()

	// Start partition processors
	vd.startPartitionProcessors()

	// Start keyboard input handler
	go vd.handleKeyboardInput()

	// Start display loop
	vd.runDisplayLoop()
}

func (vd *VisualDashboard) startTransactionGenerators() {
	// Generate transactions for each partition
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}

	for _, partition := range partitions {
		go func(p string) {
			ticker := time.NewTicker(50 * time.Millisecond) // 20 tx/sec per partition
			defer ticker.Stop()

			for {
				select {
				case <-vd.ctx.Done():
					return
				case <-ticker.C:
					vd.sendTransaction(p)
				}
			}
		}(partition)
	}
}

func (vd *VisualDashboard) sendTransaction(partition string) {
	// Increment global sequence
	sequence := atomic.AddInt64(&vd.monitor.globalSequence, 1)

	vd.monitor.mu.Lock()
	state := vd.monitor.partitions[partition]

	// Update last sent sequence
	state.LastSentSequence = sequence
	state.TotalSent++

	// Check if partition is healthy
	if !state.IsHealthy {
		state.TotalDropped++
		state.LagAmount = sequence - state.LastAckedSequence
		vd.monitor.mu.Unlock()
		return
	}

	// Check circuit breaker
	if state.CircuitOpen {
		// Check if we should try half-open
		if time.Since(state.CircuitOpenTime) > 5*time.Second {
			state.CircuitOpen = false
		} else {
			state.TotalDropped++
			state.LagAmount = sequence - state.LastAckedSequence
			vd.monitor.mu.Unlock()
			return
		}
	}

	vd.monitor.mu.Unlock()

	// Try to send through dispatcher
	vd.dispatcher.mu.RLock()
	healthy := vd.dispatcher.partitionHealth[partition]
	queue := vd.dispatcher.partitionQueues[partition]
	vd.dispatcher.mu.RUnlock()

	if healthy {
		// Add to partition's queue
		queue.mu.Lock()
		queue.messages = append(queue.messages, QueuedMessage{
			Sequence:  sequence,
			Timestamp: time.Now(),
		})
		queue.mu.Unlock()

		atomic.AddInt64(&vd.dispatcher.totalSubmits, 1)
		atomic.AddInt64(&vd.dispatcher.totalSuccesses, 1)

		// Reset consecutive failures
		vd.monitor.mu.Lock()
		state.ConsecutiveFails = 0
		vd.monitor.mu.Unlock()
	} else {
		// Partition is down
		atomic.AddInt64(&vd.dispatcher.totalFailures, 1)

		vd.monitor.mu.Lock()
		state.ConsecutiveFails++

		// Open circuit if threshold reached
		if state.ConsecutiveFails >= 3 && !state.CircuitOpen {
			state.CircuitOpen = true
			state.CircuitOpenTime = time.Now()
			state.IsHealthy = false
			state.DownSince = time.Now()
		}

		state.LagAmount = sequence - state.LastAckedSequence
		vd.monitor.mu.Unlock()
	}
}

func (vd *VisualDashboard) startPartitionProcessors() {
	// Simulate each partition processing its queue
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}

	for _, partition := range partitions {
		go func(p string) {
			for {
				select {
				case <-vd.ctx.Done():
					return
				default:
					vd.processPartitionQueue(p)
					time.Sleep(10 * time.Millisecond)
				}
			}
		}(partition)
	}
}

func (vd *VisualDashboard) processPartitionQueue(partition string) {
	vd.dispatcher.mu.RLock()
	healthy := vd.dispatcher.partitionHealth[partition]
	queue := vd.dispatcher.partitionQueues[partition]
	vd.dispatcher.mu.RUnlock()

	if !healthy {
		return
	}

	// Process messages in queue
	queue.mu.Lock()
	if len(queue.messages) > 0 {
		// Process batch of messages (simulate catch-up)
		batchSize := 1

		vd.monitor.mu.RLock()
		state := vd.monitor.partitions[partition]
		isCatchingUp := state.LagAmount > 10
		vd.monitor.mu.RUnlock()

		if isCatchingUp {
			batchSize = 5 // Process faster during catch-up
		}

		processed := 0
		for i := 0; i < len(queue.messages) && processed < batchSize; i++ {
			if !queue.messages[i].Processed {
				queue.messages[i].Processed = true

				// Update acknowledged sequence
				vd.monitor.mu.Lock()
				state := vd.monitor.partitions[partition]
				state.LastAckedSequence = queue.messages[i].Sequence
				state.TotalAcked++

				// Update lag
				state.LagAmount = state.LastSentSequence - state.LastAckedSequence
				if state.LagAmount < 0 {
					state.LagAmount = 0
				}

				// Calculate catch-up rate
				if isCatchingUp && state.CatchUpStarted.IsZero() {
					state.CatchUpStarted = time.Now()
				} else if !isCatchingUp && !state.CatchUpStarted.IsZero() {
					duration := time.Since(state.CatchUpStarted).Seconds()
					if duration > 0 {
						state.CatchUpRate = float64(processed) / duration
					}
					state.CatchUpStarted = time.Time{}
				}

				vd.monitor.mu.Unlock()

				processed++
			}
		}

		// Clean up processed messages
		newQueue := []QueuedMessage{}
		for _, msg := range queue.messages {
			if !msg.Processed {
				newQueue = append(newQueue, msg)
			}
		}
		queue.messages = newQueue
	}
	queue.mu.Unlock()
}

func (vd *VisualDashboard) runDisplayLoop() {
	ticker := time.NewTicker(vd.refreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-vd.ctx.Done():
			return
		case <-ticker.C:
			vd.updateDisplay()
		}
	}
}

func (vd *VisualDashboard) updateDisplay() {
	// Clear screen
	fmt.Print("\033[H\033[2J")

	// Header
	fmt.Println("================================================================================")
	fmt.Println("                    PARTITION LAG AND CATCH-UP MONITOR")
	fmt.Println("================================================================================")
	fmt.Printf("Running: %s | Global Sequence: %d\n",
		time.Since(vd.monitor.startTime).Round(time.Second),
		atomic.LoadInt64(&vd.monitor.globalSequence))
	fmt.Println()

	// Partition status table
	fmt.Println("┌─────────────┬──────────┬──────────┬──────────┬──────────┬─────────────────────┐")
	fmt.Println("│ Partition   │ Status   │ Sent     │ Acked    │ Lag      │ Progress            │")
	fmt.Println("├─────────────┼──────────┼──────────┼──────────┼──────────┼─────────────────────┤")

	vd.monitor.mu.RLock()
	defer vd.monitor.mu.RUnlock()

	partitionOrder := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	for _, name := range partitionOrder {
		state := vd.monitor.partitions[name]

		// Status icon
		statusIcon := "🟢"
		statusText := "HEALTHY"
		if !state.IsHealthy || state.CircuitOpen {
			statusIcon = "🔴"
			statusText = "DOWN   "

			if state.CircuitOpen {
				statusText = "CIRCUIT"
			}
		} else if state.LagAmount > 10 {
			statusIcon = "🟡"
			statusText = "LAGGING"
		}

		// Progress bar for lag
		progressBar := vd.createProgressBar(state.LagAmount, 100)

		fmt.Printf("│ %-11s │ %s %-7s │ %-8d │ %-8d │ %-8d │ %s │\n",
			name,
			statusIcon,
			statusText,
			state.TotalSent,
			state.TotalAcked,
			state.LagAmount,
			progressBar)
	}

	fmt.Println("└─────────────┴──────────┴──────────┴──────────┴──────────┴─────────────────────┘")
	fmt.Println()

	// Catch-up status
	fmt.Println("📊 CATCH-UP STATUS")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")

	catchingUp := false
	for _, name := range partitionOrder {
		state := vd.monitor.partitions[name]

		if state.LagAmount > 10 && state.IsHealthy {
			catchingUp = true
			catchUpRate := "calculating..."
			if state.CatchUpRate > 0 {
				catchUpRate = fmt.Sprintf("%.1f tx/sec", state.CatchUpRate)
			}

			eta := "unknown"
			if state.CatchUpRate > 0 {
				seconds := float64(state.LagAmount) / state.CatchUpRate
				eta = fmt.Sprintf("%.1f seconds", seconds)
			}

			fmt.Printf("  %s is catching up: %d behind, Rate: %s, ETA: %s\n",
				name, state.LagAmount, catchUpRate, eta)
		}

		if !state.IsHealthy && !state.DownSince.IsZero() {
			downDuration := time.Since(state.DownSince).Round(time.Second)
			fmt.Printf("  %s has been down for %s (accumulated lag: %d)\n",
				name, downDuration, state.LagAmount)
		}
	}

	if !catchingUp {
		hasDown := false
		for _, name := range partitionOrder {
			if !vd.monitor.partitions[name].IsHealthy {
				hasDown = true
				break
			}
		}

		if !hasDown {
			fmt.Println("  ✅ All partitions are in sync")
		}
	}

	// Network statistics
	fmt.Println()
	fmt.Println("🌐 NETWORK STATISTICS")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")

	totalSubmits := atomic.LoadInt64(&vd.dispatcher.totalSubmits)
	totalSuccesses := atomic.LoadInt64(&vd.dispatcher.totalSuccesses)
	totalFailures := atomic.LoadInt64(&vd.dispatcher.totalFailures)

	successRate := float64(0)
	if totalSubmits > 0 {
		successRate = float64(totalSuccesses) / float64(totalSubmits) * 100
	}

	fmt.Printf("  Submits: %d | Successes: %d | Failures: %d | Success Rate: %.1f%%\n",
		totalSubmits, totalSuccesses, totalFailures, successRate)

	// Calculate total dropped
	totalDropped := int64(0)
	for _, name := range partitionOrder {
		totalDropped += vd.monitor.partitions[name].TotalDropped
	}

	if totalDropped > 0 {
		fmt.Printf("  ⚠️  Dropped Transactions: %d (will be recovered from ledger)\n", totalDropped)
	}

	// Controls reminder
	fmt.Println()
	fmt.Println("🎮 CONTROLS")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	fmt.Println("  1: Toggle BVN0  | 2: Toggle BVN1  | 3: Toggle BVN2  | 4: Toggle Directory")
	fmt.Println("  c: Cascade fail | r: Recover all  | q: Quit")
}

func (vd *VisualDashboard) createProgressBar(current, max int64) string {
	if current <= 0 {
		return "                   "
	}

	barLength := 19
	filled := int(float64(current) / float64(max) * float64(barLength))
	if filled > barLength {
		filled = barLength
	}

	bar := ""
	for i := 0; i < filled; i++ {
		bar += "█"
	}
	for i := filled; i < barLength; i++ {
		bar += "░"
	}

	return bar
}

func (vd *VisualDashboard) handleKeyboardInput() {
	// Note: In a real implementation, you'd use a proper keyboard input library
	// For this demo, we'll simulate with automatic actions

	go func() {
		time.Sleep(10 * time.Second)
		vd.togglePartition("BVN1")

		time.Sleep(15 * time.Second)
		vd.togglePartition("BVN1")

		time.Sleep(10 * time.Second)
		vd.cascadeFailure()

		time.Sleep(15 * time.Second)
		vd.recoverAll()
	}()
}

func (vd *VisualDashboard) togglePartition(name string) {
	vd.dispatcher.mu.Lock()
	current := vd.dispatcher.partitionHealth[name]
	vd.dispatcher.partitionHealth[name] = !current
	vd.dispatcher.mu.Unlock()

	vd.monitor.mu.Lock()
	state := vd.monitor.partitions[name]

	if !current {
		// Was down, now up
		state.IsHealthy = true
		state.CircuitOpen = false
		state.RecoveredAt = time.Now()
		state.ConsecutiveFails = 0
		fmt.Printf("\n🔄 %s RECOVERED\n", name)
	} else {
		// Was up, now down
		state.IsHealthy = false
		state.DownSince = time.Now()
		fmt.Printf("\n💥 %s FAILED\n", name)
	}
	vd.monitor.mu.Unlock()
}

func (vd *VisualDashboard) cascadeFailure() {
	partitions := []string{"BVN0", "BVN1", "BVN2"}
	for _, p := range partitions {
		vd.dispatcher.mu.Lock()
		vd.dispatcher.partitionHealth[p] = false
		vd.dispatcher.mu.Unlock()

		vd.monitor.mu.Lock()
		state := vd.monitor.partitions[p]
		state.IsHealthy = false
		state.DownSince = time.Now()
		vd.monitor.mu.Unlock()

		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("\n💥 CASCADE FAILURE TRIGGERED")
}

func (vd *VisualDashboard) recoverAll() {
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	for _, p := range partitions {
		vd.dispatcher.mu.Lock()
		vd.dispatcher.partitionHealth[p] = true
		vd.dispatcher.mu.Unlock()

		vd.monitor.mu.Lock()
		state := vd.monitor.partitions[p]
		state.IsHealthy = true
		state.CircuitOpen = false
		state.RecoveredAt = time.Now()
		state.ConsecutiveFails = 0
		state.CatchUpStarted = time.Now()
		vd.monitor.mu.Unlock()
	}
	fmt.Println("\n✅ ALL PARTITIONS RECOVERED")
}

// Run the visual monitor
func RunVisualMonitor() {
	monitor := NewPartitionMonitor()
	dispatcher := NewSmartDispatcher()
	handler := NewSmartPartitionHandler(monitor, dispatcher)
	dashboard := NewVisualDashboard(monitor, dispatcher, handler)
	dashboard.Start()
}
