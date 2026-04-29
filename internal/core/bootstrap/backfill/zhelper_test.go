package backfill

import (
    "context"

    "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
    "gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
    "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type nopSrc struct{}

func (nopSrc) QueryAccount(_ context.Context, _ *url.URL, _ *api.DefaultQuery) (*api.AccountRecord, error) {
    return &api.AccountRecord{}, nil
}
func (nopSrc) QueryDirectoryUrls(_ context.Context, _ *url.URL, _ *api.DirectoryQuery) (*api.RecordRange[*api.UrlRecord], error) {
    return &api.RecordRange[*api.UrlRecord]{}, nil
}
func (nopSrc) QueryPendingIds(_ context.Context, _ *url.URL, _ *api.PendingQuery) (*api.RecordRange[*api.TxIDRecord], error) {
    return &api.RecordRange[*api.TxIDRecord]{}, nil
}
func (nopSrc) QueryAccountChains(_ context.Context, _ *url.URL, _ *api.ChainQuery) (*api.RecordRange[*api.ChainRecord], error) {
    return &api.RecordRange[*api.ChainRecord]{}, nil
}
func (nopSrc) QueryChainEntries(_ context.Context, _ *url.URL, _ *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
    return &api.RecordRange[*api.ChainEntryRecord[api.Record]]{}, nil
}
func (nopSrc) QueryMessage(_ context.Context, _ *url.TxID, _ *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error) {
    return nil, nil
}
