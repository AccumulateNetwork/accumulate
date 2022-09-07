package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run ../../../tools/cmd/gen-enum --package api enums.yml
//go:generate go run ../../../tools/cmd/gen-types --long-union-discriminator --package api responses.yml options.yml records.yml types.yml --reference ../../../smt/managed/types.yml
//go:generate go run ../../../tools/cmd/gen-types --package api --language go-union --out unions_gen.go records.yml

type RecordType uint64

type Record interface {
	encoding.BinaryValue
	RecordType() RecordType
}

type NodeService interface {
	Status(context.Context) (*NodeStatus, error)
	Version(context.Context) (*NodeVersion, error)
	Describe(context.Context) (*NodeDescription, error)
	Metrics(context.Context) (*NodeMetrics, error)
}

type QueryService interface {
	QueryRecord(ctx context.Context, account *url.URL, fragment []string, opts QueryRecordOptions) (Record, error)
	QueryRange(ctx context.Context, account *url.URL, fragment []string, opts QueryRangeOptions) (*RecordRange[Record], error)
	// Search(ctx context.Context, scope *url.URL, query string, opts SearchOptions) (Record, error)
}

type SubmitService interface {
	Submit(ctx context.Context, envelope *protocol.Envelope, opts SubmitOptions) ([]*Submission, error)
}

type FaucetService interface {
	Faucet(ctx context.Context, account *url.URL, opts SubmitOptions) (*Submission, error)
}
