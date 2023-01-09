// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package api enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --long-union-discriminator --package api responses.yml options.yml records.yml events.yml types.yml queries.yml --reference ../../types/merkle/types.yml,../../../protocol/general.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --long-union-discriminator --package api --language go-union --out unions_gen.go records.yml events.yml queries.yml --reference options.yml

// ServiceType is used to identify services.
type ServiceType uint64

// QueryType is the type of a [Query].
type QueryType uint64

// RecordType is the type of a [Record].
type RecordType uint64

// EventType is the type of an [Event].
type EventType uint64

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

// Event is an event returned by [EventService].
type Event interface {
	encoding.UnionValue
	EventType() EventType
}

type NodeService interface {
	// NodeStatus returns the status of the node.
	NodeStatus(ctx context.Context, opts NodeStatusOptions) (*NodeStatus, error)
}

type NetworkService interface {
	// NetworkService returns the status of the network.
	NetworkStatus(ctx context.Context, opts NetworkStatusOptions) (*NetworkStatus, error)
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
