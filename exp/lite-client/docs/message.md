# Notes on the v3 Message Client

I went into `pkg/api/v3/message/client.go` and I found what might be a promising client struct defined as follows:

```go
// Client is a binary message client for API v3.
type Client struct {
    Transport Transport
}

type AddressedClient struct {
    *Client
    Address multiaddr.Multiaddr
}
```

So then I looked into what the interface `Transport` was. I found its definition in `pkg/api/v3/message/transport.go`:

```go
type Transport interface {
    // RoundTrip opens one or more streams to nodes that can serve the requests
    // and processes a request-response round trip on a stream for each request.
    RoundTrip(ctx context.Context, requests []Message, callback ResponseCallback) error

    // OpenStream opens a stream to a node that can serve the request, processes
    // a request-response round trip on the stream, and returns the open stream.
    //
    // If a batch has been opened for the context, multiple calls to OpenStream
    // (with that context) will return the same Stream. Depending on the service
    // it may not be safe to reuse streams in this way.
    //
    // If a batch has _not_ been opened for the context, it is the
    // responsibility of the caller to close the stream or cancel the context.
    OpenStream(ctx context.Context, request Message, callback ResponseCallback) (Stream, error)
}
```

`RoutedTransport` is a `[Transport]` implementation that routes messages to the appropriate service:

```go
type RoutedTransport struct {
    // Network to query. Some queries are only available when a network is specified.
    Network string

    // Dialer dials connections to a given address. The client indicates that
    // the stream can be closed by canceling the context passed to Dial.
    Dialer Dialer

    // Router determines the address a message should be routed to.
    Router Router

    // Attempts is the number of connection attempts to make.
    Attempts int

    // Debug prints round trip requests to stderr.
    Debug bool
}
```

And along with it I also found this struct that implements the `Transport` interface for concrete instantiation and such.

---

Then I looked into how the test cases were conducted for querying using this client and I found the following tests:

```go
func TestNodeService(t *testing.T) {
    expect := &api.ConsensusStatus{Ok: true, Version: "asdf", ValidatorKeyHash: [32]byte{1, 2, 3}}
    s := mocks.NewConsensusService(t)
    s.EXPECT().ConsensusStatus(mock.Anything, mock.Anything).Return(expect, nil)
    c := SetupTest(t, ConsensusService{ConsensusService: s})
    actual, err := c.ConsensusStatus(context.Background(), api.ConsensusStatusOptions{NodeID: "QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN", Partition: "foo"})
    require.NoError(t, err)
    require.True(t, expect.Equal(actual))
}

func TestNetworkService(t *testing.T) {
    g := core.NewGlobals(nil)
    expect := &api.NetworkStatus{Oracle: g.Oracle, Globals: g.Globals, Network: g.Network, Routing: g.Routing}
    s := mocks.NewNetworkService(t)
    s.EXPECT().NetworkStatus(mock.Anything, mock.Anything).Return(expect, nil)
    c := SetupTest(t, NetworkService{NetworkService: s})
    actual, err := c.NetworkStatus(context.Background(), api.NetworkStatusOptions{Partition: "foo"})
    require.NoError(t, err)
    require.True(t, expect.Equal(actual))
}

func TestMetrics(t *testing.T) {
    expect := &api.Metrics{TPS: 10}
    s := mocks.NewMetricsService(t)
    s.EXPECT().Metrics(mock.Anything, mock.Anything).Return(expect, nil)
    c := SetupTest(t, MetricsService{MetricsService: s})
    actual, err := c.Metrics(context.Background(), api.MetricsOptions{Partition: "foo"})
    require.NoError(t, err)
    require.True(t, expect.Equal(actual))
}

func TestQuerier(t *testing.T) {
    expect := &api.UrlRecord{Value: protocol.AccountUrl("foo")}
    s := mocks.NewQuerier(t)
    s.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
    c := SetupTest(t, Querier{Querier: s})
    actual, err := c.Query(context.Background(), protocol.AccountUrl("foo"), nil)
    require.NoError(t, err)
    require.True(t, api.EqualRecord(expect, actual))
}

func TestSubmitter(t *testing.T) {
    expect := []*api.Submission{{Success: true, Status: &protocol.TransactionStatus{}}}
    s := mocks.NewSubmitter(t)
    s.EXPECT().Submit(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
    c := SetupTest(t, Submitter{Submitter: s})
    sig := &protocol.ED25519Signature{Signer: protocol.AccountUrl("foo")}
    actual, err := c.Submit(context.Background(), &messaging.Envelope{Signatures: []protocol.Signature{sig}}, api.SubmitOptions{})
    require.NoError(t, err)
    require.Equal(t, len(expect), len(actual))
    require.True(t, expect[0].Equal(actual[0]))
}

func TestValidator(t *testing.T) {
    expect := []*api.Submission{{Success: true, Status: &protocol.TransactionStatus{}}}
    s := mocks.NewValidator(t)
    s.EXPECT().Validate(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
    c := SetupTest(t, Validator{Validator: s})
    sig := &protocol.ED25519Signature{Signer: protocol.AccountUrl("foo")}
    actual, err := c.Validate(context.Background(), &messaging.Envelope{Signatures: []protocol.Signature{sig}}, api.ValidateOptions{})
    require.NoError(t, err)
    require.Equal(t, len(expect), len(actual))
    require.True(t, expect[0].Equal(actual[0]))
}
```

These tests show the different things that can be done with the client.

---

## Lite Client Implementation

I'm currently working on the implementation of the Lite Client. The lite client will:

1. Create cryptographic proofs based on the BPT root hash for a set of account states.
2. Validate signatures from the genesis block to the current major block
3. Validate the signatures for the minor blocks to the present block
4. Create the cryptographic receipt to the root hash that covers the BPT root hash.
5. Collect the hashes and transactions for the set of accounts

> **Note:** Specifically, I have been stuck for days on Phase 2 of implementing the Lite Client.

### Phase 2: Major Block Signature Validation

*Aligns with Purpose Step 2: Validate signatures from the genesis block to the current major block.*

- **Implement signature validation from genesis to the latest major block:**
    1. Retrieve the genesis block and its authority
    2. For each major block, verify its signature against the authority of its time
    3. Track authority changes and update the verification chain accordingly
- **Implement major block validation:**
    1. Verify each major block's index is sequential
    2. Validate that each block's timestamp aligns with the major block schedule
    3. Verify anchoring data for each major block

Maybe I can use the v3 message client to help me with phase 2.

I looked into `pkg/api/v3/types_gen.go` and I found some structs that might contain the data I'm looking for:

```go
// MessageRecord
// (T is messaging.Message)
type MessageRecord[T messaging.Message] struct {
    fieldsSet []bool
    ID        *url.TxID                  `json:"id,omitempty" form:"id" query:"id" validate:"required"`
    Message   T                          `json:"message,omitempty" form:"message" query:"message" validate:"required"`
    Status    errors2.Status             `json:"status,omitempty" form:"status" query:"status" validate:"required"`
    Error     *errors2.Error             `json:"error,omitempty" form:"error" query:"error" validate:"required"`
    Result    protocol.TransactionResult `json:"result,omitempty" form:"result" query:"result" validate:"required"`
    // Received is the block when the transaction was first received.
    Received      uint64                            `json:"received,omitempty" form:"received" query:"received" validate:"required"`
    Produced      *RecordRange[*TxIDRecord]         `json:"produced,omitempty" form:"produced" query:"produced" validate:"required"`
    Cause         *RecordRange[*TxIDRecord]         `json:"cause,omitempty" form:"cause" query:"cause" validate:"required"`
    Signatures    *RecordRange[*SignatureSetRecord] `json:"signatures,omitempty" form:"signatures" query:"signatures" validate:"required"`
    Historical    bool                              `json:"historical,omitempty" form:"historical" query:"historical" validate:"required"`
    Sequence      *messaging.SequencedMessage       `json:"sequence,omitempty" form:"sequence" query:"sequence"`
    SourceReceipt *merkle.Receipt                   `json:"sourceReceipt,omitempty" form:"sourceReceipt" query:"sourceReceipt" validate:"required"`
    LastBlockTime *time.Time                        `json:"lastBlockTime,omitempty" form:"lastBlockTime" query:"lastBlockTime" validate:"required"`
    extraData     []byte
}

// PublicKeySearchQuery
// (for signature lookups)
type PublicKeySearchQuery struct {
    fieldsSet []bool
    PublicKey []byte                 `json:"publicKey,omitempty" form:"publicKey" query:"publicKey" validate:"required"`
    Type      protocol.SignatureType `json:"type,omitempty" form:"type" query:"type" validate:"required"`
    extraData []byte
}

// ValidateOptions
// (for signature validation)
type ValidateOptions struct {
    fieldsSet []bool
    // Full fully validates the signatures and transactions (default yes).
    Full      *bool `json:"full,omitempty" form:"full" query:"full"`
    extraData []byte
}

// SignatureSetRecord
// (signatures for an account)
type SignatureSetRecord struct {
    fieldsSet  []bool
    Account    protocol.Account                                `json:"account,omitempty" form:"account" query:"account" validate:"required"`
    Signatures *RecordRange[*MessageRecord[messaging.Message]] `json:"signatures,omitempty" form:"signatures" query:"signatures" validate:"required"`
    extraData  []byte
}

// KeyRecord
// (authority and key info)
type KeyRecord struct {
    fieldsSet []bool
    Authority *url.URL          `json:"authority,omitempty" form:"authority" query:"authority" validate:"required"`
    Signer    *url.URL          `json:"signer,omitempty" form:"signer" query:"signer" validate:"required"`
    Version   uint64            `json:"version,omitempty" form:"version" query:"version" validate:"required"`
    Index     uint64            `json:"index,omitempty" form:"index" query:"index" validate:"required"`
    Entry     *protocol.KeySpec `json:"entry,omitempty" form:"entry" query:"entry" validate:"required"`
    extraData []byte
}
```

These are the only fields that have something related to signatures as fields.