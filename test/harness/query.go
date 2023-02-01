// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package harness

import (
	"context"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NetworkStatus calls the Harness's service, failing if the call returns an
// error.
func (h *Harness) NetworkStatus(opts api.NetworkStatusOptions) *api.NetworkStatus {
	ns, err := h.services.NetworkStatus(context.Background(), opts)
	require.NoError(h.TB, err)
	return ns
}

// QueryAccountAs calls Harness.QueryAccountAs with a new T and returns that
// value.
func QueryAccountAs[T protocol.Account](h *Harness, scope *url.URL) T {
	h.TB.Helper()
	var v T
	h.QueryAccountAs(scope, nil, &v)
	return v
}

// Query returns the Harness's service as an api.Querier2.
func (h *Harness) Query() api.Querier2 {
	return api.Querier2{Querier: h.services}
}

// QueryAccount queries the Harness's service, passing the given arguments.
// QueryAccount fails if Query returns an error. See api.Querier2.QueryAccount.
func (h *Harness) QueryAccount(scope *url.URL, query *api.DefaultQuery) *api.AccountRecord {
	h.TB.Helper()
	r, err := h.Query().QueryAccount(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryAccountAs queries the Harness's service, passing the given arguments.
// QueryAccountAs fails if Query returns an error. See
// api.Querier2.QueryAccountAs.
func (h *Harness) QueryAccountAs(scope *url.URL, query *api.DefaultQuery, target any) *api.AccountRecord {
	h.TB.Helper()
	r, err := h.Query().QueryAccountAs(context.Background(), scope, query, target)
	require.NoError(h.TB, err)
	return r
}

// QueryTransaction queries the Harness's service, passing the given arguments.
// QueryTransaction fails if Query returns an error. See
// api.Querier2.QueryTransaction.
func (h *Harness) QueryTransaction(txid *url.TxID, query *api.DefaultQuery) *api.TransactionRecord {
	h.TB.Helper()
	r, err := h.Query().QueryTransaction(context.Background(), txid, query)
	require.NoError(h.TB, err)
	return r
}

// QueryChain queries the Harness's service, passing the given arguments.
// QueryChain fails if Query returns an error. See api.Querier2.QueryChain.
func (h *Harness) QueryChain(scope *url.URL, query *api.ChainQuery) *api.ChainRecord {
	h.TB.Helper()
	r, err := h.Query().QueryChain(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryAccountChains queries the Harness's service, passing the given
// arguments. QueryAccountChains fails if Query returns an error. See
// api.Querier2.QueryAccountChains.
func (h *Harness) QueryAccountChains(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryAccountChains(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryTransactionChains queries the Harness's service, passing the given
// arguments. QueryTransactionChains fails if Query returns an error. See
// api.Querier2.QueryTransactionChains.
func (h *Harness) QueryTransactionChains(scope *url.TxID, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[api.Record]] {
	h.TB.Helper()
	r, err := h.Query().QueryTransactionChains(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryChainEntry queries the Harness's service, passing the given arguments.
// QueryChainEntry fails if Query returns an error. See
// api.Querier2.QueryChainEntry.
func (h *Harness) QueryChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[api.Record] {
	h.TB.Helper()
	r, err := h.Query().QueryChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryChainEntries queries the Harness's service, passing the given arguments.
// QueryChainEntries fails if Query returns an error. See
// api.Querier2.QueryChainEntries.
func (h *Harness) QueryChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[api.Record]] {
	h.TB.Helper()
	r, err := h.Query().QueryChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryTxnChainEntry queries the Harness's service, passing the given
// arguments. QueryTxnChainEntry fails if Query returns an error. See
// api.Querier2.QueryTxnChainEntry.
func (h *Harness) QueryTxnChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[*api.TransactionRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryTxnChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryTxnChainEntries queries the Harness's service, passing the given
// arguments. QueryTxnChainEntries fails if Query returns an error. See
// api.Querier2.QueryTxnChainEntries.
func (h *Harness) QueryTxnChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[*api.TransactionRecord]] {
	h.TB.Helper()
	r, err := h.Query().QueryTxnChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QuerySigChainEntry queries the Harness's service, passing the given
// arguments. QuerySigChainEntry fails if Query returns an error. See
// api.Querier2.QuerySigChainEntry.
func (h *Harness) QuerySigChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[*api.SignatureRecord] {
	h.TB.Helper()
	r, err := h.Query().QuerySigChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QuerySigChainEntries queries the Harness's service, passing the given
// arguments. QuerySigChainEntries fails if Query returns an error. See
// api.Querier2.QuerySigChainEntries.
func (h *Harness) QuerySigChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[*api.SignatureRecord]] {
	h.TB.Helper()
	r, err := h.Query().QuerySigChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryIdxChainEntry queries the Harness's service, passing the given
// arguments. QueryIdxChainEntry fails if Query returns an error. See
// api.Querier2.QueryIdxChainEntry.
func (h *Harness) QueryIdxChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[*api.IndexEntryRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryIdxChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryIdxChainEntries queries the Harness's service, passing the given
// arguments. QueryIdxChainEntries fails if Query returns an error. See
// api.Querier2.QueryIdxChainEntries.
func (h *Harness) QueryIdxChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[*api.IndexEntryRecord]] {
	h.TB.Helper()
	r, err := h.Query().QueryIdxChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryDataEntry queries the Harness's service, passing the given arguments.
// QueryDataEntry fails if Query returns an error. See
// api.Querier2.QueryDataEntry.
func (h *Harness) QueryDataEntry(scope *url.URL, query *api.DataQuery) *api.ChainEntryRecord[*api.TransactionRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryDataEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryDataEntries queries the Harness's service, passing the given arguments.
// QueryDataEntries fails if Query returns an error. See
// api.Querier2.QueryDataEntries.
func (h *Harness) QueryDataEntries(scope *url.URL, query *api.DataQuery) *api.RecordRange[*api.ChainEntryRecord[*api.TransactionRecord]] {
	h.TB.Helper()
	r, err := h.Query().QueryDataEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryDirectoryUrls queries the Harness's service, passing the given
// arguments. QueryDirectoryUrls fails if Query returns an error. See
// api.Querier2.QueryDirectoryUrls.
func (h *Harness) QueryDirectoryUrls(scope *url.URL, query *api.DirectoryQuery) *api.RecordRange[*api.UrlRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryDirectoryUrls(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryDirectory queries the Harness's service, passing the given arguments.
// QueryDirectory fails if Query returns an error. See
// api.Querier2.QueryDirectory.
func (h *Harness) QueryDirectory(scope *url.URL, query *api.DirectoryQuery) *api.RecordRange[*api.AccountRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryDirectory(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryPendingIds queries the Harness's service, passing the given arguments.
// QueryPendingIds fails if Query returns an error. See
// api.Querier2.QueryPendingIds.
func (h *Harness) QueryPendingIds(scope *url.URL, query *api.PendingQuery) *api.RecordRange[*api.TxIDRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryPendingIds(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryPending queries the Harness's service, passing the given arguments.
// QueryPending fails if Query returns an error. See api.Querier2.QueryPending.
func (h *Harness) QueryPending(scope *url.URL, query *api.PendingQuery) *api.RecordRange[*api.TransactionRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryPending(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryMinorBlock queries the Harness's service, passing the given arguments.
// QueryMinorBlock fails if Query returns an error. See
// api.Querier2.QueryMinorBlock.
func (h *Harness) QueryMinorBlock(scope *url.URL, query *api.BlockQuery) *api.MinorBlockRecord {
	h.TB.Helper()
	r, err := h.Query().QueryMinorBlock(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryMinorBlocks queries the Harness's service, passing the given arguments.
// QueryMinorBlocks fails if Query returns an error. See
// api.Querier2.QueryMinorBlocks.
func (h *Harness) QueryMinorBlocks(scope *url.URL, query *api.BlockQuery) *api.RecordRange[*api.MinorBlockRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryMinorBlocks(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryMajorBlock queries the Harness's service, passing the given arguments.
// QueryMajorBlock fails if Query returns an error. See
// api.Querier2.QueryMajorBlock.
func (h *Harness) QueryMajorBlock(scope *url.URL, query *api.BlockQuery) *api.MajorBlockRecord {
	h.TB.Helper()
	r, err := h.Query().QueryMajorBlock(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryMajorBlocks queries the Harness's service, passing the given arguments.
// QueryMajorBlocks fails if Query returns an error. See
// api.Querier2.QueryMajorBlocks.
func (h *Harness) QueryMajorBlocks(scope *url.URL, query *api.BlockQuery) *api.RecordRange[*api.MajorBlockRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryMajorBlocks(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// SearchForAnchor queries the Harness's service, passing the given arguments.
// SearchForAnchor fails if Query returns an error. See
// api.Querier2.SearchForAnchor.
func (h *Harness) SearchForAnchor(scope *url.URL, search *api.AnchorSearchQuery) *api.RecordRange[*api.ChainEntryRecord[api.Record]] {
	h.TB.Helper()
	r, err := h.Query().SearchForAnchor(context.Background(), scope, search)
	require.NoError(h.TB, err)
	return r
}

// SearchForPublicKey queries the Harness's service, passing the given
// arguments. SearchForPublicKey fails if Query returns an error. See
// api.Querier2.SearchForPublicKey.
func (h *Harness) SearchForPublicKey(scope *url.URL, search *api.PublicKeySearchQuery) *api.RecordRange[*api.KeyRecord] {
	h.TB.Helper()
	r, err := h.Query().SearchForPublicKey(context.Background(), scope, search)
	require.NoError(h.TB, err)
	return r
}

// SearchForPublicKeyHash queries the Harness's service, passing the given
// arguments. SearchForPublicKeyHash fails if Query returns an error. See
// api.Querier2.SearchForPublicKeyHash.
func (h *Harness) SearchForPublicKeyHash(scope *url.URL, search *api.PublicKeyHashSearchQuery) *api.RecordRange[*api.KeyRecord] {
	h.TB.Helper()
	r, err := h.Query().SearchForPublicKeyHash(context.Background(), scope, search)
	require.NoError(h.TB, err)
	return r
}

// SearchForDelegate queries the Harness's service, passing the given arguments.
// SearchForDelegate fails if Query returns an error. See
// api.Querier2.SearchForDelegate.
func (h *Harness) SearchForDelegate(scope *url.URL, search *api.DelegateSearchQuery) *api.RecordRange[*api.KeyRecord] {
	h.TB.Helper()
	r, err := h.Query().SearchForDelegate(context.Background(), scope, search)
	require.NoError(h.TB, err)
	return r
}

// SearchForTransactionHash queries the Harness's service, passing the given
// arguments. SearchForTransactionHash fails if Query returns an error. See
// api.Querier2.SearchForTransactionHash.
func (h *Harness) SearchForTransactionHash(scope *url.URL, search *api.MessageHashSearchQuery) *api.RecordRange[*api.TxIDRecord] {
	h.TB.Helper()
	r, err := h.Query().SearchForTransactionHash(context.Background(), scope, search)
	require.NoError(h.TB, err)
	return r
}
