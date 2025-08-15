# Load Generator Metrics Specification

## Overview
Comprehensive metrics tracking for load generator performance analysis, system health monitoring, and transaction pattern observation.

## Core Time Metrics

### Session Metrics
- **Start Time**: Timestamp when load generation begins
- **End Time**: Timestamp when load generation completes
- **Total Duration**: End time - Start time
- **Active Duration**: Time actually generating load (excludes pauses)
- **Pause Duration**: Total time spent paused
- **Ramp-Up Duration**: Time to reach target TPS
- **Ramp-Down Duration**: Time to gracefully stop

### Window Metrics
- **Current Window Start**: Beginning of current measurement window
- **Window Duration**: Size of measurement window (default: 1 minute)
- **Windows Completed**: Number of full windows measured
- **Current Window Progress**: Percentage of current window complete

## Transaction Volume Metrics

### Absolute Counts
- **Total Attempted**: All transactions initiated
- **Total Submitted**: Transactions sent to network
- **Total Successful**: Transactions confirmed successful
- **Total Failed**: Transactions that failed
- **Total Pending**: Currently in-flight transactions
- **Total Timed Out**: Transactions exceeding timeout
- **Total Retried**: Transactions that required retry

### Rate Metrics
- **Current TPS**: Transactions per second (last window)
- **Average TPS**: Mean TPS over entire session
- **Peak TPS**: Maximum TPS achieved
- **Minimum TPS**: Minimum TPS (excluding ramp)
- **Target TPS**: Configured target rate
- **TPS Variance**: Standard deviation of TPS

### Window-Based Counts
- **Last Minute Transactions**: Count in last 60 seconds
- **Last 5 Minutes Transactions**: Count in last 5 minutes
- **Last Hour Transactions**: Count in last hour
- **Current Window Transactions**: Count in current window

## Transaction Mix Metrics

### By Type Distribution
- **Infrastructure Percentage**: ADI/account creation transactions
- **Value Transfer Percentage**: Token movement transactions
- **Data Operations Percentage**: Data write transactions
- **Token Issuance Percentage**: Token creation/minting

### Detailed Type Breakdown
For each transaction type:
- **Count**: Number attempted
- **Success Count**: Number successful
- **Failure Count**: Number failed
- **Success Rate**: Percentage successful
- **Average Latency**: Mean completion time
- **P50 Latency**: Median completion time
- **P95 Latency**: 95th percentile latency
- **P99 Latency**: 99th percentile latency
- **Percentage of Total**: Proportion of all transactions

## Partition Metrics

### Per-Partition Tracking
For each partition in the network:
- **Transaction Count**: Total transactions to partition
- **Success Count**: Successful transactions
- **Failure Count**: Failed transactions
- **Current TPS**: Current rate for partition
- **Peak TPS**: Maximum rate achieved
- **Average Latency**: Mean transaction time
- **P95 Latency**: 95th percentile latency
- **P99 Latency**: 99th percentile latency
- **Queue Depth**: Current pending for partition
- **Active Connections**: Open connections to partition
- **Error Count**: Total errors for partition
- **Error Rate**: Errors per second
- **Accounts Created**: New accounts in partition
- **Active Accounts**: Accounts with recent activity
- **Outbound Cross-Partition**: Transactions leaving partition
- **Inbound Cross-Partition**: Transactions entering partition

### Partition Distribution
- **Total Partitions**: Number of partitions
- **Active Partitions**: Partitions with activity
- **Partition Balance**: Evenness of load distribution (Gini coefficient)
- **Hottest Partition**: Most active partition ID
- **Coldest Partition**: Least active partition ID
- **Load Imbalance**: Percentage difference hot/cold

### Cross-Partition Metrics
- **Total Cross-Partition**: Count crossing partitions
- **Cross-Partition Percentage**: Proportion of all transactions
- **Cross-Partition Success Rate**: Success rate
- **Average Cross-Partition Latency**: Mean processing time
- **Cross-Partition Overhead**: Additional latency vs same-partition
- **Throughput Impact**: TPS reduction percentage

### Cross-Partition Routes
For each source→destination partition pair:
- **Route Count**: Transactions on this route
- **Route Success Rate**: Success percentage
- **Average Route Latency**: Mean time for route
- **P95 Route Latency**: 95th percentile
- **P99 Route Latency**: 99th percentile
- **Queued Count**: Waiting transactions
- **Dropped Count**: Transactions dropped
- **Retry Count**: Retries on route
- **Last Used**: Most recent transaction
- **Peak Hour**: Busiest hour for route
- **Peak Load**: Maximum transactions/hour

### Synthetic Transaction Metrics
- **Synthetic Count**: Total synthetic transactions
- **Synthetic Latency**: Average synthetic processing
- **Synthetic Success**: Successful synthetics
- **Synthetic Failure**: Failed synthetics
- **Synthetic Percentage**: Proportion of cross-partition

### Anchor Transaction Metrics
- **Anchor Count**: Total anchor transactions
- **Anchor Latency**: Average anchor processing
- **Anchor Success**: Successful anchors
- **Anchor Failure**: Failed anchors
- **Anchor Percentage**: Proportion of cross-partition

## Performance Metrics

### Latency Types
Two distinct latency measurements are tracked:

#### Submission Latency
Time to submit transaction to network and receive acknowledgment:
- **Minimum Submission**: Fastest submission
- **Maximum Submission**: Slowest submission
- **Average Submission**: Mean submission time
- **P95 Submission**: 95th percentile submission

#### End-to-End Processing Latency (Latency Mode)
Time from submission until transaction effects are readable (balance updated):
- **Minimum Processing**: Fastest complete processing
- **Maximum Processing**: Slowest complete processing
- **Average Processing**: Mean processing time
- **Median Processing**: P50 processing time
- **P75 Processing**: 75th percentile
- **P90 Processing**: 90th percentile
- **P95 Processing**: 95th percentile
- **P99 Processing**: 99th percentile
- **P99.9 Processing**: 99.9th percentile
- **Processing Deviation**: Processing time variance

#### Latency Mode Operation
In latency mode, each transaction follows this measurement flow:
1. **T0**: Transaction submitted to network
2. **T1**: Submission acknowledged (submission latency = T1-T0)
3. **Polling Phase**: Destination account polled repeatedly
4. **T2**: Balance change detected at destination
5. **End-to-End Latency**: T2-T0 (complete processing time)

#### Polling Metrics
- **Average Poll Count**: Polls required to detect change
- **Poll Interval**: Time between polls (configurable)
- **Detection Time**: Time from last poll to detection
- **False Positive Rate**: Incorrect balance detections
- **Poll Timeout Rate**: Transactions never detected

### Throughput Metrics
- **Bytes Sent**: Total data transmitted
- **Bytes Received**: Total data received
- **Average Request Size**: Mean transaction size
- **Average Response Size**: Mean response size
- **Bandwidth Utilization**: Current network usage
- **Peak Bandwidth**: Maximum bandwidth used

### Queue Metrics
- **Queue Depth**: Current pending transactions
- **Average Queue Depth**: Mean queue size
- **Maximum Queue Depth**: Peak queue size
- **Queue Wait Time**: Average time in queue
- **Queue Overflow Count**: Transactions rejected due to full queue

## Error Metrics

### Error Categories
- **Network Errors**: Connection/timeout failures
- **Validation Errors**: Invalid transaction errors
- **Authority Errors**: Permission/signature failures
- **Balance Errors**: Insufficient funds errors
- **State Errors**: Invalid state transitions
- **Unknown Errors**: Uncategorized failures

### Error Tracking
- **Total Error Count**: All errors encountered
- **Error Rate**: Errors per second
- **Error Percentage**: Proportion of transactions failing
- **Consecutive Errors**: Current error streak
- **Maximum Error Streak**: Longest consecutive errors
- **Time Since Last Error**: Duration since last failure
- **Most Common Error**: Most frequent error type
- **Error Distribution**: Count by error category

### Recovery Metrics
- **Retry Success Count**: Successful after retry
- **Retry Failure Count**: Failed after all retries
- **Average Retry Count**: Mean retries per transaction
- **Recovery Rate**: Percentage recovered via retry
- **Circuit Breaker Trips**: Times circuit opened
- **Circuit Breaker Recovery Time**: Average time to close

## Resource Metrics

### Worker Pool
- **Active Workers**: Currently processing
- **Idle Workers**: Available workers
- **Worker Utilization**: Percentage active
- **Average Worker Load**: Transactions per worker
- **Worker Creation Rate**: New workers spawned
- **Worker Termination Rate**: Workers stopped

### Memory Usage
- **Current Memory**: Current RAM usage
- **Peak Memory**: Maximum RAM used
- **Average Memory**: Mean RAM usage
- **Memory Growth Rate**: Rate of increase
- **GC Count**: Garbage collections
- **GC Pause Time**: Average GC duration

### CPU Usage
- **Current CPU**: Current utilization
- **Average CPU**: Mean utilization
- **Peak CPU**: Maximum utilization
- **CPU per Transaction**: Processing cost

## Network Metrics

### Connection Pool
- **Active Connections**: Currently in use
- **Idle Connections**: Available connections
- **Connection Creation Rate**: New connections/second
- **Connection Reuse Rate**: Percentage reused
- **Connection Errors**: Failed connections
- **Average Connection Lifetime**: Mean duration

### Network Health
- **Average RTT**: Round-trip time
- **Packet Loss Rate**: Lost packets percentage
- **Connection Timeouts**: Timeout count
- **DNS Resolution Time**: Average lookup time
- **TLS Handshake Time**: Average SSL setup

## Entity Creation Metrics

### Account Metrics
- **ADIs Created**: Total ADI count
- **Lite Accounts Created**: Total lite accounts
- **Token Accounts Created**: Token account count
- **Data Accounts Created**: Data account count
- **Average Accounts per ADI**: Mean account count
- **Account Creation Rate**: Accounts per second

### Key Management Metrics
- **Key Books Created**: Total key books
- **Key Pages Created**: Total key pages
- **Keys Added**: Total keys registered
- **Multi-Sig Configurations**: Multi-sig setups
- **Average Keys per Page**: Mean key count

### Token Metrics
- **Custom Tokens Created**: New token types
- **Total Tokens Issued**: Tokens minted
- **Total Tokens Burned**: Tokens destroyed
- **Token Accounts Locked**: Locked accounts
- **Average Token Velocity**: Transfer rate

## Balance Metrics

### ACME Tracking
- **Total ACME Moved**: Sum of transfers
- **Average Transfer Amount**: Mean transfer size
- **Largest Transfer**: Maximum single transfer
- **Smallest Transfer**: Minimum transfer
- **Credits Consumed**: Total credits used
- **Credits Remaining**: Available credits

### Distribution Metrics
- **Gini Coefficient**: Wealth concentration
- **Top 10% Holdings**: Wealth in top decile
- **Active Accounts**: Accounts with activity
- **Dormant Accounts**: Inactive accounts
- **Zero Balance Accounts**: Empty accounts

## Time-Series Metrics

### Rolling Windows
- **1-Minute Rolling TPS**: Last 60 seconds
- **5-Minute Rolling TPS**: Last 5 minutes
- **15-Minute Rolling TPS**: Last 15 minutes
- **1-Hour Rolling TPS**: Last hour

### Trend Metrics
- **TPS Trend**: Increasing/decreasing/stable
- **Latency Trend**: Performance direction
- **Error Rate Trend**: Failure trend
- **Queue Depth Trend**: Backlog trend

## Percentile Tracking

### Response Time Percentiles
- **P1**: 1st percentile (fastest)
- **P5**: 5th percentile
- **P10**: 10th percentile
- **P25**: 25th percentile (Q1)
- **P50**: 50th percentile (median)
- **P75**: 75th percentile (Q3)
- **P90**: 90th percentile
- **P95**: 95th percentile
- **P99**: 99th percentile
- **P99.9**: 99.9th percentile
- **P100**: 100th percentile (maximum)

## Latency Mode Configuration

### Mode Selection
- **Fast Mode**: Only track submission latency (high TPS)
- **Latency Mode**: Track end-to-end processing (accurate timing)
- **Hybrid Mode**: Sample percentage for full tracking

### Latency Mode Parameters
- **Poll Interval**: Time between balance checks (default: 100ms)
- **Poll Timeout**: Maximum wait time (default: 30s)
- **Verification Method**: Balance check vs transaction status
- **Batch Polling**: Group polls for efficiency
- **Smart Polling**: Exponential backoff strategy

### Impact on Performance
- **TPS Reduction**: Lower throughput in latency mode
- **Resource Usage**: Higher due to polling overhead
- **Accuracy Improvement**: True end-to-end timing
- **Network Load**: Additional queries for verification

### Latency Mode Metrics
- **Verification Success Rate**: Transactions successfully verified
- **Average Verification Time**: Mean time to confirm
- **Polling Efficiency**: Successful polls / total polls
- **Timeout Rate**: Transactions exceeding poll timeout
- **Verification Backlog**: Pending verifications

## Reporting Formats

### Real-Time Dashboard
Updated every second:
- Current TPS
- Active transactions
- Recent errors
- Queue depth
- Worker status

### Periodic Summary (Every Minute)
- Transactions in period
- Success rate
- Average latency
- Error count
- Transaction mix

### Detailed Report (Every 5 Minutes)
- All metrics with trends
- Percentile distributions
- Error analysis
- Resource utilization
- Network health

### Final Report
Complete session analysis:
- Total statistics
- Performance graphs
- Error summary
- Recommendations
- Bottleneck analysis

## Metric Collection Methods

### Sampling Strategy
- **Full Collection**: Every transaction tracked
- **Sampling Rate**: For high volume (1:N)
- **Reservoir Sampling**: For percentiles
- **Sliding Window**: For rate calculations
- **Exponential Decay**: For weighted averages

### Storage Optimization
- **In-Memory Buffers**: Recent data
- **Compressed History**: Older data
- **Aggregated Summaries**: Period rollups
- **Metric Rotation**: Age-out old data

## Alert Thresholds

### Performance Alerts
- TPS below 80% of target
- Latency P95 > 5 seconds
- Queue depth > 1000
- Error rate > 5%

### System Alerts
- Memory usage > 80%
- CPU usage > 90%
- Worker pool exhausted
- Connection pool exhausted

### Critical Alerts
- Consecutive errors > 10
- Circuit breaker open
- Zero successful transactions
- Network unreachable

## Export Formats

### JSON Metrics
Real-time metrics in JSON format for API consumption

### Prometheus Format
Time-series metrics for Prometheus scraping

### CSV Export
Tabular data for spreadsheet analysis

### Grafana Dashboard
Pre-configured dashboard definitions

### HTML Report
Human-readable report with charts

## Summary
Comprehensive metrics framework providing real-time monitoring, historical analysis, and performance insights for load testing with configurable collection, storage, and reporting mechanisms.