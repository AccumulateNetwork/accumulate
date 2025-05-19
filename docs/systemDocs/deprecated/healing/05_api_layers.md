# API Layers

## Introduction

The healing processes in Accumulate interact with multiple API layers in the stack. This document provides a detailed explanation of these API layers, how they are used in the healing processes, and their relationships.

## API Layer Overview

Accumulate's API architecture consists of several layers:

1. **Tendermint APIs**: Low-level APIs for consensus and blockchain management
2. **Original Accumulate APIs (v1)**: Basic functionality for interacting with the blockchain
3. **Accumulate v2 APIs**: Enhanced query capabilities and improved transaction submission
4. **Accumulate v3 APIs**: Current generation with message-based architecture and scope property

Understanding these layers is crucial for comprehending how the healing processes work and how they interact with the rest of the Accumulate system.

## Tendermint APIs

At the lowest level, Accumulate uses Tendermint for consensus and blockchain management. The healing processes interact with Tendermint APIs in several ways:

### ABCI (Application Blockchain Interface)

```go
// From internal/node/abci/client.go
func NewClient(logger log.Logger, app abci.Application, socketPath string) (*Client, error) {
    socket := abciclient.NewSocketClient(socketPath, false)
    err := socket.Start()
    if err != nil {
        return nil, errors.UnknownError.WithFormat("start ABCI client: %w", err)
    }

    return &Client{
        logger: logger,
        app:    app,
        client: socket,
    }, nil
}
```

The ABCI interface is used for transaction submission and validation. It provides a standardized interface between the Tendermint consensus engine and the Accumulate application logic.

### RPC (Remote Procedure Call)

```go
// From internal/node/rpc.go
func (n *Node) initRPC(ctx context.Context) error {
    n.logger.Info("Starting RPC server", "address", n.config.Tendermint.RPC.ListenAddress)
    rpcConfig := n.config.Tendermint.RPC
    rpcConfig.MaxBodyBytes = int64(n.config.Tendermint.MaxBodyBytes)
    rpcConfig.MaxHeaderBytes = n.config.Tendermint.MaxHeaderBytes

    rpccore := rpc.NewCoreClient(n.client, n.logger.With("module", "rpc"))
    mux := http.NewServeMux()
    rpc.RegisterRPCFuncs(mux, rpccore.Routes, n.logger)
    
    // ...
}
```

The RPC interface is used to query transaction status and blockchain state. It provides a way for clients to interact with the Tendermint node and retrieve information about the blockchain.

### P2P (Peer-to-Peer)

```go
// From internal/node/p2p.go
func (n *Node) initP2P(ctx context.Context) error {
    n.logger.Info("Starting P2P server", "address", n.config.Tendermint.P2P.ListenAddress)
    
    // ...
    
    sw := p2p.NewSwitch(p2p.SwitchConfig{
        MaxNumInboundPeers:     n.config.Tendermint.P2P.MaxNumInboundPeers,
        MaxNumOutboundPeers:    n.config.Tendermint.P2P.MaxNumOutboundPeers,
        PersistentPeers:        n.config.Tendermint.P2P.PersistentPeers,
        Seeds:                  n.config.Tendermint.P2P.Seeds,
        DialTimeout:            n.config.Tendermint.P2P.DialTimeout,
        HandshakeTimeout:       n.config.Tendermint.P2P.HandshakeTimeout,
        MConfig:                mConfig,
        Fuzz:                   false,
        AuthEnc:                true,
    })
    
    // ...
}
```

The P2P interface is used for node discovery and communication. It enables Tendermint nodes to find each other, exchange information, and maintain a connected network.

## Original Accumulate APIs (v1)

The original Accumulate APIs provided basic functionality for interacting with the blockchain:

### Transaction Submission

```go
// From pkg/client/api/v1/submit.go
func (c *Client) Submit(ctx context.Context, envelope *protocol.Envelope) (*api.TxResponse, error) {
    data, err := envelope.MarshalBinary()
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    var resp api.TxResponse
    err = c.RequestAPIv2(ctx, "submit", map[string]interface{}{
        "data": data,
    }, &resp)
    if err != nil {
        return nil, err
    }

    return &resp, nil
}
```

This API allows clients to submit transactions to the Accumulate network. It takes an envelope containing one or more transactions and signatures and submits it to the network.

### Query Operations

```go
// From pkg/client/api/v1/query.go
func (c *Client) Query(ctx context.Context, query *api.Query) (*api.QueryResponse, error) {
    var resp api.QueryResponse
    err := c.RequestAPIv2(ctx, "query", query, &resp)
    if err != nil {
        return nil, err
    }

    return &resp, nil
}
```

This API allows clients to query the state of the Accumulate network. It supports various query types, such as querying an account, a transaction, or a chain.

### Limitations

The original API design had several limitations:

1. **Limited Query Options**: No support for complex queries or filtering
2. **No Pagination**: No built-in support for paginating large result sets
3. **Limited Error Handling**: No standardized error types or error codes
4. **No Scope Control**: No way to control the scope of a query (local, network, global)

## Accumulate v2 APIs

The v2 APIs introduced significant improvements to address the limitations of the original APIs:

### Enhanced Query Capabilities

```go
// From pkg/client/api/v2/query.go
func (c *Client) QueryTransaction(ctx context.Context, txid *url.TxID, opts api.QueryOptions) (*api.TransactionQueryResponse, error) {
    var resp api.TransactionQueryResponse
    err := c.RequestAPIv2(ctx, "query-tx", map[string]interface{}{
        "txid":             txid,
        "wait":             opts.Wait,
        "include-receipt":  opts.IncludeReceipt,
        "include-status":   opts.IncludeStatus,
        "include-pending":  opts.IncludePending,
        "include-produced": opts.IncludeProduced,
    }, &resp)
    if err != nil {
        return nil, err
    }

    return &resp, nil
}
```

The v2 APIs introduced enhanced query capabilities with options for including additional information in the response, such as receipts, status, and pending transactions.

### Improved Transaction Submission

```go
// From pkg/client/api/v2/submit.go
func (c *Client) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) (*api.SubmitResponse, error) {
    data, err := envelope.MarshalBinary()
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    var resp api.SubmitResponse
    err = c.RequestAPIv2(ctx, "submit", map[string]interface{}{
        "data": data,
        "wait": opts.Wait,
    }, &resp)
    if err != nil {
        return nil, err
    }

    return &resp, nil
}
```

The v2 APIs improved transaction submission with options for waiting for transaction completion, which is particularly useful for the healing processes.

### Structured Error Handling

```go
// From pkg/errors/errors.go
var (
    // ErrNotFound is returned when a requested resource is not found
    ErrNotFound = Define(NotFound, "not found")
    
    // ErrBadRequest is returned when a request is malformed or invalid
    ErrBadRequest = Define(BadRequest, "bad request")
    
    // ErrDelivered is returned when a transaction has already been delivered
    ErrDelivered = Define(Delivered, "already delivered")
    
    // ...
)
```

The v2 APIs introduced structured error handling with defined error types and error codes, making it easier to handle specific error conditions in the healing processes.

## Accumulate v3 APIs

The v3 APIs represent the current generation and are used extensively by the healing processes:

### Message-Based Architecture

```go
// From pkg/api/v3/message/client.go
func (c *Client) Request(ctx context.Context, req Message) (Message, error) {
    // Marshal the request
    data, err := req.MarshalBinary()
    if err != nil {
        return nil, errors.UnknownError.WithFormat("marshal request: %w", err)
    }

    // Send the request
    resp, err := c.Transport.Request(ctx, data)
    if err != nil {
        return nil, err
    }

    // Unmarshal the response
    msg, err := UnmarshalBinary(resp)
    if err != nil {
        return nil, errors.UnknownError.WithFormat("unmarshal response: %w", err)
    }

    // Check for error response
    if err, ok := msg.(*ErrorResponse); ok {
        return nil, errors.UnknownError.WithFormat("API error: %v", err.Error)
    }

    return msg, nil
}
```

The v3 APIs use a message-based architecture where all requests and responses are messages that can be marshaled and unmarshaled. This provides a more flexible and extensible API design.

### Scope Property

The scope property is a critical feature of v3 APIs that determines the context of an API call:

```go
// From pkg/api/v3/api.go
type QueryOptions struct {
    // Scope controls how far the query will propagate
    Scope Scope

    // Wait indicates that the caller wants to wait for the transaction to be delivered
    Wait bool

    // ...
}

// Scope controls how far a query will propagate
type Scope int

const (
    // ScopeLocal limits the query to the local partition
    ScopeLocal Scope = iota

    // ScopeNetwork extends the query to the entire network the partition belongs to
    ScopeNetwork

    // ScopeGlobal extends the query to all known networks
    ScopeGlobal
)
```

The scope property allows clients to control where transactions and anchors are queried from:

- **ScopeLocal**: Limits the query to the local partition
- **ScopeNetwork**: Extends the query to the entire network the partition belongs to
- **ScopeGlobal**: Extends the query to all known networks

In the healing context, scope is used to control where transactions and anchors are queried from:

```go
// Example from healing code using scope
Q := api.Querier2{Querier: args.Querier}
s, err := Q.QueryMessage(ctx, r.ID, &api.QueryOptions{
    Scope: api.ScopeNetwork, // Look across the network for the message
})
```

### Addressed Client

```go
// From pkg/api/v3/message/client.go
type AddressedClient struct {
    Client  *Client
    Address multiaddr.Multiaddr
}

// NetworkStatus implements [api.NetworkService.NetworkStatus].
func (c AddressedClient) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
    // Wrap the request as a NetworkStatusRequest and expect a
    // NetworkStatusResponse, which is unpacked into a NetworkStatus
    return typedRequest[*NetworkStatusResponse, *api.NetworkStatus](c, ctx, &NetworkStatusRequest{NetworkStatusOptions: opts})
}
```

The addressed client allows directing requests to specific nodes, which is essential for signature collection in the anchor healing process:

```go
// From internal/core/healing/anchors.go
res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
```

### Streaming Support

```go
// From pkg/api/v3/message/client.go
func (c *Client) Subscribe(ctx context.Context, opts api.SubscribeOptions) (api.Subscription, error) {
    return c.ForAddress(nil).Subscribe(ctx, opts)
}

func (c AddressedClient) Subscribe(ctx context.Context, opts api.SubscribeOptions) (api.Subscription, error) {
    req := &SubscribeRequest{SubscribeOptions: opts}
    
    // ...
    
    return &subscription{
        ctx:    ctx,
        client: c.Client,
        stream: stream,
    }, nil
}
```

Streaming allows the healing process to monitor events in real-time, which can be useful for tracking the delivery of healed transactions and anchors.

## API Usage in Healing Processes

The healing processes primarily use v3 APIs, but interact with all layers:

### Tendermint Layer Usage

```go
// From internal/core/healing/anchors.go
addr := multiaddr.StringCast("/p2p/" + peer.String())
if len(info.Addresses) > 0 {
    addr = info.Addresses[0].Encapsulate(addr)
}
```

The healing processes use Tendermint's P2P layer for node discovery and communication, particularly for collecting signatures in the anchor healing process.

### v1/v2 Compatibility

```go
// From internal/core/healing/synthetic.go
// Check if we're using v1 or v2 APIs
if args.NetInfo.Status.ExecutorVersion.V2Enabled() {
    // Use v2 API logic
} else {
    // Use v1 API logic
}
```

The healing processes include version detection to handle different API versions, ensuring compatibility with older networks.

### v3 Primary Usage

```go
// From internal/core/healing/synthetic.go
// Query the synthetic transaction
r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
if err != nil {
    return err
}
si.ID = r.ID

// Query the status
Q := api.Querier2{Querier: args.Querier}
if s, err := Q.QueryMessage(ctx, r.ID, nil); err == nil &&
    // Has it already been delivered?
    s.Status.Delivered() &&
    // Does the sequence info match?
    s.Sequence != nil &&
    s.Sequence.Source.Equal(protocol.PartitionUrl(si.Source)) &&
    s.Sequence.Destination.Equal(protocol.PartitionUrl(si.Destination)) &&
    s.Sequence.Number == si.Number {
    // If it's been delivered, skip it
    slog.InfoContext(ctx, "Synthetic message has been delivered", "id", si.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
    return errors.Delivered
}
```

The healing processes primarily use v3 APIs for network status queries, transaction and anchor queries, signature collection, and transaction submission.

### Scope Usage

```go
// From internal/core/healing/synthetic.go
// Query with network scope to find the transaction across the network
s, err := Q.QueryMessage(ctx, r.ID, &api.QueryOptions{
    Scope: api.ScopeNetwork,
})
```

The healing processes use different scope values depending on the operation:

- **ScopeLocal**: Used when querying known partition-specific data
- **ScopeNetwork**: Used for most healing operations to ensure network-wide consistency
- **ScopeGlobal**: Rarely used in healing, primarily for cross-network operations

## Command-Line Interface Integration

The command-line interface for the healing processes integrates with the API layers:

```go
// From tools/cmd/debug/heal_common.go
func (h *healer) heal(args []string) {
    // ...
    
    // Use the client API to get network information
    var err error
    h.net, err = getNetworkInfo(h.ctx, h.C1, args[0])
    if err != nil {
        slog.ErrorContext(h.ctx, "Failed to get network information", "error", err)
        return
    }
    
    // ...
    
    // Use the query API to check transaction status
    delivered, err := h.isDelivered(src, dst, i)
    if err != nil {
        return nil, err
    }
    
    // ...
    
    // Use the submit API to submit healed transactions
    err := healing.HealTransaction(h.ctx, healing.HealTransactionArgs{
        Client:  h.C2.ForAddress(nil),
        Source:  source,
        Dest:    destination,
        TxID:    id,
        Number:  number,
        TxnMap:  txns,
    })
    
    // ...
}
```

This integration allows the command-line interface to interact with the various API layers in a consistent and efficient manner.

## Recent Changes

Recent changes to the API usage in the healing processes include:

1. **Enhanced Error Handling**: Improved error handling to better handle specific error conditions
2. **Scope-Aware Queries**: More strategic use of scope properties in v3 API calls
3. **Addressed Client Usage**: Enhanced use of addressed clients for directing requests to specific nodes
4. **Version Detection**: Improved version detection to handle different API versions

## Best Practices

When working with the API layers in the healing processes, it's important to follow these best practices:

1. **Use the Appropriate Scope**: Use the appropriate scope for each query to minimize network traffic and improve performance
2. **Handle Version Differences**: Include version-specific logic to handle different API versions
3. **Properly Handle Errors**: Use structured error handling to properly handle specific error conditions
4. **Use Addressed Clients**: Use addressed clients when directing requests to specific nodes
5. **Follow Data Integrity Rules**: Never fabricate or fake data that isn't available from the network

```go
// Example of following data integrity rules
// From internal/core/healing/synthetic.go
// Query the synthetic transaction
r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
if err != nil {
    return err // Return the error, don't fabricate data
}
```

## Conclusion

The healing processes in Accumulate interact with multiple API layers, from low-level Tendermint APIs to high-level v3 APIs. Understanding these layers and how they are used is crucial for comprehending the healing processes and how they maintain data consistency across the network.

In the next document, we will explore the database and caching infrastructure used by the healing processes.
