package api

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run ../../tools/cmd/gen-enum --package api enums.yml
//go:generate go run ../../tools/cmd/gen-types --package api responses.yml options.yml types.yml
//go:generate go run ../../tools/cmd/gen-types --package api --language go-union --out unions_gen.go responses.yml

type NetworkModule interface {
	Metrics() (*NetworkMetrics, error)
}

type NodeModule interface {
	Status() (*NodeStatus, error)
	Version() (*NodeVersion, error)
	Describe() (*NodeDescription, error)
	Metrics() (*NodeMetrics, error)
}

type QueryModule interface {
	QueryState(account *url.URL, fragment []string, opts QueryStateOptions) (Record, error)
	QuerySet(account *url.URL, fragment []string, opts QuerySetOptions) (Record, error)
	Search(scope *url.URL, query string, opts SearchOptions) (Record, error)
}

type SubmitModule interface {
	Submit(envelope *protocol.Envelope, opts SubmitOptions) (*Submission, error)
}

type FaucetModule interface {
	SubmitModule
	Faucet(account *url.URL, opts SubmitOptions) (*Submission, error)
}

type SubmitMode uint64

type RecordType uint64

type Record interface {
	encoding.BinaryValue
	Type() RecordType
}
