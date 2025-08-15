# Simplified Load Generation Design

## Overview
Minimal load generator that creates ADIs, token accounts, and sends tokens between them. Built with extensible structure for easy enhancement.

## Core Architecture

```
┌──────────────────────────────────────┐
│         Load Generator                │
├──────────┬───────────┬───────────────┤
│  Wallet  │ Generator │    Runner     │
├──────────┴───────────┴───────────────┤
│         Simple & Extensible           │
└───────────────────────────────────────┘
```

## Component Design

### 1. Wallet (wallet.go)

Simple account tracking with minimal state management.

```go
type Wallet struct {
    ADIs          map[string]*ADIAccount
    LiteAccounts  map[string]*LiteAccount
    NextADIIndex  int
}

type ADIAccount struct {
    URL           string
    KeyPageURL    string
    TokenAccounts []string
    Key           *Key
}

type LiteAccount struct {
    URL     string
    Key     *Key
    Balance uint64  // Optimistic balance
}

type Key struct {
    PublicKey  []byte
    PrivateKey []byte
}
```

**Core Methods:**
- `GetOrCreateADI()` - Returns existing or creates new ADI
- `GetFundedAccount()` - Returns account with balance
- `UpdateBalance()` - Simple balance tracking

### 2. Transaction Generator (generator.go)

Minimal transaction creation focused on essentials.

```go
type Generator struct {
    wallet *Wallet
    client Client
}

type Transaction interface {
    Build() (*protocol.Transaction, error)
    Type() string
}
```

**Transaction Types (Phase 1):**
1. **CreateADI** - Create ADI with key page
2. **CreateTokenAccount** - Add token account to ADI  
3. **SendTokens** - Transfer between accounts

```go
// Simple transaction selection
func (g *Generator) NextTransaction() Transaction {
    r := rand.Float64()
    
    if r < 0.1 {  // 10% - Create infrastructure
        if rand.Float64() < 0.5 {
            return g.buildCreateADI()
        }
        return g.buildCreateTokenAccount()
    }
    
    // 90% - Send tokens
    return g.buildSendTokens()
}
```

### 3. Load Runner (runner.go)

Simple execution engine with two modes.

```go
type Runner struct {
    generator *Generator
    mode      Mode
    targetTPS int
}

type Mode int
const (
    ModeBlocking Mode = iota
    ModeNonBlocking
)
```

**Blocking Mode:**
```go
func (r *Runner) runBlocking() {
    for {
        tx := r.generator.NextTransaction()
        
        start := time.Now()
        txID := r.submit(tx)
        r.waitForCompletion(txID)
        
        elapsed := time.Since(start)
        r.recordLatency(elapsed)
        
        r.rateLimit()
    }
}
```

**Non-Blocking Mode:**
```go
func (r *Runner) runNonBlocking() {
    for {
        tx := r.generator.NextTransaction()
        
        r.submit(tx)
        r.recordSubmission()
        
        r.rateLimit()
    }
}
```

## Simplified Transaction Flow

### Create ADI
```
1. Generate unique name (adi-0001, adi-0002, etc.)
2. Create key pair
3. Build CreateIdentity transaction
4. Submit
5. Store in wallet
```

### Create Token Account
```
1. Select existing ADI
2. Generate account name (tokens-1, tokens-2, etc.)
3. Build CreateTokenAccount transaction
4. Submit
5. Add to ADI's account list
```

### Send Tokens
```
1. Find source account with balance
2. Select destination account
3. Pick random amount (1-1000 ACME)
4. Build SendTokens transaction
5. Submit
6. Update optimistic balances
```

## Minimal Metrics

```go
type Metrics struct {
    StartTime        time.Time
    TotalSubmitted   uint64
    TotalSuccessful  uint64
    TotalFailed      uint64
    
    // Simple latency tracking (blocking mode only)
    LatencySum       time.Duration
    LatencyCount     uint64
}

func (m *Metrics) AverageLatency() time.Duration {
    if m.LatencyCount == 0 {
        return 0
    }
    return m.LatencySum / time.Duration(m.LatencyCount)
}

func (m *Metrics) CurrentTPS() float64 {
    elapsed := time.Since(m.StartTime).Seconds()
    return float64(m.TotalSubmitted) / elapsed
}
```

## Simple Rate Control

```go
type RateLimiter struct {
    targetTPS int
    lastTx    time.Time
}

func (r *RateLimiter) Wait() {
    if r.targetTPS <= 0 {
        return // No limit
    }
    
    interval := time.Second / time.Duration(r.targetTPS)
    elapsed := time.Since(r.lastTx)
    
    if elapsed < interval {
        time.Sleep(interval - elapsed)
    }
    
    r.lastTx = time.Now()
}
```

## Minimal Error Handling

```go
// Simple retry with fixed delay
func submitWithRetry(tx Transaction) error {
    for i := 0; i < 3; i++ {
        err := submit(tx)
        if err == nil {
            return nil
        }
        
        time.Sleep(time.Second)
    }
    return fmt.Errorf("failed after 3 attempts")
}

// Skip on permanent errors
if err != nil {
    if isPermanentError(err) {
        log.Printf("Skipping: %v", err)
        continue
    }
    // Otherwise retry
}
```

## Configuration

```yaml
# Minimal configuration
load_generator:
  mode: blocking        # or non-blocking
  target_tps: 10       # 0 for unlimited
  duration: 60s        # How long to run
  
  # Initial setup
  initial_adis: 10     # Pre-create ADIs
  initial_accounts: 2  # Token accounts per ADI
```

## Extension Points

The design is structured for easy enhancement:

### Adding New Transaction Types
```go
// 1. Implement Transaction interface
type WriteData struct {
    account *DataAccount
    data    []byte
}

func (w *WriteData) Build() (*protocol.Transaction, error) {
    // Build transaction
}

func (w *WriteData) Type() string {
    return "write_data"
}

// 2. Add to generator selection
if r < 0.15 {  // New 5% allocation
    return g.buildWriteData()
}
```

### Adding Metrics
```go
// Extend Metrics struct
type Metrics struct {
    // ... existing fields ...
    
    // New metrics
    TypeCounts map[string]uint64
    Latencies  []time.Duration  // For percentiles
}

// Add collection points
func (m *Metrics) RecordTransaction(txType string, latency time.Duration) {
    m.TypeCounts[txType]++
    m.Latencies = append(m.Latencies, latency)
}
```

### Adding Error Categories
```go
// Extend error handling
type ErrorTracker struct {
    NetworkErrors   uint64
    BalanceErrors   uint64
    ValidationErrors uint64
}

func categorizeError(err error) {
    // Simple string matching
    if strings.Contains(err.Error(), "timeout") {
        tracker.NetworkErrors++
    }
}
```

## File Structure

```
loadgen/
├── wallet.go          # Account management
├── generator.go       # Transaction creation
├── runner.go          # Execution engine
├── metrics.go         # Simple metrics
├── config.go          # Configuration
└── main.go           # Entry point
```

## Usage Example

```go
func main() {
    // Load config
    cfg := LoadConfig("config.yaml")
    
    // Initialize
    wallet := NewWallet()
    generator := NewGenerator(wallet, client)
    runner := NewRunner(generator, cfg.Mode, cfg.TargetTPS)
    
    // Pre-create some accounts
    for i := 0; i < cfg.InitialADIs; i++ {
        generator.CreateADI()
    }
    
    // Run load test
    runner.Run(cfg.Duration)
    
    // Print results
    runner.PrintMetrics()
}
```

## Key Simplifications

### What We Keep
- ADI creation
- Token account creation
- Token transfers
- Two modes (blocking/non-blocking)
- Basic metrics (TPS, latency)
- Simple wallet tracking

### What We Remove (Phase 1)
- Complex error recovery
- Detailed metrics percentiles
- Transaction mix configuration
- Worker pools
- Circuit breakers
- Adaptive rate control
- Cross-partition tracking
- Resource monitoring
- Queue management

### What's Easy to Add Later
- More transaction types (plug into generator)
- Better metrics (extend Metrics struct)
- Worker pools (wrap runner.Run)
- Better error handling (extend retry logic)
- Configuration options (extend config struct)

## Performance Expectations

### Blocking Mode
- **TPS**: 1-50 (limited by verification)
- **Accuracy**: High (verified balances)
- **Complexity**: Low

### Non-Blocking Mode
- **TPS**: 100-1000 (limited by client)
- **Accuracy**: Medium (optimistic)
- **Complexity**: Very Low

## Summary

This simplified design provides:
1. **Core functionality**: ADIs, token accounts, transfers
2. **Two clear modes**: Blocking and non-blocking
3. **Extensible structure**: Easy to add features
4. **Minimal complexity**: ~500 lines of code total
5. **Clear separation**: Wallet, Generator, Runner

The structure supports growth without refactoring - just add new transaction types to the generator, new fields to metrics, or new features to the runner.