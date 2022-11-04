# API v3

The API has been broken up both along lines of call type and transport
mechanism. The specification defines services which implement methods, and
transport mechanisms for exchanging remote method calls.

## Links

### Packages

- [Specification](.)
  - [Services](api.go)
- [JSON-RPC transport](jsonrpc)
- [P2P transport](p2p) (messages, streams, handlers)
- [Websocket transport](websocket)
- [Service implementation](/internal/node/api)
  - [P2P internals](/internal/node/api/p2p) (the rest of the P2P stuff)

### Other

- [pkg.go.dev](https://pkg.go.dev/gitlab.com/accumulatenetwork/accumulate@v1.0.0-rc3.3.0.20221022212648-f9808866894c/pkg/api/v3)

## Services

Each service defines a single method. Having one method per service was not
entirely intentional, but it is convenient and allows for flexibility of
implementation:

- Services can be implemented independently.
- A service provider is not obligated to implement all services.
- Implementing middleware is straight forward.

### Node and network services

The node service's node status method returns the status of the node. The
request includes a node ID so that routing middleware may route the request to
the specified node.

The network service's network status method returns the currently active network
global variables. This may be expanded to include the status of nodes in the
network and other related information.

The metrics service's metrics method returns the transactions per second of the
last N blocks of the receiving partition. The request includes a partition ID so
that routing middleware may route the request to the specified partition. The
current terminal implementation actually returns chain entries per second, not
transactions per second, since that is less intensive to calculate.

### Submit services

The submit service's submit method submits an envelope to the network.

The validate service's validate method calls CheckTx. In the future once
signatures and CheckTx are reworked, validate will partially or fully validate
the envelope. Partial validation will check if the signature is valid and the
signer can pay, i.e. whether the envelope would pass CheckTx. Full validation
will attempt to determine whether the envelope would succeed when executed. This
will be provided partially as a debugging aid once CheckTx is updated to no
longer execute that check.

The faucet service's faucet method constructs and submits a transaction that
deposits ACME into a lite token account.

### Query services

The query service's query method retrieves data. The query method accepts a
scope (URL) and a structured query. The query parameter is a union, including
types such as chain query, data query, public key search, etc. Depending on the
type of query and the parameters of the query, the query method can return a
range of different record types. Query types are defined [here](queries.yml) and
record types are defined [here](records.yml).

The event service's subscribe method subscribes to event notifications. The
implementation is incomplete and subject to change.

## Transports

Transport mechanisms are responsible for transporting the input and output
parameters of a remote method call between the caller (client) and service
provider (server).

The implementation of a transport mechanisms must be defined such that they are
completely transparent from an outside perspective. The service provider
component of a transporter must forward requests to a service implementation,
and the caller component must present itself as a service implementation, such
that from the caller's perspective there is little difference between a remote
call and a local call. For example, the constructors should have something like
the following signature:

```go
func NewServer(api.Querier, ServerOptions) QuerierServer

func NewClient(ClientOptions) api.Querier
```

### JSON-RPC

The JSON-RPC transport transports remote method calls as JSON-RPC method calls.
The server provides a set of service implementations, which are wrapped and
exposed as JSON-RPC methods via an HTTP listener. The client provides a server
URL and implements all the API services via JSON-RPC calls.

### Websocket

The websocket transport transports remote method calls and events over a
websocket. The implementation is incomplete and subject to change.

### P2P

The P2P transport transports remote method calls over a libp2p network. The P2P
nodes gossip about which nodes are validators and what partition they belong to,
and which are not, maintaining a table of other nodes and their role in the
network.

Each P2P node can be both a service provider and a caller. The node can provide
services by configuring a remote method call message handler, wrapping its
service implementation. The node provides a client that wraps calls as a message
and sends it to the appropriate node. Messages can be sent directly to a
specific node, or the caller can determine which partition the message should be
routed to and use its peer table to select an appropriate node.

## Architecture & The Future

The idea is for validators to expose only Tendermint P2P and Accumulate P2P, and
for API nodes to expose traditional APIs and proxy between those APIs and the
P2P network.

The P2P transport should be expanded to recognize which nodes are validators and
which are data servers, so that queries can be routed to data servers.

The P2P node internals should be updated to impose rate limits and balance load
across validator and data server nodes. The API nodes could use a distributed
hash table such that load is balanced overall across service provider nodes,
instead of just balanced per API node.

The executor's internal API calls should be migrated from v2 to v3.

The P2P internals should be updated to route node queries to specific nodes, so
that the API v2 compatibility layer can be decoupled from validator nodes and
moved to the API nodes.

The P2P network should be split into red and black (private and public) sides,
with the API nodes providing a gateway between the two. The private network can
be gated via an on-chain whitelist such that incoming connections from untrusted
peers are rejected. This will protect validator and data server nodes from
malicious or misbehaved peers, and allow the API to (relatively) easily impose
rate limits.