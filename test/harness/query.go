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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
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

// ConsensusStatus calls the Harness's service, failing if the call returns an
// error.
func (h *Harness) ConsensusStatus(opts api.ConsensusStatusOptions) *api.ConsensusStatus {
	ns, err := h.services.ConsensusStatus(context.Background(), opts)
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

// Network returns the Harness's service as an api.NetworkService.
func (h *Harness) Network() api.NetworkService {
	return h.services
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

// QuerySignature queries the Harness's service, passing the given arguments.
// QuerySignature fails if Query returns an error. See
// api.Querier2.QuerySignature.
func (h *Harness) QuerySignature(txid *url.TxID, query *api.DefaultQuery) *api.MessageRecord[*messaging.SignatureMessage] {
	h.TB.Helper()
	r, err := h.Query().QuerySignature(context.Background(), txid, query)
	require.NoError(h.TB, err)
	return r
}

// QueryMessage queries the Harness's service, passing the given arguments.
// QueryMessage fails if Query returns an error. See
// api.Querier2.QueryMessage.
func (h *Harness) QueryMessage(txid *url.TxID, query *api.DefaultQuery) *api.MessageRecord[messaging.Message] {
	h.TB.Helper()
	r, err := h.Query().QueryMessage(context.Background(), txid, query)
	require.NoError(h.TB, err)
	return r
}

// QueryTransaction queries the Harness's service, passing the given arguments.
// QueryTransaction fails if Query returns an error. See
// api.Querier2.QueryTransaction.
func (h *Harness) QueryTransaction(txid *url.TxID, query *api.DefaultQuery) *api.MessageRecord[*messaging.TransactionMessage] {
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

// QueryMainChainEntry queries the Harness's service, passing the given
// arguments. QueryMainChainEntry fails if Query returns an error. See
// api.Querier2.QueryMainChainEntry.
func (h *Harness) QueryMainChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]] {
	h.TB.Helper()
	r, err := h.Query().QueryMainChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryMainChainEntries queries the Harness's service, passing the given
// arguments. QueryMainChainEntries fails if Query returns an error. See
// api.Querier2.QueryMainChainEntries.
func (h *Harness) QueryMainChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]]] {
	h.TB.Helper()
	r, err := h.Query().QueryMainChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QuerySignatureChainEntry queries the Harness's service, passing the given
// arguments. QuerySignatureChainEntry fails if Query returns an error. See
// api.Querier2.QuerySignatureChainEntry.
func (h *Harness) QuerySignatureChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[*api.MessageRecord[messaging.Message]] {
	h.TB.Helper()
	r, err := h.Query().QuerySignatureChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QuerySignatureChainEntries queries the Harness's service, passing the given
// arguments. QuerySignatureChainEntries fails if Query returns an error. See
// api.Querier2.QuerySignatureChainEntries.
func (h *Harness) QuerySignatureChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[*api.MessageRecord[messaging.Message]]] {
	h.TB.Helper()
	r, err := h.Query().QuerySignatureChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryIndexChainEntry queries the Harness's service, passing the given
// arguments. QueryIndexChainEntry fails if Query returns an error. See
// api.Querier2.QueryIndexChainEntry.
func (h *Harness) QueryIndexChainEntry(scope *url.URL, query *api.ChainQuery) *api.ChainEntryRecord[*api.IndexEntryRecord] {
	h.TB.Helper()
	r, err := h.Query().QueryIndexChainEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryIndexChainEntries queries the Harness's service, passing the given
// arguments. QueryIndexChainEntries fails if Query returns an error. See
// api.Querier2.QueryIndexChainEntries.
func (h *Harness) QueryIndexChainEntries(scope *url.URL, query *api.ChainQuery) *api.RecordRange[*api.ChainEntryRecord[*api.IndexEntryRecord]] {
	h.TB.Helper()
	r, err := h.Query().QueryIndexChainEntries(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryDataEntry queries the Harness's service, passing the given arguments.
// QueryDataEntry fails if Query returns an error. See
// api.Querier2.QueryDataEntry.
func (h *Harness) QueryDataEntry(scope *url.URL, query *api.DataQuery) *api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]] {
	h.TB.Helper()
	r, err := h.Query().QueryDataEntry(context.Background(), scope, query)
	require.NoError(h.TB, err)
	return r
}

// QueryDataEntries queries the Harness's service, passing the given arguments.
// QueryDataEntries fails if Query returns an error. See
// api.Querier2.QueryDataEntries.
func (h *Harness) QueryDataEntries(scope *url.URL, query *api.DataQuery) *api.RecordRange[*api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]]] {
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
func (h *Harness) QueryPending(scope *url.URL, query *api.PendingQuery) *api.RecordRange[*api.MessageRecord[*messaging.TransactionMessage]] {
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
func (h *Harness) SearchForMessage(ctx context.Context, hash [32]byte) *api.RecordRange[*api.TxIDRecord] {
	h.TB.Helper()
	r, err := h.Query().SearchForMessage(context.Background(), hash)
	require.NoError(h.TB, err)
	return r
}
