package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run ../../../tools/cmd/gen-enum --package api enums.yml
//go:generate go run ../../../tools/cmd/gen-types --long-union-discriminator --package api responses.yml options.yml records.yml events.yml types.yml --reference ../../../smt/managed/types.yml,../../../protocol/general.yml
//go:generate go run ../../../tools/cmd/gen-types --package api --language go-union --out unions_gen.go records.yml events.yml

type RecordType uint64

type EventType uint64

type Record interface {
	encoding.BinaryValue
	RecordType() RecordType
}

type Event interface {
	encoding.BinaryValue
	EventType() EventType
}

type NodeService interface {
	Status(ctx context.Context) (*NodeStatus, error)
	Metrics(ctx context.Context) (*NodeMetrics, error)
}

type QueryService interface {
	QueryRecord(ctx context.Context, account *url.URL, fragment []string, opts QueryRecordOptions) (Record, error)
	QueryRange(ctx context.Context, account *url.URL, fragment []string, opts QueryRangeOptions) (*RecordRange[Record], error)
	// Search(ctx context.Context, scope *url.URL, query string, opts SearchOptions) (Record, error)
}

type EventService interface {
	Subscribe(ctx context.Context) (<-chan Event, error)
}

type SubmitService interface {
	Submit(ctx context.Context, envelope *protocol.Envelope, opts SubmitOptions) ([]*Submission, error)
}

type FaucetService interface {
	Faucet(ctx context.Context, account *url.URL, opts SubmitOptions) (*Submission, error)
}
