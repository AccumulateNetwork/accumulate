package private

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type Sequencer interface {
	Sequence(ctx context.Context, src, dst *url.URL, num uint64) (*api.TransactionRecord, error)
}
