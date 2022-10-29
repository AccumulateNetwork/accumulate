package harness

import (
	"context"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func QueryAccountAs[T protocol.Account](h *Harness, scope *url.URL) T {
	h.TB.Helper()
	var v T
	h.QueryAccountAs(scope, nil, &v)
	return v
}

func (h *Harness) Query() api.Querier2 {
	return api.Querier2{Querier: h.services}
}

func (h *Harness) QueryAccount(scope *url.URL, query *api.DefaultQuery) *api.AccountRecord {
	h.TB.Helper()
	r, err := h.Query().QueryAccount(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryAccountAs(scope *url.URL, query *api.DefaultQuery, target any) *api.AccountRecord {
	h.TB.Helper()
	r, err := h.Query().QueryAccountAs(context.Background(), scope, query, target)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryTransaction(txid *url.TxID, query *api.DefaultQuery) *api.TransactionRecord {
	h.TB.Helper()
	r, err := h.Query().QueryTransaction(context.Background(), txid, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryChain(scope *url.URL, query *api.ChainQuery) *api.ChainRecord {
	h.TB.Helper()
	r, err := h.Query().QueryChain(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryChains(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryChains(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[api.Record] {
	h.TB.Helper()
	r, err := h.Query().QueryChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[api.Record]] {
	h.TB.Helper()
	r, err := h.Query().QueryChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryTxnChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[*api.TransactionRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryTxnChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryTxnChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[*api.TransactionRecord]] {
	h.TB.Helper()
	r, err := h.Query().QueryTxnChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QuerySigChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[*api.SignatureRecord] {
	h.TB.Helper()
	r, err := h.Query().QuerySigChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QuerySigChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[*api.SignatureRecord]] {
	h.TB.Helper()
	r, err := h.Query().QuerySigChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryIdxChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[*api.IndexEntryRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryIdxChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryIdxChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[*api.IndexEntryRecord]] {
	h.TB.Helper()
	r, err := h.Query().QueryIdxChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryDataEntry(scope *url.URL, query *api.DataQuery) *api.ChainEntryRecord[*api.TransactionRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryDataEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryDataEntries(scope *url.URL, query *api.DataQuery) *api.RecordRange[*api.ChainEntryRecord[*api.TransactionRecord]] {
	h.TB.Helper()
	r, err := h.Query().QueryDataEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryDirectoryUrls(scope *url.URL, query *api.DirectoryQuery) *api.RecordRange[*api.UrlRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryDirectoryUrls(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryDirectory(scope *url.URL, query *api.DirectoryQuery) *api.RecordRange[*api.AccountRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryDirectory(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryPendingIds(scope *url.URL, query *api.PendingQuery) *api.RecordRange[*api.TxIDRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryPendingIds(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryPending(scope *url.URL, query *api.PendingQuery) *api.RecordRange[*api.TransactionRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryPending(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryMinorBlock(scope *url.URL, query *api.BlockQuery) *api.MinorBlockRecord {
	h.TB.Helper()
	r, err := h.Query().QueryMinorBlock(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryMinorBlocks(scope *url.URL, query *api.BlockQuery) *api.RecordRange[*api.MinorBlockRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryMinorBlocks(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryMajorBlock(scope *url.URL, query *api.BlockQuery) *api.MajorBlockRecord {
	h.TB.Helper()
	r, err := h.Query().QueryMajorBlock(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) QueryMajorBlocks(scope *url.URL, query *api.BlockQuery) *api.RecordRange[*api.MajorBlockRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryMajorBlocks(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) SearchForAnchor(scope *url.URL, search *api.AnchorSearchQuery) *api.RecordRange[*api.ChainEntryRecord[api.Record]] {
	h.TB.Helper()
	r, err := h.Query().SearchForAnchor(context.Background(), scope, search)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) SearchForPublicKey(scope *url.URL, search *api.PublicKeySearchQuery) *api.RecordRange[*api.KeyRecord] {
	h.TB.Helper()
	r, err := h.Query().SearchForPublicKey(context.Background(), scope, search)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) SearchForPublicKeyHash(scope *url.URL, search *api.PublicKeyHashSearchQuery) *api.RecordRange[*api.KeyRecord] {
	h.TB.Helper()
	r, err := h.Query().SearchForPublicKeyHash(context.Background(), scope, search)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) SearchForDelegate(scope *url.URL, search *api.DelegateSearchQuery) *api.RecordRange[*api.KeyRecord] {
	h.TB.Helper()
	r, err := h.Query().SearchForDelegate(context.Background(), scope, search)
	require.NoError(h.TB, err)
	return r
}

func (h *Harness) SearchForTransactionHash(scope *url.URL, search *api.TransactionHashSearchQuery) *api.RecordRange[*api.TxIDRecord] {
	h.TB.Helper()
	r, err := h.Query().SearchForTransactionHash(context.Background(), scope, search)
	require.NoError(h.TB, err)
	return r
}
