# API v3 WebSocket Client Reference

**Package**: `pkg/api/v3/websocket`  
**Transport**: WebSocket using `github.com/gorilla/websocket`  
**Best For**: Real-time applications, event subscriptions, streaming data

## Overview

The API v3 WebSocket client provides real-time access to the Accumulate network through WebSocket connections. It supports all v3 API service interfaces plus real-time event subscriptions and streaming capabilities.

## Client Creation

```go
import "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3/websocket"

// Create WebSocket client
client, err := websocket.NewClient("wss://mainnet.accumulatenetwork.io/v3/ws")
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

## Key Features

- **Real-time Events**: Subscribe to blockchain events as they happen
- **Streaming Data**: Continuous data streams for transactions, blocks, etc.
- **Full API Access**: All v3 API service interfaces available
- **Concurrent Streams**: Multiple concurrent sub-streams supported
- **Event-driven Architecture**: Designed for reactive applications
- **Connection Management**: Automatic reconnection and error handling

## Service Interfaces

The WebSocket client implements the same service interfaces as the JSON-RPC client:

```go
// All v3 API services available
client.NodeService()        // Node operations
client.NetworkService()     // Network information
client.ConsensusService()   // Consensus operations
client.QueryService()       // Account and transaction queries
client.SubmitService()      // Transaction submission
client.ValidateService()    // Transaction validation
client.MetricsService()     // Performance metrics

// Plus real-time capabilities
client.Subscribe(...)       // Event subscriptions
client.Stream(...)          // Data streaming
```

## Basic API Usage

### Standard Queries
```go
ctx := context.Background()

// Same interface as JSON-RPC client
account, err := client.QueryService().QueryAccount(ctx, &api.AccountQuery{
    Url: "acc://example.acme",
})
if err != nil {
    log.Printf("Query failed: %v", err)
    return
}

log.Printf("Account: %+v", account.Data)
```

### Node and Network Status
```go
// Get node status
nodeStatus, err := client.NodeService().NodeStatus(ctx, &api.NodeStatusRequest{})
if err != nil {
    log.Printf("Node status failed: %v", err)
    return
}

// Get network status  
networkStatus, err := client.NetworkService().NetworkStatus(ctx, &api.NetworkStatusRequest{})
if err != nil {
    log.Printf("Network status failed: %v", err)
    return
}
```

## Real-time Event Subscriptions

### Basic Event Subscription
```go
// Subscribe to account events
subscription, err := client.Subscribe(ctx, &api.EventSubscription{
    Filter: &api.EventFilter{
        Account: "acc://example.acme",
        Types:   []string{"transaction", "state_change"},
    },
})
if err != nil {
    log.Printf("Subscription failed: %v", err)
    return
}
defer subscription.Close()

// Handle events
go func() {
    for event := range subscription.Events() {
        log.Printf("Event: Type=%s, Account=%s, Data=%+v", 
            event.Type, event.Account, event.Data)
    }
}()

// Handle subscription errors
go func() {
    for err := range subscription.Errors() {
        log.Printf("Subscription error: %v", err)
        // Implement reconnection logic here
    }
}()
```

### Transaction Events
```go
// Subscribe to all transaction events
txSubscription, err := client.Subscribe(ctx, &api.EventSubscription{
    Filter: &api.EventFilter{
        Types: []string{"transaction_submitted", "transaction_executed"},
    },
})
if err != nil {
    log.Fatal(err)
}

for event := range txSubscription.Events() {
    switch event.Type {
    case "transaction_submitted":
        log.Printf("Transaction submitted: %x", event.TransactionHash)
    case "transaction_executed":
        log.Printf("Transaction executed: %x, Status: %s", 
            event.TransactionHash, event.Status)
    }
}
```

### Block Events
```go
// Subscribe to new blocks
blockSubscription, err := client.Subscribe(ctx, &api.EventSubscription{
    Filter: &api.EventFilter{
        Types: []string{"block_committed"},
    },
})
if err != nil {
    log.Fatal(err)
}

for event := range blockSubscription.Events() {
    block := event.Data.(*api.BlockData)
    log.Printf("New block: Height=%d, Hash=%x, TxCount=%d",
        block.Height, block.Hash, len(block.Transactions))
}
```

## Data Streaming

### Transaction Stream
```go
// Stream all transactions in real-time
txStream, err := client.StreamTransactions(ctx, &api.TransactionStreamRequest{
    Filter: &api.TransactionFilter{
        // Optional filters
        Account:   "acc://example.acme", // Specific account
        Type:      "token_transfer",     // Specific transaction type
        StartTime: time.Now().Add(-1 * time.Hour), // Historical start
    },
})
if err != nil {
    log.Fatal(err)
}

for tx := range txStream.Transactions() {
    log.Printf("Transaction: Hash=%x, Type=%s, From=%s, To=%s",
        tx.Hash, tx.Type, tx.From, tx.To)
}
```

### Account State Stream
```go
// Stream account state changes
stateStream, err := client.StreamAccountState(ctx, &api.AccountStateStreamRequest{
    Account: "acc://example.acme",
})
if err != nil {
    log.Fatal(err)
}

for state := range stateStream.States() {
    log.Printf("Account state changed: %+v", state)
}
```

### Metrics Stream
```go
// Stream real-time network metrics
metricsStream, err := client.StreamMetrics(ctx, &api.MetricsStreamRequest{
    Interval: 10 * time.Second, // Update frequency
})
if err != nil {
    log.Fatal(err)
}

for metrics := range metricsStream.Metrics() {
    log.Printf("TPS: %.2f, Block Time: %v, Active Validators: %d",
        metrics.TransactionsPerSecond, metrics.AverageBlockTime, metrics.ValidatorCount)
}
```

## Connection Management

### Connection Configuration
```go
// Configure WebSocket connection
client, err := websocket.NewClientWithConfig("wss://mainnet.accumulatenetwork.io/v3/ws", &websocket.Config{
    ReadTimeout:     60 * time.Second,
    WriteTimeout:    10 * time.Second,
    PingInterval:    30 * time.Second,
    MaxMessageSize:  1024 * 1024, // 1MB
    EnableCompression: true,
})
if err != nil {
    log.Fatal(err)
}
```

### Reconnection Handling
```go
func connectWithRetry(url string) *websocket.Client {
    maxRetries := 5
    backoff := time.Second
    
    for i := 0; i < maxRetries; i++ {
        client, err := websocket.NewClient(url)
        if err == nil {
            return client
        }
        
        log.Printf("Connection failed (attempt %d/%d): %v", i+1, maxRetries, err)
        
        if i < maxRetries-1 {
            time.Sleep(backoff)
            backoff *= 2 // Exponential backoff
        }
    }
    
    log.Fatal("Failed to connect after maximum retries")
    return nil
}
```

### Health Monitoring
```go
// Monitor connection health
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        if !client.IsConnected() {
            log.Println("Connection lost, attempting reconnection...")
            // Implement reconnection logic
        }
    }
}()
```

## Advanced Usage Patterns

### Multi-Stream Application
```go
type StreamManager struct {
    client        *websocket.Client
    subscriptions map[string]*api.Subscription
    streams       map[string]interface{}
    mu           sync.RWMutex
}

func (sm *StreamManager) AddTransactionSubscription(accountURL string) error {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    subscription, err := sm.client.Subscribe(context.Background(), &api.EventSubscription{
        Filter: &api.EventFilter{
            Account: accountURL,
            Types:   []string{"transaction"},
        },
    })
    if err != nil {
        return err
    }
    
    sm.subscriptions[accountURL] = subscription
    
    // Handle events
    go func() {
        for event := range subscription.Events() {
            sm.handleTransactionEvent(accountURL, event)
        }
    }()
    
    return nil
}

func (sm *StreamManager) handleTransactionEvent(account string, event *api.Event) {
    log.Printf("Transaction event for %s: %+v", account, event)
    // Process event
}
```

### Event Aggregation
```go
type EventAggregator struct {
    events chan *api.Event
    stats  map[string]int
    mu     sync.RWMutex
}

func (ea *EventAggregator) Start(client *websocket.Client) {
    subscription, err := client.Subscribe(context.Background(), &api.EventSubscription{
        Filter: &api.EventFilter{
            Types: []string{"transaction", "block", "validator_change"},
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    
    go func() {
        for event := range subscription.Events() {
            ea.mu.Lock()
            ea.stats[event.Type]++
            ea.mu.Unlock()
            
            select {
            case ea.events <- event:
            default:
                log.Println("Event buffer full, dropping event")
            }
        }
    }()
}

func (ea *EventAggregator) GetStats() map[string]int {
    ea.mu.RLock()
    defer ea.mu.RUnlock()
    
    stats := make(map[string]int)
    for k, v := range ea.stats {
        stats[k] = v
    }
    return stats
}
```

## Error Handling

### Connection Errors
```go
client, err := websocket.NewClient(url)
if err != nil {
    switch e := err.(type) {
    case *websocket.HandshakeError:
        log.Printf("WebSocket handshake failed: %v", e)
    case *net.OpError:
        log.Printf("Network error: %v", e)
    default:
        log.Printf("Connection error: %v", e)
    }
    return
}
```

### Subscription Errors
```go
subscription, err := client.Subscribe(ctx, filter)
if err != nil {
    log.Printf("Subscription failed: %v", err)
    return
}

// Monitor for subscription errors
go func() {
    for err := range subscription.Errors() {
        switch e := err.(type) {
        case *websocket.CloseError:
            log.Printf("Connection closed: Code=%d, Text=%s", e.Code, e.Text)
            // Implement reconnection
        case *api.Error:
            log.Printf("API Error: %s", e.Message)
        default:
            log.Printf("Subscription error: %v", e)
        }
    }
}()
```

## Performance Considerations

### Buffer Management
```go
// Configure event buffer sizes
subscription, err := client.Subscribe(ctx, &api.EventSubscription{
    Filter:     filter,
    BufferSize: 1000, // Event buffer size
})
```

### Resource Cleanup
```go
// Proper cleanup
defer func() {
    // Close all subscriptions
    for _, sub := range subscriptions {
        sub.Close()
    }
    
    // Close client connection
    client.Close()
}()
```

### Memory Management
```go
// Limit concurrent subscriptions
const maxSubscriptions = 10

type SubscriptionManager struct {
    subscriptions []*api.Subscription
    semaphore     chan struct{}
}

func NewSubscriptionManager() *SubscriptionManager {
    return &SubscriptionManager{
        semaphore: make(chan struct{}, maxSubscriptions),
    }
}

func (sm *SubscriptionManager) Subscribe(client *websocket.Client, filter *api.EventSubscription) error {
    select {
    case sm.semaphore <- struct{}{}:
        // Proceed with subscription
    default:
        return errors.New("maximum subscriptions reached")
    }
    
    subscription, err := client.Subscribe(context.Background(), filter)
    if err != nil {
        <-sm.semaphore // Release semaphore
        return err
    }
    
    sm.subscriptions = append(sm.subscriptions, subscription)
    return nil
}
```

## Network Endpoints

| Network | URL | Description |
|---------|-----|-------------|
| Local | `ws://localhost:26657/v3/ws` | Local development node |
| Testnet | `wss://testnet.accumulatenetwork.io/v3/ws` | Testnet network |
| Mainnet | `wss://mainnet.accumulatenetwork.io/v3/ws` | Production mainnet |

## Best Practices

1. **Connection Reuse**: Maintain persistent connections for multiple operations
2. **Error Handling**: Implement robust error handling and reconnection logic
3. **Resource Management**: Properly close subscriptions and connections
4. **Buffer Sizing**: Configure appropriate buffer sizes for your use case
5. **Concurrency**: Use goroutines for handling multiple streams concurrently
6. **Monitoring**: Monitor connection health and subscription status
7. **Graceful Shutdown**: Implement proper cleanup on application shutdown

## Troubleshooting

### Common Issues

1. **Connection Drops**: Implement reconnection logic with exponential backoff
2. **Event Loss**: Increase buffer sizes or process events faster
3. **Memory Leaks**: Ensure proper cleanup of subscriptions and goroutines
4. **High CPU Usage**: Optimize event processing and reduce subscription frequency

### Debug Configuration
```go
// Enable WebSocket debugging
client, err := websocket.NewClientWithConfig(url, &websocket.Config{
    Debug: true, // Enable debug logging
})
```

---

*The API v3 WebSocket client is ideal for applications requiring real-time data and event-driven architectures in the Accumulate ecosystem.*
