package blocks

import (
	"context"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	messaging "gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	url "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// QueryAnchorRecord fetches a BlockAnchor MessageRecord by URL using the v3 API client (JSON-RPC)
func QueryAnchorRecord(ctx context.Context, client api.Querier, anchorUrl *url.URL) (*api.MessageRecord[*messaging.BlockAnchor], error) {
	rec, err := client.Query(ctx, anchorUrl, nil)
	if err != nil {
		return nil, err
	}
	anchorRec, ok := rec.(*api.MessageRecord[*messaging.BlockAnchor])
	if !ok {
		return nil, errors.NotFound.WithFormat("not a BlockAnchor record: %T", rec)
	}
	return anchorRec, nil
}
