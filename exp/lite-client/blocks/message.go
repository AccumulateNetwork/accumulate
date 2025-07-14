package blocks

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// QueryMessageRecord fetches a MessageRecord by URL using the v3 API client (JSON-RPC)
func QueryMessageRecord(ctx context.Context, client api.Querier, recordUrl *url.URL) (*api.MessageRecord[messaging.Message], error) {
	rec, err := client.Query(ctx, recordUrl, nil)
	if err != nil {
		return nil, err
	}
	msgRec, ok := rec.(*api.MessageRecord[messaging.Message])
	if !ok {
		return nil, errors.NotFound.WithFormat("not a MessageRecord: %T", rec)
	}
	return msgRec, nil
}
