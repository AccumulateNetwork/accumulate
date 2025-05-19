# Network Initialization Replacement Plan

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (that follows) will not be deleted.

```yaml
# AI-METADATA
document_type: implementation_plan
project: accumulate_network
component: debug_tools
topic: network_initialization_replacement
version: 1.1
date: 2025-05-02
```

## Implementation Mantra

> "We do not modify how the network initialization is done. No improvements, no performance enhancements, no simplifying the code.
> new-mainnet-status is our reference implementation. The only modifications allowed are those needed to plug into debug commands like heal anchor and heal synth."

## 1. Detailed Code Analysis

### Current Implementation in heal_anchor.go

The heal_anchor.go file calls `h.setup(ctx, args[0])` on line 265 for network initialization. This setup function is defined in heal_common.go and contains the following key components:

```go
func (h *healer) setup(ctx context.Context, network string) {
    h.ctx = ctx
    h.network = network
    h.accounts = map[[32]byte]protocol.Account{}
    
    // Initialize the network using the resilient network initializer
    resilientNetworkInitializer(ctx, h, network)
    if h.net == nil {
        slog.Error("Error initializing network: network information is nil")
        return
    }
    
    // Initialize JSON-RPC client
    endpoint := accumulate.ResolveWellKnownEndpoint(network, "v3")
    h.C1 = jsonrpc.NewClient(endpoint)
    h.C1.Client.Timeout = time.Hour
    h.C1.Debug = debug
    
    // Initialize P2P node
    // ...
}
```

The `resilientNetworkInitializer` function in heal_network.go attempts to use enhanced P2P discovery first, then falls back to `collectCompleteNetworkInfo` if that fails. This approach has issues with P2P multiaddresses not having the required components, causing initialization failures.

### New-Mainnet-Status Implementation

The new-mainnet-status implementation in `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/debug/docs2/debugging/new-mainnet-status/` uses a simpler, more direct approach:

```go
// Initialize JSON-RPC client
endpoint := resolveEndpoint(h.network)
slog.Info("Connecting to Accumulate mainnet", "endpoint", endpoint)

// Create JSON-RPC client
client := jsonrpc.NewClient(endpoint)

// Set timeout for JSON-RPC client
client.Client.Timeout = time.Minute

// Store client in healer
h.C1 = client

// Collect network information - simple approach like mainnet-status
slog.Info("Collecting network information")
var err error
h.net, err = collectNetworkInfo(h.ctx, h.C1, h.network)

// Extract node information
slog.Info("Extracting node information")
h.nodes = extractNodeInfo(h.ctx, h.net)

// Check API v3 connectivity - EXACTLY like mainnet-status
checkAPIv3Connectivity(h.ctx, h.nodes, h.net, h.router)
```

This implementation properly handles P2P multiaddresses and has been proven to work correctly.

## 2. Key Issues to Address

1. **P2P Multiaddress Validation**: The current implementation doesn't properly validate P2P multiaddresses, causing initialization failures with "parse address: invalid p2p multiaddr" errors.

2. **Peer Discovery**: The current implementation doesn't properly discover and use peers from the network status information.

3. **API V3 Connectivity**: The current implementation doesn't check API V3 connectivity in the same way as mainnet-status.

4. **Error Handling**: The current implementation doesn't handle errors gracefully, especially when network initialization fails.

## 3. Implementation Strategy

Based on the detailed code analysis, we will:

1. Create a new `setupImproved` function in heal_common.go based on the new-mainnet-status implementation
2. Modify heal_anchor.go to use this new function instead of the current setup
3. Implement proper P2P multiaddress validation and filtering
4. Ensure proper error handling throughout the implementation

## 4. Specific Implementation Steps

### Step 1: Create setupImproved function in heal_common.go

```go
func (h *healer) setupImproved(ctx context.Context, network string) {
    h.ctx = ctx
    h.network = network
    h.accounts = map[[32]byte]protocol.Account{}
    
    slog.Info("Setting up healer with improved network initialization", "network", network)
    
    // Handle pprof if requested
    if pprof != "" {
        l, err := net.Listen("tcp", pprof)
        if err != nil {
            slog.Error("Error encountered", "error", err)
            return
        }
        s := new(http.Server)
        s.ReadHeaderTimeout = time.Minute
        go func() { check(s.Serve(l)) }()
        go func() { <-ctx.Done(); _ = s.Shutdown(context.Background()) }()
    }
    
    // Initialize JSON-RPC client
    endpoint := accumulate.ResolveWellKnownEndpoint(network, "v3")
    slog.Info("Connecting to API endpoint", "endpoint", endpoint)
    
    h.C1 = jsonrpc.NewClient(endpoint)
    h.C1.Client.Timeout = time.Hour
    h.C1.Debug = debug
    
    // Collect network information using the improved implementation
    var err error
    h.net, err = collectImprovedNetworkInfo(h.ctx, h.C1, network)
    if err != nil {
        slog.Error("Failed to collect network info", "error", err)
        return
    }
    
    // Get network status
    ns, err := h.C1.NetworkStatus(h.ctx, api.NetworkStatusOptions{})
    if err != nil {
        slog.Error("Failed to get network status", "error", err)
        return
    }
    
    // Create router
    h.router = routing.NewRouter(routing.RouterOptions{Initial: ns.Routing})
    
    // Extract node information
    h.nodes = extractImprovedNodeInfo(h.ctx, h.net)
    
    // Initialize P2P node with validated peers
    node, err := initializeImprovedP2PNode(ctx, network, h.net, peerDb)
    if err != nil {
        slog.Error("Error initializing P2P node", "error", err)
        return
    }
    
    // Initialize router
    h.router, err = apiutil.InitRouter(apiutil.RouterOptions{
        Context: ctx,
        Node:    node,
        Network: network,
    })
    if err != nil {
        slog.Error("Error initializing router", "error", err)
        return
    }
    
    // Wait for router to be ready
    ok := <-h.router.(*routing.RouterInstance).Ready()
    if !ok {
        slog.Error("Failed to initialize router - router not ready")
        return
    }
    
    // Initialize message client transport
    transport := &message.RoutedTransport{
        Network: network,
        Dialer:  node.DialNetwork(),
        Router:  routing.MessageRouter{Router: h.router},
        Debug:   debug,
    }
    h.C2 = &message.Client{
        Transport: transport,
    }
    
    // Check API v3 connectivity
    checkImprovedAPIv3Connectivity(h.ctx, h.nodes, h.net, h.router)
    
    // Handle light client if needed
    if lightDb != "" {
        setupLightClient(h, network)
    }
}
```

### Step 2: Implement collectImprovedNetworkInfo function

```go
func collectImprovedNetworkInfo(ctx context.Context, client *jsonrpc.Client, network string) (*healing.NetworkInfo, error) {
    slog.Info("Collecting network information", "network", network)

    // Get network status
    ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to get network status: %w", err)
    }

    // Log network status
    slog.Info("Network status retrieved", "partitions", len(ns.Network.Partitions))

    // Scan the network
    slog.Info("Scanning the network")
    networkInfo, err := healing.ScanNetwork(ctx, client)
    if err != nil {
        return nil, fmt.Errorf("failed to scan network: %w", err)
    }

    // Make sure the status is set
    networkInfo.Status = ns
    networkInfo.ID = strings.ToLower(ns.Network.NetworkName)

    // Return the network info
    return networkInfo, nil
}
```

### Step 3: Implement extractImprovedNodeInfo function

This function will be based on the extractNodeInfo function in new-mainnet-status/node_info.go, but adapted to work with the heal_anchor command.

### Step 4: Implement P2P multiaddress validation

```go
func validateMultiaddress(addr multiaddr.Multiaddr) bool {
    var hasIP, hasTCP, hasP2P bool
    
    multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
        switch c.Protocol().Code {
        case multiaddr.P_IP4, multiaddr.P_IP6, multiaddr.P_DNS, multiaddr.P_DNS4, multiaddr.P_DNS6:
            hasIP = true
        case multiaddr.P_TCP:
            hasTCP = true
        case multiaddr.P_P2P:
            hasP2P = true
        }
        return true
    })
    
    return hasIP && hasTCP && hasP2P
}
```

### Step 5: Implement improved P2P node initialization

```go
func initializeImprovedP2PNode(ctx context.Context, network string, netInfo *healing.NetworkInfo, peerDbPath string) (api.P2PNode, error) {
    // Extract valid bootstrap peers from network info
    var bootstrapPeers []multiaddr.Multiaddr
    
    // Process each partition's peers
    for _, peers := range netInfo.Peers {
        for _, peer := range peers {
            // Create P2P component
            peerComponent, err := multiaddr.NewComponent("p2p", peer.ID.String())
            if err != nil {
                slog.Debug("Failed to create p2p component", "error", err)
                continue
            }
            
            // Add P2P component to each address
            for _, addr := range peer.Addresses {
                fullAddr := addr.Encapsulate(peerComponent)
                
                // Validate the address
                if validateMultiaddress(fullAddr) {
                    bootstrapPeers = append(bootstrapPeers, fullAddr)
                }
            }
        }
    }
    
    // Initialize P2P node with validated peers
    slog.Info("Initializing P2P node", "network", network, "bootstrap_peers", len(bootstrapPeers))
    
    // Create P2P node options
    opts := []api.P2PNodeOption{
        api.P2PNodeWithNetwork(network),
        api.P2PNodeWithBootstrap(bootstrapPeers),
    }
    
    // Add peer database if specified
    if peerDbPath != "" {
        opts = append(opts, api.P2PNodeWithPeerDB(peerDbPath))
    }
    
    // Create P2P node
    return api.NewP2PNode(ctx, opts...)
}
```

### Step 6: Modify heal_anchor.go to use the new setup function

```go
// Before:
h.setup(ctx, args[0])

// After:
h.setupImproved(ctx, args[0])
```

## 5. Testing Plan

1. **Functional Testing**:
   - Test the improved implementation with the heal anchor command
   - Verify that it correctly discovers all validators
   - Confirm that it properly handles P2P multiaddresses
   - Check that API V3 connectivity works correctly

2. **Comparison Testing**:
   - Compare the output of the improved implementation with new-mainnet-status
   - Ensure that the same validators and non-validators are discovered
   - Verify that the same API V3 connectivity checks pass

3. **Error Handling Testing**:
   - Test the improved implementation with various error conditions
   - Verify that it properly handles network errors
   - Confirm that it gracefully handles invalid addresses

## 6. Success Criteria

The implementation will be considered successful if:

1. It matches the behavior of new-mainnet-status
2. It correctly discovers all validators in the network
3. It properly handles P2P multiaddresses
4. It resolves the "parse address: invalid p2p multiaddr" error
5. It successfully integrates with the heal anchor command

## 7. Implementation Timeline

1. Day 1: Create setupImproved function and collectImprovedNetworkInfo function
2. Day 2: Implement extractImprovedNodeInfo function and P2P multiaddress validation
3. Day 3: Implement improved P2P node initialization and modify heal_anchor.go
4. Day 4: Testing and debugging
5. Day 5: Final integration and documentation

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (above) will not be deleted.