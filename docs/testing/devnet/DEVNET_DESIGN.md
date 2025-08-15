# DevNet Design and Architecture

## Executive Summary

The Accumulate DevNet is a sophisticated local development and testing environment that simulates a complete blockchain network with configurable topology. It supports flexible partition (BVN) and validator configurations, enabling comprehensive testing of network behaviors including cross-chain messaging, consensus, and failure recovery mechanisms.

## Architecture Overview

### Core Components

```
┌──────────────────────────────────────────────────────────┐
│                     DevNet Orchestrator                   │
│  (devnet_config.sh / accumulated run devnet)             │
└────────────┬─────────────────────────────────────────────┘
             │
             ├─── Directory Network (DN)
             │    ├── Validators (configurable 1-N)
             │    └── Followers (configurable 0-N)
             │
             ├─── Block Validator Network 0 (BVN0)
             │    ├── Validators (configurable 1-N)
             │    └── Followers (configurable 0-N)
             │
             ├─── Block Validator Network 1 (BVN1)
             │    ├── Validators (configurable 1-N)
             │    └── Followers (configurable 0-N)
             │
             └─── ... (up to N BVNs)
```

### Network Topology

1. **Directory Network (DN)**
   - Central identity and routing registry
   - Maintains global state about all partitions
   - Handles account creation and routing decisions
   - Single logical network, can have multiple validators

2. **Block Validator Networks (BVNs)**
   - Independent blockchain partitions
   - Process transactions for accounts assigned to them
   - Communicate via CrossChain Conductor (CCC)
   - Each BVN maintains its own consensus

3. **CrossChain Conductor (CCC)**
   - Embedded in each validator node
   - Manages cross-partition message delivery
   - Implements gap recovery via simple index-based mechanism
   - Supports pause/resume for testing (with testnet build tag)

## Configuration System

### Flexible Configuration Parameters

The DevNet supports dynamic configuration through multiple mechanisms:

#### Command-Line Parameters
```bash
go run ./cmd/accumulated run devnet \
  --bvns <number>        # Number of BVN partitions
  --validators <number>  # Validators per partition
  --followers <number>   # Followers per partition
  --port <base_port>     # Starting port number
  --work-dir <path>      # Data directory
  --name <network_name>  # Network identifier
  --reset               # Clean state before starting
```

#### Configuration Files
```bash
# devnet_configs/example.conf
BVNS=3
VALIDATORS=3
FOLLOWERS=1
BASE_PORT=26656
```

#### Pre-defined Profiles
- **minimal**: 2 BVNs, 1 validator, 0 followers (quick testing)
- **standard**: 2 BVNs, 3 validators, 1 follower (balanced)
- **large**: 3 BVNs, 3 validators, 2 followers (stress testing)
- **multi**: 5 BVNs, 2 validators, 1 follower (cross-chain testing)
- **stress**: 5 BVNs, 3 validators, 2 followers (maximum load)

### Port Allocation Strategy

Each node requires 4 ports for different services:

```
Node Port Allocation:
├── RPC Port     (base + 0)  - RPC API endpoint
├── P2P Port     (base + 1)  - Peer-to-peer communication
├── Metrics Port (base + 2)  - Prometheus metrics
└── API Port     (base + 3)  - JSON-RPC v2 API

Example for 3 BVNs with 2 validators, 1 follower:
DN:
  Node 0: 26656-26659
  Node 1: 26660-26663
  Node 2: 26664-26667
BVN0:
  Node 0: 26668-26671
  Node 1: 26672-26675
  Node 2: 26676-26679
BVN1:
  Node 0: 26680-26683
  ...
```

Total ports required: `(validators + followers) × (1 + BVNs) × 4`

## Process Management

### Initialization Flow

```mermaid
graph TD
    A[Start DevNet] --> B[Kill Existing Processes]
    B --> C[Clean Data Directory]
    C --> D[Calculate Resource Requirements]
    D --> E[Initialize Directory Network]
    E --> F[Initialize BVN Partitions]
    F --> G[Start Validator Nodes]
    G --> H[Start Follower Nodes]
    H --> I[Wait for Network Ready]
    I --> J[Verify All Partitions Active]
```

### Health Monitoring

The DevNet manager continuously monitors:
1. **Process Health**: PID tracking and liveness checks
2. **API Availability**: HTTP endpoint responsiveness
3. **Network Formation**: Peer discovery and consensus
4. **Partition Communication**: Cross-chain message flow

## CrossChain Conductor Integration

### Gap Recovery Mechanism

The CCC implements a simple yet effective gap recovery system:

#### Core Design
```go
type DestinationSendState struct {
    Destination    *url.URL
    SentTxIndex    uint64  // Last successfully sent sequence
    CurrentTxIndex uint64  // Latest available sequence
    Messages       map[uint64]messaging.Message
}
```

#### Recovery Flow
1. **Normal Operation**: Advance SentTxIndex on successful sends
2. **Failure Handling**: Keep SentTxIndex unchanged on failure
3. **Gap Detection**: Destination reports LastKnownSequence < SentTxIndex
4. **Recovery**: Reset SentTxIndex to LastKnownSequence
5. **Batch Send**: Next send includes all messages from reset point

### Testing Features

#### Pause/Resume Capability (testnet build)
```bash
# Build with testnet features
go build -tags testnet -o accumulated ./cmd/accumulated

# HTTP endpoints for control
curl -X POST http://localhost:27010/debug/ccc/pause   # Pause partition
curl -X POST http://localhost:27010/debug/ccc/resume  # Resume partition
curl http://localhost:27010/debug/ccc/status         # Check status
```

#### Interactive Testing Interface
```bash
./interactive_pause_test.sh
# Menu options:
# - Pause/resume individual partitions
# - Monitor gap detection
# - Run test transactions
# - View real-time logs
```

## Resource Management

### Memory Requirements

| Component | Memory per Node | Calculation |
|-----------|----------------|-------------|
| Validator | 200-300 MB | Active consensus, full state |
| Follower | 150-200 MB | Passive sync, read-only state |
| Base overhead | 500 MB | Shared libraries, runtime |

Example: 3 BVNs, 3 validators, 1 follower each
- Total nodes: 16 (4 DN + 12 BVN)
- Memory: ~3-4 GB

### CPU Requirements

- **Consensus**: 1 core per 2-3 validators
- **Transaction Processing**: Scales with load
- **Cross-chain Messages**: Minimal overhead with CCC

### Storage Requirements

- **Per Node**: 50-100 MB base
- **Transaction History**: ~1 KB per transaction
- **State Growth**: Linear with account creation

## Testing Capabilities

### Supported Test Scenarios

1. **Consensus Testing**
   - Byzantine fault tolerance
   - Leader election
   - Network partitioning

2. **Cross-Chain Testing**
   - Message delivery guarantees
   - Gap recovery mechanisms
   - Ordering preservation

3. **Performance Testing**
   - Transaction throughput
   - Latency measurements
   - Resource utilization

4. **Failure Recovery Testing**
   - Node failures
   - Network partitions
   - State synchronization

### Load Testing Integration

```bash
# Automatic adaptation to network topology
./devnet_load_test.sh
# - Detects partition count
# - Distributes load across BVNs
# - Measures cross-partition performance
# - Reports metrics per partition
```

## Monitoring and Debugging

### Log Aggregation

```bash
# Centralized logging
devnet_config.log        # Orchestrator logs
devnet.log              # Node execution logs
devnet_load_test.log    # Test execution logs

# Partition-specific filtering
grep "BVN1" devnet.log | tail -20
grep "gap.*recovery" devnet.log
```

### Metrics Collection

```bash
# Prometheus metrics endpoints
http://localhost:26658/metrics  # DN metrics
http://localhost:26670/metrics  # BVN0 metrics
http://localhost:26682/metrics  # BVN1 metrics

# Custom CCC metrics
curl http://localhost:27010/debug/ccc/metrics
{
  "destinations": {
    "BVN1": {
      "sent_tx_index": 1000,
      "current_tx_index": 1050,
      "gap_size": 50,
      "total_sent": 950,
      "total_failed": 5,
      "gap_resets": 2
    }
  }
}
```

### Debug Endpoints

```bash
# Network status
curl http://localhost:26660/v3/network/status

# Partition health
curl http://localhost:26660/v3/partitions

# Transaction tracing
curl http://localhost:26660/debug/tx/<txid>
```

## Automation and CI/CD

### GitHub Actions Integration

```yaml
name: DevNet Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
      
      - name: Start DevNet
        run: ./devnet_config.sh start 3 2 1
        
      - name: Wait for Network
        run: ./devnet_config.sh status
        
      - name: Run Tests
        run: ./devnet_load_test.sh
        
      - name: Cleanup
        run: ./devnet_config.sh clean
        if: always()
```

### Docker Deployment

```dockerfile
FROM golang:1.19
WORKDIR /accumulate
COPY . .
RUN go build -o accumulated ./cmd/accumulated
EXPOSE 26656-26700
CMD ["./accumulated", "run", "devnet", "--bvns", "3"]
```

## Security Considerations

### Development-Only Features

⚠️ **WARNING**: DevNet includes features NOT suitable for production:

1. **Simplified Consensus**: Reduced Byzantine fault tolerance
2. **Debug Endpoints**: Unrestricted access to internal state
3. **Test Keys**: Pre-generated, publicly known keys
4. **Pause/Resume**: Network control endpoints (testnet build)
5. **Reduced Validation**: Faster but less secure

### Network Isolation

DevNet binds to localhost by default:
- No external network exposure
- Suitable for development environments
- Use firewall rules if binding to public IPs

## Performance Optimization

### Fast Iteration Mode

```bash
# Minimal setup for development
./devnet_config.sh start 2 1 0
# - Quick startup (< 5 seconds)
# - Low memory (< 1 GB)
# - Suitable for unit testing
```

### Production Simulation Mode

```bash
# Realistic network behavior
./devnet_config.sh start 3 5 2
# - Full consensus simulation
# - Realistic latencies
# - Production-like resource usage
```

### Stress Testing Mode

```bash
# Maximum load testing
./devnet_config.sh start 5 3 2
# - High partition count
# - Maximum validator sets
# - Resource limits testing
```

## Future Enhancements

### Planned Features

1. **Dynamic Scaling**
   - Add/remove BVNs at runtime
   - Hot validator addition/removal
   - Automatic load balancing

2. **Advanced Testing**
   - Chaos engineering integration
   - Network delay injection
   - Byzantine behavior simulation

3. **Observability**
   - Grafana dashboard templates
   - Distributed tracing
   - Performance profiling integration

4. **Deployment Options**
   - Kubernetes operators
   - Terraform modules
   - Cloud-native deployments

## Troubleshooting Guide

### Common Issues and Solutions

| Issue | Symptom | Solution |
|-------|---------|----------|
| Port conflicts | "bind: address already in use" | Change BASE_PORT or kill existing processes |
| Memory exhaustion | OOM errors, slow performance | Reduce validators/followers count |
| Slow startup | Network not ready after 60s | Increase wait timeout, check system resources |
| Gap recovery not working | Messages not delivered | Verify testnet build, check CCC logs |
| API not responding | Connection refused | Wait for full startup, check process health |

### Debug Commands

```bash
# Check all running nodes
ps aux | grep accumulated

# View port usage
lsof -i :26656-26700

# Monitor resource usage
top -p $(pgrep -d, accumulated)

# Clean restart
./devnet_config.sh clean
./devnet_config.sh start 2 3 1
```

## Web Dashboard Integration

### Overview

The DevNet includes an integrated web-based dashboard that provides real-time monitoring and control capabilities for all network partitions and cross-chain communications. The dashboard serves as a comprehensive visualization and testing tool for developers working with the DevNet.

### Dashboard Architecture

The dashboard is implemented as a Go web server that integrates seamlessly with the DevNet lifecycle:

```
┌─────────────────────────────────────────────────────┐
│                 DevNet Process                      │
├─────────────────────────────────────────────────────┤
│  ┌─────────────────┐    ┌──────────────────────────┐ │
│  │   Partition     │    │     Web Dashboard        │ │
│  │   Nodes         │◄───┤     (Go HTTP Server)     │ │
│  │   DN, BVN1-N    │    │     Port: 8080           │ │
│  └─────────────────┘    └──────────────────────────┘ │
└─────────────────────────────────────────────────────┘
                              │
                              ▼
                     Browser Interface
                    http://localhost:8080
```

### Core Dashboard Features

#### 1. Partition Status Monitoring
- **Real-time Block Heights**: Live display of current block height for each partition (DN, BVN1, BVN2, etc.)
- **Partition Health**: Visual indicators showing partition status (active, paused, error)
- **Validator Information**: Display of validator count and base port for each partition
- **Auto-refresh**: Updates every 2 seconds for real-time monitoring

#### 2. CrossChain Conductor (CCC) Monitoring
The dashboard provides comprehensive monitoring of cross-chain message flow:

**Anchor Exchange Tracking**:
- **DN ↔ BVN Communications**: Monitors bidirectional anchor exchange between Directory Network and each Block Validator Network
- **Source Heights**: Displays the latest anchor produced by source partition
- **Destination Heights**: Shows the latest anchor received by destination partition
- **Gap Analysis**: Real-time calculation and display of height differences (gaps)
- **Status Indicators**: Visual status showing normal, behind, or critical gap states

**Important**: BVNs do not exchange anchors with other BVNs - all anchor communication flows through the DN as the central hub.

#### 3. Partition Control Interface
**Pause/Resume Functionality**:
- **Individual Partition Control**: Ability to pause/resume CrossChain Conductor on specific partitions
- **Testing Support**: Enables controlled testing of gap recovery mechanisms
- **Visual State Management**: Clear indication of paused vs. active partitions
- **Gap Recovery Testing**: Allows partitions to fall behind, then resume to test catch-up mechanisms

#### 4. Responsive Design Requirements
The dashboard must provide optimal viewing across different screen sizes and orientations:

**Vertical Scaling**:
- Partition cards stack vertically on narrow screens
- Crosschain table becomes scrollable on small heights
- Maintains readability at minimum 320px width
- Supports unlimited vertical content expansion

**Horizontal Scaling**:
- Partition cards arrange in responsive grid (1-4 columns based on screen width)
- Crosschain table expands to use full available width
- Content centers within maximum container width (1400px)
- Supports ultra-wide displays without content stretching

**Responsive Breakpoints**:
- Mobile: 320px-768px (single column layout)
- Tablet: 768px-1024px (dual column layout)
- Desktop: 1024px-1400px (triple/quad column layout)
- Ultra-wide: >1400px (quad+ column with centered container)

#### 5. Technical Implementation Requirements

**Data Collection**:
```go
type PartitionInfo struct {
    ID             string `json:"id"`           // DN, BVN1, BVN2, etc.
    Height         uint64 `json:"height"`       // Current block height
    Type           string `json:"type"`         // "directory" or "bvn"
    IsPaused       bool   `json:"isPaused"`     // CCC pause state
    BasePort       int    `json:"basePort"`     // API endpoint port
    ValidatorCount int    `json:"validatorCount"` // Number of validators
}

type CrosschainInfo struct {
    Source       string `json:"source"`       // Source partition ID
    Destination  string `json:"destination"`  // Destination partition ID
    Type         string `json:"type"`         // Always "anchor"
    SourceHeight uint64 `json:"sourceHeight"` // Anchors produced
    DestHeight   uint64 `json:"destHeight"`   // Anchors received
}
```

**API Endpoints**:
- `GET /`: Dashboard HTML interface
- `GET /api/data`: JSON data feed for partition and crosschain status
- `POST /api/pause`: Pause/resume control for individual partitions

**Query Mechanism**:
- Queries partition ledgers via JSON-RPC v3: `acc://{partition}/ledger`
- Queries anchor pools via: `acc://{source}/anchors/{destination}`
- Supports multiple API ports for partition discovery
- Graceful handling of partition startup/shutdown states

### Integration with DevNet Lifecycle

**Automatic Launch**:
- Dashboard automatically starts when DevNet is launched
- Runs on configurable port (default: 8080)
- Auto-opens browser tab pointing to dashboard
- Integrated process management with DevNet cleanup

**Development Workflow**:
1. Developer runs: `go run ./cmd/accumulated run devnet`
2. DevNet initializes all partitions
3. Dashboard server starts automatically
4. Browser opens to `http://localhost:8080`
5. Developer sees real-time partition status and crosschain flow
6. Developer can pause/resume partitions for testing
7. Dashboard updates in real-time as network processes transactions

### Gap Recovery Testing Workflow

The dashboard enables comprehensive testing of the CrossChain Conductor's gap recovery mechanism:

1. **Normal Operation**: All partitions show synchronized anchor heights
2. **Induce Gap**: Pause CCC on target partition (e.g., BVN1)
3. **Monitor Gap Growth**: Watch DN→BVN1 destination height lag behind source
4. **Resume Operation**: Unpause CCC on BVN1
5. **Observe Recovery**: Monitor rapid catch-up as gap closes
6. **Validate Success**: Confirm heights re-synchronize

### Dashboard CSS Architecture

**Responsive Grid System**:
```css
.grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: 20px;
}

@media (max-width: 768px) {
    .grid {
        grid-template-columns: 1fr;
    }
}

@media (min-width: 1400px) {
    .container {
        max-width: 1400px;
        margin: 0 auto;
    }
}
```

**Visual Design Principles**:
- Modern glass-morphism aesthetic with backdrop blur effects
- Blue gradient background for professional appearance
- Card-based layout for partition information
- Color-coded status indicators (green=OK, orange=behind, red=critical)
- Hover effects and smooth transitions for interactivity

### Performance and Scalability

**Update Frequency**: 2-second polling interval balances responsiveness with resource usage
**Concurrent Access**: Supports multiple browser sessions viewing same dashboard
**Memory Efficiency**: Minimal state storage, data refreshed on each request
**Network Efficiency**: Lightweight JSON payloads, client-side rendering

## Load Generator Integration

### Overview

The DevNet includes an integrated load generator that creates continuous transaction activity for testing and visualization purposes. This allows developers to observe network behavior under load and verify transaction processing, consensus, and cross-chain message flow.

### Load Generator Architecture

The load generator runs as a separate process that interacts with the DevNet through its API endpoints:

```
┌─────────────────────────────────────────────────────┐
│                 DevNet Ecosystem                     │
├─────────────────────────────────────────────────────┤
│  ┌──────────────┐    ┌─────────────┐    ┌─────────┐│
│  │   DevNet     │◄───┤    Load     │───►│Dashboard││
│  │   Nodes      │    │  Generator  │    │ Monitor ││
│  │              │    │             │    │         ││
│  └──────────────┘    └─────────────┘    └─────────┘│
└─────────────────────────────────────────────────────┘
```

### Core Load Generation Features

#### 1. Continuous Transaction Generation
- **Faucet Operations**: Continuous creation of lite accounts and faucet requests
- **Token Transfers**: Automated ACME token transfers between accounts
- **ADI Operations**: Identity creation and management transactions
- **Data Transactions**: Simulated data entry operations

#### 2. Load Patterns
- **Steady Load**: Constant rate of transactions per second
- **Burst Load**: Periodic spikes in transaction volume
- **Ramp Load**: Gradually increasing transaction rate
- **Chaos Load**: Random variations in transaction patterns

#### 3. Partition Distribution
- Transactions distributed across all BVNs
- Cross-partition operations to test anchor flow
- Balanced load to prevent single partition bottlenecks

### Integration with DevNet

**Automatic Launch Options**:
- Load generator can be launched with DevNet via flag
- Configurable transaction rate and patterns
- Graceful shutdown with DevNet termination

**Manual Operation**:
- Can be started/stopped independently
- Useful for targeted testing scenarios
- Multiple instances for stress testing

### Monitoring and Metrics

Load generator activity is visible through:
- Dashboard transaction counters
- Block height progression
- Anchor exchange rates
- Network throughput metrics

## Summary

The DevNet design provides a comprehensive, flexible, and powerful testing environment for Accumulate development:

### Key Strengths
- **Flexible Configuration**: Easily adjust network topology
- **Integrated Testing**: Built-in support for various test scenarios
- **Gap Recovery**: Simple, effective cross-chain message recovery
- **Resource Efficient**: Scales from minimal to stress configurations
- **Developer Friendly**: Quick iteration, comprehensive debugging

### Use Cases
- **Development**: Fast local testing environment
- **Integration Testing**: Multi-partition transaction flows
- **Performance Testing**: Throughput and latency measurements
- **Failure Testing**: Network partition and recovery scenarios
- **CI/CD**: Automated testing in pipelines

The DevNet architecture successfully balances simplicity with capability, providing developers with a powerful tool for building and testing distributed applications on Accumulate.