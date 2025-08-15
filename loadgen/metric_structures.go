package loadgen

import (
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector is the main metrics collection system
type MetricsCollector struct {
	mu sync.RWMutex
	
	// Core components
	session      *SessionMetrics
	volume       *VolumeMetrics
	mix          *TransactionMixMetrics
	performance  *PerformanceMetrics
	errors       *ErrorMetrics
	resources    *ResourceMetrics
	network      *NetworkMetrics
	entities     *EntityMetrics
	balances     *BalanceMetrics
	timeSeries   *TimeSeriesMetrics
	latencyMode  *LatencyModeMetrics
	partition    *PartitionMetrics
	
	// Configuration
	config       *MetricsConfig
	
	// Reporting
	reporters    []MetricsReporter
	exporters    []MetricsExporter
}

// SessionMetrics tracks session timing information
type SessionMetrics struct {
	StartTime          time.Time
	EndTime            time.Time
	ActiveDuration     time.Duration
	PauseDuration      time.Duration
	RampUpDuration     time.Duration
	RampDownDuration   time.Duration
	
	// Window tracking
	CurrentWindowStart time.Time
	WindowDuration     time.Duration
	WindowsCompleted   uint64
	WindowProgress     float64
}

// VolumeMetrics tracks transaction counts and rates
type VolumeMetrics struct {
	// Absolute counts
	TotalAttempted  atomic.Uint64
	TotalSubmitted  atomic.Uint64
	TotalSuccessful atomic.Uint64
	TotalFailed     atomic.Uint64
	TotalPending    atomic.Int64
	TotalTimedOut   atomic.Uint64
	TotalRetried    atomic.Uint64
	
	// Rate metrics
	CurrentTPS      atomic.Value // float64
	AverageTPS      atomic.Value // float64
	PeakTPS         atomic.Value // float64
	MinimumTPS      atomic.Value // float64
	TargetTPS       float64
	TPSVariance     atomic.Value // float64
	
	// Window-based counts
	LastMinute      *WindowCounter
	Last5Minutes    *WindowCounter
	LastHour        *WindowCounter
	CurrentWindow   *WindowCounter
}

// WindowCounter tracks counts in a time window
type WindowCounter struct {
	mu          sync.RWMutex
	window      time.Duration
	buckets     []uint64
	bucketSize  time.Duration
	currentIdx  int
	lastUpdate  time.Time
}

// TransactionMixMetrics tracks transaction type distribution
type TransactionMixMetrics struct {
	mu sync.RWMutex
	
	// Category percentages
	InfrastructurePercent atomic.Value // float64
	ValueTransferPercent  atomic.Value // float64
	DataOperationsPercent atomic.Value // float64
	TokenIssuancePercent  atomic.Value // float64
	
	// Detailed breakdown by type
	TypeMetrics map[TransactionType]*TypeMetric
	
	// Cross-partition metrics
	CrossPartition *CrossPartitionMetrics
}

// TypeMetric tracks metrics for a specific transaction type
type TypeMetric struct {
	Count          atomic.Uint64
	SuccessCount   atomic.Uint64
	FailureCount   atomic.Uint64
	
	// Latency tracking
	latencies      *LatencyTracker
	
	// Calculated fields
	SuccessRate    atomic.Value // float64
	PercentOfTotal atomic.Value // float64
}

// PartitionMetrics tracks metrics by partition
type PartitionMetrics struct {
	mu sync.RWMutex
	
	// Per-partition metrics
	PartitionStats map[string]*PartitionStat
	
	// Cross-partition tracking
	CrossPartition *CrossPartitionMetrics
	
	// Partition distribution
	TotalPartitions      int
	ActivePartitions     atomic.Int64
	PartitionBalance     atomic.Value // float64 - distribution evenness
	HottestPartition     atomic.Value // string - most active partition
	ColdestPartition     atomic.Value // string - least active partition
}

// PartitionStat tracks metrics for a single partition
type PartitionStat struct {
	// Basic counts
	TransactionCount     atomic.Uint64
	SuccessCount        atomic.Uint64
	FailureCount        atomic.Uint64
	
	// Latency
	AvgLatency          atomic.Value // time.Duration
	P95Latency          atomic.Value // time.Duration
	P99Latency          atomic.Value // time.Duration
	
	// Rate
	CurrentTPS          atomic.Value // float64
	PeakTPS            atomic.Value // float64
	
	// Load
	QueueDepth          atomic.Int64
	ActiveConnections   atomic.Int64
	
	// Errors
	ErrorCount          atomic.Uint64
	ErrorRate           atomic.Value // float64
	LastError           atomic.Value // time.Time
	
	// Accounts
	AccountsCreated     atomic.Uint64
	ActiveAccounts      atomic.Uint64
	
	// Cross-partition from this partition
	OutboundCrossPartition atomic.Uint64
	InboundCrossPartition  atomic.Uint64
}

// CrossPartitionMetrics tracks cross-partition transactions
type CrossPartitionMetrics struct {
	// Overall metrics
	Count         atomic.Uint64
	Percentage    atomic.Value // float64
	SuccessRate   atomic.Value // float64
	AvgLatency    atomic.Value // time.Duration
	
	// Detailed tracking
	mu sync.RWMutex
	Routes map[PartitionRoute]*RouteMetrics
	
	// Synthetic transactions
	SyntheticCount      atomic.Uint64
	SyntheticLatency    atomic.Value // time.Duration
	SyntheticSuccess    atomic.Uint64
	SyntheticFailure    atomic.Uint64
	
	// Anchor transactions
	AnchorCount         atomic.Uint64
	AnchorLatency       atomic.Value // time.Duration
	AnchorSuccess       atomic.Uint64
	AnchorFailure       atomic.Uint64
	
	// Performance impact
	CrossPartitionOverhead atomic.Value // time.Duration - additional latency
	ThroughputImpact      atomic.Value // float64 - TPS reduction percentage
}

// PartitionRoute represents a source->destination partition pair
type PartitionRoute struct {
	Source      string
	Destination string
}

// RouteMetrics tracks metrics for a specific partition route
type RouteMetrics struct {
	Count           atomic.Uint64
	SuccessCount    atomic.Uint64
	FailureCount    atomic.Uint64
	AvgLatency      atomic.Value // time.Duration
	P95Latency      atomic.Value // time.Duration
	P99Latency      atomic.Value // time.Duration
	
	// Congestion metrics
	QueuedCount     atomic.Uint64
	DroppedCount    atomic.Uint64
	RetryCount      atomic.Uint64
	
	// Time-based
	LastUsed        atomic.Value // time.Time
	PeakHour        atomic.Value // time.Time
	PeakLoad        atomic.Uint64
}

// PerformanceMetrics tracks latency and throughput
type PerformanceMetrics struct {
	// Submission latency
	SubmissionLatency *LatencyDistribution
	
	// End-to-end processing latency (latency mode)
	ProcessingLatency *LatencyDistribution
	
	// Throughput
	BytesSent         atomic.Uint64
	BytesReceived     atomic.Uint64
	AvgRequestSize    atomic.Value // uint64
	AvgResponseSize   atomic.Value // uint64
	BandwidthUsage    atomic.Value // float64
	PeakBandwidth     atomic.Value // float64
	
	// Queue metrics
	QueueDepth        atomic.Int64
	AvgQueueDepth     atomic.Value // float64
	MaxQueueDepth     atomic.Uint64
	QueueWaitTime     atomic.Value // time.Duration
	QueueOverflow     atomic.Uint64
}

// LatencyDistribution tracks latency percentiles
type LatencyDistribution struct {
	mu sync.RWMutex
	
	// Basic stats
	Minimum       time.Duration
	Maximum       time.Duration
	Average       atomic.Value // time.Duration
	StdDeviation  atomic.Value // time.Duration
	
	// Percentiles
	P1            atomic.Value // time.Duration
	P5            atomic.Value // time.Duration
	P10           atomic.Value // time.Duration
	P25           atomic.Value // time.Duration
	P50           atomic.Value // time.Duration (median)
	P75           atomic.Value // time.Duration
	P90           atomic.Value // time.Duration
	P95           atomic.Value // time.Duration
	P99           atomic.Value // time.Duration
	P999          atomic.Value // time.Duration
	P100          time.Duration // maximum
	
	// Internal tracking
	samples       *ReservoirSample
}

// LatencyModeMetrics specific to latency mode operation
type LatencyModeMetrics struct {
	// Mode configuration
	Mode              LatencyMode
	PollInterval      time.Duration
	PollTimeout       time.Duration
	VerificationMethod string
	BatchPolling      bool
	SmartPolling      bool
	
	// Polling metrics
	AvgPollCount      atomic.Value // float64
	DetectionTime     atomic.Value // time.Duration
	FalsePositiveRate atomic.Value // float64
	PollTimeoutRate   atomic.Value // float64
	
	// Verification metrics
	VerificationRate  atomic.Value // float64
	AvgVerifyTime     atomic.Value // time.Duration
	PollingEfficiency atomic.Value // float64
	TimeoutRate       atomic.Value // float64
	VerifyBacklog     atomic.Int64
}

// LatencyMode defines the latency tracking mode
type LatencyMode int

const (
	FastMode LatencyMode = iota    // Only submission latency
	LatencyMode                     // Full end-to-end tracking
	HybridMode                      // Sample percentage
)

// ErrorMetrics tracks error information
type ErrorMetrics struct {
	mu sync.RWMutex
	
	// Error categories
	NetworkErrors    atomic.Uint64
	ValidationErrors atomic.Uint64
	AuthorityErrors  atomic.Uint64
	BalanceErrors    atomic.Uint64
	StateErrors      atomic.Uint64
	UnknownErrors    atomic.Uint64
	
	// Error tracking
	TotalErrors       atomic.Uint64
	ErrorRate         atomic.Value // float64
	ErrorPercentage   atomic.Value // float64
	ConsecutiveErrors atomic.Uint64
	MaxErrorStreak    atomic.Uint64
	LastErrorTime     atomic.Value // time.Time
	
	// Error details
	ErrorDistribution map[string]*atomic.Uint64
	RecentErrors      *CircularBuffer
	MostCommonError   atomic.Value // string
	
	// Recovery metrics
	RetrySuccess      atomic.Uint64
	RetryFailure      atomic.Uint64
	AvgRetryCount     atomic.Value // float64
	RecoveryRate      atomic.Value // float64
	CircuitTrips      atomic.Uint64
	CircuitRecovery   atomic.Value // time.Duration
}

// ResourceMetrics tracks system resource usage
type ResourceMetrics struct {
	// Worker pool
	ActiveWorkers     atomic.Int64
	IdleWorkers       atomic.Int64
	WorkerUtilization atomic.Value // float64
	AvgWorkerLoad     atomic.Value // float64
	WorkerCreation    atomic.Uint64
	WorkerTermination atomic.Uint64
	
	// Memory
	CurrentMemory     atomic.Uint64
	PeakMemory        atomic.Uint64
	AvgMemory         atomic.Value // uint64
	MemoryGrowthRate  atomic.Value // float64
	GCCount           atomic.Uint64
	GCPauseTime       atomic.Value // time.Duration
	
	// CPU
	CurrentCPU        atomic.Value // float64
	AverageCPU        atomic.Value // float64
	PeakCPU           atomic.Value // float64
	CPUPerTransaction atomic.Value // float64
}

// NetworkMetrics tracks network health
type NetworkMetrics struct {
	// Connection pool
	ActiveConnections   atomic.Int64
	IdleConnections     atomic.Int64
	ConnectionCreation  atomic.Uint64
	ConnectionReuse     atomic.Value // float64
	ConnectionErrors    atomic.Uint64
	AvgConnectionLife   atomic.Value // time.Duration
	
	// Network health
	AverageRTT          atomic.Value // time.Duration
	PacketLossRate      atomic.Value // float64
	ConnectionTimeouts  atomic.Uint64
	DNSResolutionTime   atomic.Value // time.Duration
	TLSHandshakeTime    atomic.Value // time.Duration
}

// EntityMetrics tracks created entities
type EntityMetrics struct {
	// Accounts
	ADIsCreated          atomic.Uint64
	LiteAccountsCreated  atomic.Uint64
	TokenAccountsCreated atomic.Uint64
	DataAccountsCreated  atomic.Uint64
	AvgAccountsPerADI    atomic.Value // float64
	AccountCreationRate  atomic.Value // float64
	
	// Key management
	KeyBooksCreated      atomic.Uint64
	KeyPagesCreated      atomic.Uint64
	KeysAdded            atomic.Uint64
	MultiSigConfigs      atomic.Uint64
	AvgKeysPerPage       atomic.Value // float64
	
	// Tokens
	CustomTokensCreated  atomic.Uint64
	TotalTokensIssued    atomic.Uint64
	TotalTokensBurned    atomic.Uint64
	TokenAccountsLocked  atomic.Uint64
	AvgTokenVelocity     atomic.Value // float64
}

// BalanceMetrics tracks token movements
type BalanceMetrics struct {
	// ACME tracking
	TotalACMEMoved      atomic.Uint64
	AvgTransferAmount   atomic.Value // uint64
	LargestTransfer     atomic.Uint64
	SmallestTransfer    atomic.Uint64
	CreditsConsumed     atomic.Uint64
	CreditsRemaining    atomic.Uint64
	
	// Distribution
	GiniCoefficient     atomic.Value // float64
	Top10Holdings       atomic.Value // float64
	ActiveAccounts      atomic.Uint64
	DormantAccounts     atomic.Uint64
	ZeroBalanceAccounts atomic.Uint64
}

// TimeSeriesMetrics tracks rolling windows and trends
type TimeSeriesMetrics struct {
	mu sync.RWMutex
	
	// Rolling windows
	OneMinuteRolling    *RollingWindow
	FiveMinuteRolling   *RollingWindow
	FifteenMinuteRolling *RollingWindow
	OneHourRolling      *RollingWindow
	
	// Trends
	TPSTrend            Trend
	LatencyTrend        Trend
	ErrorRateTrend      Trend
	QueueDepthTrend     Trend
}

// RollingWindow tracks metrics over a sliding time window
type RollingWindow struct {
	mu         sync.RWMutex
	window     time.Duration
	samples    []Sample
	maxSamples int
}

// Sample represents a metric sample at a point in time
type Sample struct {
	Timestamp time.Time
	Value     float64
}

// Trend represents the direction of a metric
type Trend int

const (
	TrendStable Trend = iota
	TrendIncreasing
	TrendDecreasing
)

// MetricsConfig configures metrics collection
type MetricsConfig struct {
	// Collection settings
	FullCollection   bool
	SamplingRate     int
	ReservoirSize    int
	WindowSize       time.Duration
	
	// Storage settings
	InMemoryBuffers  int
	CompressHistory  bool
	AggregateRollups bool
	MetricRotation   time.Duration
	
	// Alert thresholds
	AlertThresholds  *AlertThresholds
	
	// Export settings
	ExportFormat     []string
	ExportInterval   time.Duration
}

// AlertThresholds defines alert trigger points
type AlertThresholds struct {
	// Performance
	TPSBelowTarget      float64
	LatencyP95          time.Duration
	QueueDepthMax       int
	ErrorRateMax        float64
	
	// System
	MemoryUsageMax      float64
	CPUUsageMax         float64
	WorkerPoolExhausted bool
	ConnectionPoolMax   int
	
	// Critical
	ConsecutiveErrors   int
	CircuitBreakerOpen  bool
	ZeroSuccess         time.Duration
	NetworkUnreachable  bool
}

// MetricsSnapshot provides point-in-time metrics view
type MetricsSnapshot struct {
	Timestamp    time.Time
	Duration     time.Duration
	
	// Transaction counts
	Total        uint64
	Succeeded    uint64
	Failed       uint64
	Pending      uint64
	
	// By type breakdown
	ByType       map[TransactionType]*TypeSnapshot
	
	// Performance
	CurrentTPS   float64
	AverageTPS   float64
	PeakTPS      float64
	
	// Latency percentiles (ms)
	P50          float64
	P95          float64
	P99          float64
	P999         float64
	
	// Errors
	TopErrors    []ErrorSummary
}

// TypeSnapshot is a snapshot of metrics for a transaction type
type TypeSnapshot struct {
	Attempted    uint64
	Succeeded    uint64
	Failed       uint64
	AvgLatency   time.Duration
	P95Latency   time.Duration
}

// ErrorSummary summarizes an error type
type ErrorSummary struct {
	Error        string
	Count        uint64
	LastOccurred time.Time
	Type         TransactionType
}

// MetricsReporter interface for reporting metrics
type MetricsReporter interface {
	Report(snapshot *MetricsSnapshot)
	ReportPeriodic(metrics *MetricsCollector)
	ReportFinal(metrics *MetricsCollector)
}

// MetricsExporter interface for exporting metrics
type MetricsExporter interface {
	ExportJSON(metrics *MetricsCollector) ([]byte, error)
	ExportPrometheus(metrics *MetricsCollector) ([]byte, error)
	ExportCSV(metrics *MetricsCollector) ([]byte, error)
	ExportHTML(metrics *MetricsCollector) ([]byte, error)
}

// LatencyTracker efficiently tracks latency percentiles
type LatencyTracker struct {
	mu         sync.RWMutex
	reservoir  *ReservoirSample
	sorted     bool
}

// ReservoirSample implements reservoir sampling for percentiles
type ReservoirSample struct {
	samples    []time.Duration
	maxSize    int
	count      uint64
}

// CircularBuffer for recent error tracking
type CircularBuffer struct {
	mu       sync.RWMutex
	buffer   []interface{}
	size     int
	head     int
	tail     int
	count    int
}

