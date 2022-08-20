package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run ../../../tools/cmd/gen-enum --package api enums.yml
//go:generate go run ../../../tools/cmd/gen-types --package api responses.yml options.yml types.yml
//go:generate go run ../../../tools/cmd/gen-types --package api --language go-union --out unions_gen.go responses.yml

type NetworkModule interface {
	Metrics(context.Context) (*NetworkMetrics, error)
}

type NodeModule interface {
	Status(context.Context) (*NodeStatus, error)
	Version(context.Context) (*NodeVersion, error)
	Describe(context.Context) (*NodeDescription, error)
	Metrics(context.Context) (*NodeMetrics, error)
}

type QueryModule interface {
	QueryState(ctx context.Context, account *url.URL, fragment []string, opts QueryStateOptions) (Record, error)
	QuerySet(ctx context.Context, account *url.URL, fragment []string, opts QuerySetOptions) (Record, error)
	Search(ctx context.Context, scope *url.URL, query string, opts SearchOptions) (Record, error)
}

type SubmitModule interface {
	Submit(ctx context.Context, envelope *protocol.Envelope, opts SubmitOptions) (*Submission, error)
}

type FaucetModule interface {
	SubmitModule
	Faucet(ctx context.Context, account *url.URL, opts SubmitOptions) (*Submission, error)
}

type SubmitMode uint64

type RecordType uint64

type Record interface {
	encoding.BinaryValue
	Type() RecordType
}
