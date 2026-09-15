// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package api enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --long-union-discriminator --package api responses.yml options.yml records.yml events.yml types.yml queries.yml --reference ../../database/merkle/types.yml,../../../protocol/general.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --long-union-discriminator --package api --language go-union --out unions_gen.go records.yml events.yml queries.yml --reference options.yml

// ServiceType is used to identify services.
type ServiceType uint64

// QueryType is the type of a [Query].
type QueryType uint64

// RecordType is the type of a [Record].
type RecordType uint64

// EventType is the type of an [Event].
type EventType uint64

// KnownPeerStatus is the status of a known peer.
type KnownPeerStatus int64

// Query is an API query.
type Query interface {
	encoding.UnionValue
	QueryType() QueryType

	// IsValid validates the query.
	IsValid() error
}

// Record is a record returned by a [Query].
type Record interface {
	encoding.UnionValue
	RecordType() RecordType
}

// WithLastBlockTime is a response, usually [Record], that may include the last
// block time from the node serving the request.
type WithLastBlockTime interface {
	GetLastBlockTime() *time.Time
}

// Event is an event returned by [EventService].
type Event interface {
	encoding.UnionValue
	EventType() EventType
}

type NodeService interface {
	// NodeInfo returns information about the network node.
	NodeInfo(ctx context.Context, opts NodeInfoOptions) (*NodeInfo, error)

	// FindService searches for nodes that provide the given service.
	FindService(ctx context.Context, opts FindServiceOptions) ([]*FindServiceResult, error)
}

type ConsensusService interface {
	// ConsensusStatus returns the status of the consensus node.
	ConsensusStatus(ctx context.Context, opts ConsensusStatusOptions) (*ConsensusStatus, error)
}

type NetworkService interface {
	// NetworkService returns the status of the network.
	NetworkStatus(ctx context.Context, opts NetworkStatusOptions) (*NetworkStatus, error)
}

type SnapshotService interface {
	ListSnapshots(ctx context.Context, opts ListSnapshotsOptions) ([]*SnapshotInfo, error)
}

// ProofService serves the directory's major-block spine (#4058) so a third
// party can derive the validator set by induction instead of being handed it:
// each major block's closing anchor with its archived quorum, and the binding
// of minor blocks past the spine to it. Read-only, and only the directory
// serves either method.
//
// It proves what the records said, not who was entitled to write them. The walk
// must start at a genesis anchor pinned out of band; one fetched from the same
// network is a consistency check, not a trust root. It cannot cross a network
// restart.
type ProofService interface {
	// MajorHeaderRange returns a record per major block in [start, end].
	MajorHeaderRange(ctx context.Context, opts MajorHeaderRangeOptions) ([]*MajorHeaderRecord, error)

	// MinorRootRange binds minor blocks past the spine to it.
	MinorRootRange(ctx context.Context, opts MinorRootRangeOptions) (*MinorRootRecord, error)

	// AnchorReceipt binds a partition's BPT root to a directory root — the
	// second of the two calls an account proof takes.
	//
	// A BPT is a tree of current state: every account that changes rewrites the
	// path to the root, so there is no proving an account against a past BPT.
	// The proof is built against the current one, and that root reaches the
	// directory only after an anchor round trip. So the first call returns the
	// account's receipt to its partition's BPT root, and this returns the rest.
	// On the directory the first call is already complete and this is not
	// needed.
	//
	// By default it returns the OLDEST receipt that works — the directory root
	// of the block that committed the anchor. That answer is stable: the same
	// root asked for later returns the same receipt, so a caller can record the
	// pair once. A receipt terminating at any following directory root is
	// equally valid, and AtOrAfter asks for one.
	AnchorReceipt(ctx context.Context, opts AnchorReceiptOptions) (*AnchorReceiptRecord, error)
}

type MetricsService interface {
	// Metrics returns network metrics such as transactions per second.
	Metrics(ctx context.Context, opts MetricsOptions) (*Metrics, error)
}

type Querier interface {
	// Query queries the state of an account or transaction.
	Query(ctx context.Context, scope *url.URL, query Query) (Record, error)
}

type EventService interface {
	// Subscribe subscribes to event notifications. The channel will be closed
	// once the context is canceled. Subscribe will leak goroutines if the
	// context is not canceled.
	Subscribe(ctx context.Context, opts SubscribeOptions) (<-chan Event, error)
}

type Submitter interface {
	// Submit submits an envelope for execution.
	Submit(ctx context.Context, envelope *messaging.Envelope, opts SubmitOptions) ([]*Submission, error)
}

type Validator interface {
	// Validate checks if an envelope is expected to succeed.
	Validate(ctx context.Context, envelope *messaging.Envelope, opts ValidateOptions) ([]*Submission, error)
}

type Faucet interface {
	Faucet(ctx context.Context, account *url.URL, opts FaucetOptions) (*Submission, error)
}

// Ptr returns a pointer to the argument. Ptr is intended to make it easier to
// construct queries that use pointer fields.
func Ptr[T any](v T) *T {
	return &v
}

func (r *ErrorRecord) Error() string {
	return r.Value.Error()
}

func (r *MessageRecord[T]) StatusNo() uint64 { return uint64(r.Status) }

type MessageRecordError[T messaging.Message] struct {
	*MessageRecord[T]
}

func (r *MessageRecord[T]) AsError() error {
	if r.Error == nil {
		return nil
	}
	return MessageRecordError[T]{r}
}

func (r MessageRecordError[T]) Error() string { return r.MessageRecord.Error.Error() }
