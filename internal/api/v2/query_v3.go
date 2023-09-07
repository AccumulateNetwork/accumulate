// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (m *JrpcMethods) netq() api.Querier2 {
	return api.Querier2{Querier: m.Querier}
}

func rangeOptsV3(p *QueryPagination, o *QueryOptions) *api.RangeOptions {
	r := new(api.RangeOptions)
	r.Start = p.Start
	r.Count = &p.Count
	if o != nil {
		r.Expand = &o.Expand
	}
	return r
}

func recordIs[T any](r api.Record, err error) (T, error) {
	var z T
	if err != nil {
		return z, err
	}
	if v, ok := r.(T); ok {
		return v, nil
	}
	return z, fmt.Errorf("rpc returned unexpected type: want %T, got %T", z, r)
}

func chainEntryOf[T api.Record](r api.Record, err error) (*api.ChainEntryRecord[T], error) {
	cr, err := recordIs[*api.ChainEntryRecord[api.Record]](r, err)
	if err != nil {
		return nil, err
	}
	return api.ChainEntryRecordAs[T](cr)
}

func rangeOf[T api.Record](r api.Record, err error) (*api.RecordRange[T], error) {
	rr, err := recordIs[*api.RecordRange[api.Record]](r, err)
	if err != nil {
		return nil, err
	}
	return api.MapRange(rr, func(r api.Record) (T, error) {
		if v, ok := r.(T); ok {
			return v, nil
		}
		var z T
		return z, fmt.Errorf("rpc returned unexpected type: want %T, got %T", z, r)
	})
}

func chainRangeOf[T api.Record](r api.Record, err error) (*api.RecordRange[*api.ChainEntryRecord[T]], error) {
	rr, err := recordIs[*api.RecordRange[api.Record]](r, err)
	if err != nil {
		return nil, err
	}
	return api.MapRange(rr, func(r api.Record) (*api.ChainEntryRecord[T], error) {
		return chainEntryOf[T](r, nil)
	})
}

func chainStateV3(r *api.ChainRecord) *ChainState {
	cs := new(ChainState)
	cs.Height = r.Count
	cs.Name = r.Name
	cs.Type = r.Type
	cs.Roots = r.State
	return cs
}

func chainRespV3(account *api.AccountRecord, chains *api.RecordRange[*api.ChainRecord]) *ChainQueryResponse {
	res := new(ChainQueryResponse)
	if account != nil {
		res.Type = account.Account.Type().String()
		res.Data = account.Account
		res.Receipt = receiptV3(account.Receipt)
		res.ChainId = account.Account.GetUrl().AccountID()
	}
	if chains != nil {
		for _, chain := range chains.Records {
			res.Chains = append(res.Chains, *chainStateV3(chain))
			if chain.Name == protocol.MainChain {
				res.MainChain = new(MerkleState)
				res.MainChain.Height = chain.Count
				res.MainChain.Roots = chain.State
			}
		}
	}
	return res
}

func chainEntryV3[T api.Record](r *api.ChainEntryRecord[T]) *ChainQueryResponse {
	resp := new(ChainQueryResponse)
	resp.Type = "chainEntry"
	resp.Receipt = receiptV3(r.Receipt)
	resp.MainChain = &MerkleState{Height: r.Index, Roots: r.State}

	entry := new(ChainEntry)
	entry.Height = r.Index
	entry.Entry = r.Entry[:]
	entry.State = r.State
	resp.Data = entry

	switch r.Type {
	default:
		return resp
	case protocol.ChainTypeIndex:
		v := new(protocol.IndexEntry)
		if v.UnmarshalBinary(entry.Entry) == nil {
			entry.Value = v
		}
	}

	return resp
}

func receiptV3(r *api.Receipt) *GeneralReceipt {
	if r == nil {
		return nil
	}
	gr := new(GeneralReceipt)
	gr.Proof = r.Receipt
	gr.LocalBlock = r.LocalBlock
	gr.LocalBlockTime = &r.LocalBlockTime
	gr.MajorBlock = r.MajorBlock
	return gr
}

func uint64p(v uint64) *uint64 { return &v }

func txReceiptV3[T api.Record](r *api.ChainEntryRecord[T]) *TxReceipt {
	if r.Receipt == nil {
		return nil
	}

	txr := new(TxReceipt)
	txr.Account = r.Account
	txr.Chain = r.Name
	txr.GeneralReceipt = *receiptV3(r.Receipt)
	return txr
}

func dataEntryV3(r *api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]]) *ResponseDataEntry {
	rde := new(ResponseDataEntry)
	rde.EntryHash = r.Entry
	if r.Value == nil {
		return rde
	}

	rde.TxId = r.Value.ID

	switch body := r.Value.Message.Transaction.Body.(type) {
	case *protocol.WriteData:
		rde.Entry = body.Entry
	case *protocol.WriteDataTo:
		rde.Entry = body.Entry
	case *protocol.SyntheticWriteData:
		rde.Entry = body.Entry
		rde.CauseTxId = body.Cause
	case *protocol.SystemWriteData:
		rde.Entry = body.Entry
	}
	return rde
}

func chainQueryV3(qv url.Values, arg string) (*api.RangeOptions, *uint64, []byte, error) {
	if arg == "" {
		rng := new(api.RangeOptions)
		if s := qv.Get("start"); s != "" {
			v, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("invalid start: %v", err)
			}
			rng.Start = v
		}

		if s := qv.Get("count"); s != "" {
			v, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("invalid count: %v", err)
			}
			rng.Count = &v
		} else {
			v := uint64(10)
			rng.Count = &v
		}

		return rng, nil, nil, nil
	}

	i := strings.IndexByte(arg, ':')
	if i < 0 {
		height, e1 := strconv.ParseUint(arg, 10, 64)
		entry, e2 := hex.DecodeString(arg)
		switch {
		case e1 == nil:
			return nil, &height, nil, nil
		case e2 == nil:
			return nil, nil, entry, nil
		default:
			return nil, nil, nil, fmt.Errorf("invalid entry: %q is not a number or a hash", arg)
		}
	}

	v, err := strconv.ParseUint(arg[:i], 10, 64)
	if err != nil {
		return nil, nil, nil, err
	}
	u, err := strconv.ParseUint(arg[i+1:], 10, 64)
	if err != nil {
		return nil, nil, nil, err
	}
	u += v

	rng := new(api.RangeOptions)
	rng.Start = v
	rng.Count = &u
	return rng, nil, nil, nil
}

func waitFor[T any](query func() (T, error), wait, max time.Duration) (T, error) {
	var start time.Time
	var sleepIncr time.Duration
	var sleep time.Duration
	if wait < time.Second/2 {
		wait = 0
	} else {
		if wait > max {
			wait = max
		}
		sleepIncr = wait / 50
		sleep = sleepIncr
		start = time.Now()
	}

query:
	var z T
	for {
		res, err := query()
		switch {
		case err == nil:
			// Found
			return res, nil

		case !errors.Is(err, errors.NotFound):
			// Unknown error
			return z, err

		case wait == 0 || time.Since(start) > wait:
			// Not found or pending, wait not specified or exceeded
			return z, errors.UnknownError.Wrap(err)

		default:
			// Not found or pending, try again, linearly increasing the wait time
			time.Sleep(sleep)
			sleep += sleepIncr
			goto query
		}
	}
}

func queryTx(v3 api.Querier, ctx context.Context, txid *url.TxID, includeReceipt, ignorePending, resolveSignature bool) (*TransactionQueryResponse, error) {
	r, err := api.Querier2{Querier: v3}.QueryMessage(ctx, txid, &api.DefaultQuery{IncludeReceipt: includeReceipt})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	switch msg := r.Message.(type) {
	case *messaging.TransactionMessage:
		if ignorePending && r.Status == errors.Pending {
			return nil, errors.NotFound.WithFormat("%v is pending", txid)
		}
		if !includeReceipt {
			return transactionV3(r, msg)
		}
		return txnAndReceiptV3(v3, ctx, r, msg)

	case *messaging.SignatureMessage:
		if resolveSignature {
			return queryTx(v3, ctx, msg.TxID, includeReceipt, ignorePending, true)
		}
		if !includeReceipt {
			return signatureV3(r, msg)
		}
		return sigAndReceiptV3(v3, ctx, r, msg)

	default:
		return nil, errors.BadRequest.WithFormat("%v is a %v, not a transaction or signature", r.ID, r.Message.Type())
	}
}

func (m *JrpcMethods) QueryDirectory(ctx context.Context, params json.RawMessage) any {
	req := new(DirectoryQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	r, err := recordIs[*api.RecordRange[api.Record]](m.Querier.Query(ctx, req.Url, &api.DirectoryQuery{
		Range: rangeOptsV3(&req.QueryPagination, &req.QueryOptions),
	}))
	if err != nil {
		return accumulateError(err)
	}

	res := new(MultiResponse)
	res.Type = "directory"
	res.Start = req.Start
	res.Count = req.Count
	res.Total = r.Total

	for _, entry := range r.Records {
		switch entry := entry.(type) {
		case *api.UrlRecord:
			res.Items = append(res.Items, entry.Value.String())
		case *api.AccountRecord:
			res.Items = append(res.Items, entry.Account.GetUrl().String())
			res.OtherItems = append(res.OtherItems, chainRespV3(entry, nil))
		}
	}
	return res
}

func transactionV3[T messaging.Message](r *api.MessageRecord[T], txn *messaging.TransactionMessage) (*TransactionQueryResponse, error) {
	res := new(TransactionQueryResponse)
	res.Type = txn.Transaction.Body.Type().String()
	res.Data = txn.Transaction.Body
	h := r.ID.Hash()
	res.Txid = r.ID
	res.Status = &protocol.TransactionStatus{
		TxID:     r.ID,
		Code:     r.Status,
		Error:    r.Error,
		Result:   r.Result,
		Received: r.Received,
	}
	res.TransactionHash = h[:]
	res.Transaction = txn.Transaction
	if r.Produced != nil {
		for _, r := range r.Produced.Records {
			res.Produced = append(res.Produced, r.Value)
		}
	}
	if r.Sequence != nil {
		res.Status.SourceNetwork = r.Sequence.Source
		res.Status.DestinationNetwork = r.Sequence.Destination
		res.Status.SequenceNumber = r.Sequence.Number
	}
	if r.SourceReceipt != nil {
		res.Status.Proof = r.SourceReceipt
	}

	if r.Signatures != nil {
		for _, set := range r.Signatures.Records {
			if signer, ok := set.Account.(protocol.Signer); ok {
				res.Status.Signers = append(res.Status.Signers, signer)
			}

			if set.Signatures != nil {
				for _, msg := range set.Signatures.Records {
					switch msg := msg.Message.(type) {
					case *messaging.CreditPayment:
						if msg.Initiator {
							res.Status.Initiator = msg.Payer
						}
					case *messaging.BlockAnchor:
						res.Status.AnchorSigners = append(res.Status.AnchorSigners, msg.Signature.GetPublicKey())
					}
				}
			}
		}
	}

	switch payload := txn.Transaction.Body.(type) {
	case *protocol.SendTokens:
		produced := map[[32]byte]*url.TxID{}
		for _, id := range res.Produced {
			produced[id.Account().AccountID32()] = id
		}

		res.Origin = txn.Transaction.Header.Principal
		data := new(TokenSend)
		data.From = txn.Transaction.Header.Principal
		data.To = make([]TokenDeposit, len(payload.To))
		for i, to := range payload.To {
			data.To[i].Url = to.Url
			data.To[i].Amount = to.Amount
			if id, ok := produced[to.Url.AccountID32()]; ok {
				h := id.Hash()
				data.To[i].Txid = h[:]
			}
		}

		res.Origin = txn.Transaction.Header.Principal
		res.Data = data

	case *protocol.SyntheticDepositTokens:
		res.Origin = txn.Transaction.Header.Principal
		res.Data = payload

	default:
		res.Origin = txn.Transaction.Header.Principal
		res.Data = payload
	}

	books := map[string]*SignatureBook{}
	for _, r := range r.Signatures.Records {
		if _, ok := r.Account.(protocol.Signer); !ok {
			continue
		}

		var book *SignatureBook
		signerUrl := r.Account.GetUrl()
		bookUrl, _, ok := protocol.ParseKeyPageUrl(signerUrl)
		if !ok {
			book = new(SignatureBook)
			book.Authority = signerUrl
			res.SignatureBooks = append(res.SignatureBooks, book)
		} else if book, ok = books[bookUrl.String()]; !ok {
			book = new(SignatureBook)
			book.Authority = bookUrl
			res.SignatureBooks = append(res.SignatureBooks, book)
		}

		page := new(SignaturePage)
		book.Pages = append(book.Pages, page)
		page.Signer.Type = r.Account.Type()
		page.Signer.Url = signerUrl

		keyPage, ok := r.Account.(*protocol.KeyPage)
		if ok {
			page.Signer.AcceptThreshold = keyPage.AcceptThreshold
		}

		if r.Signatures == nil {
			continue
		}

		for _, r := range r.Signatures.Records {
			msg, ok := r.Message.(*messaging.SignatureMessage)
			if !ok {
				continue
			}
			page.Signatures = append(page.Signatures, msg.Signature)
			res.Signatures = append(res.Signatures, msg.Signature)
		}
	}

	return res, nil
}

func signatureV3(r *api.MessageRecord[messaging.Message], sig *messaging.SignatureMessage) (*TransactionQueryResponse, error) {
	res := new(TransactionQueryResponse)
	res.Type = protocol.TransactionTypeRemote.String()
	res.Data = &protocol.RemoteTransaction{Hash: sig.Signature.GetTransactionHash()}
	res.Txid = sig.TxID
	res.Status = &protocol.TransactionStatus{
		TxID:     r.ID,
		Code:     r.Status,
		Error:    r.Error,
		Result:   r.Result,
		Received: r.Received,
	}
	res.TransactionHash = sig.TxID.HashSlice()
	res.Transaction = &protocol.Transaction{Body: &protocol.RemoteTransaction{Hash: sig.Signature.GetTransactionHash()}}
	if r.Produced != nil {
		for _, r := range r.Produced.Records {
			res.Produced = append(res.Produced, r.Value)
		}
	}

	res.Signatures = []protocol.Signature{sig.Signature}
	res.SignatureBooks = []*SignatureBook{{
		Pages: []*SignaturePage{{
			Signatures: []protocol.Signature{sig.Signature},
		}},
	}}

	if sig.Signature.Type().IsSystem() {
		res.SignatureBooks[0].Authority = protocol.DnUrl().JoinPath(protocol.Operators)
		res.SignatureBooks[0].Pages[0].Signer.Url = protocol.DnUrl().JoinPath(protocol.Network)
	} else {
		if book, _, ok := protocol.ParseKeyPageUrl(sig.Signature.GetSigner()); ok {
			res.SignatureBooks[0].Authority = book
		} else {
			res.SignatureBooks[0].Authority = sig.Signature.GetSigner().RootIdentity()
		}
		res.SignatureBooks[0].Pages[0].Signer.Url = sig.Signature.GetSigner()
	}

	return res, nil
}

func txnAndReceiptV3(v3 api.Querier, ctx context.Context, r *api.MessageRecord[messaging.Message], txn *messaging.TransactionMessage) (*TransactionQueryResponse, error) {
	res, err := transactionV3(r, txn)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if res.Transaction.Header.Principal == nil {
		return res, nil
	}

	// Get a receipt from the main chain
	r2, err := chainRangeOf[api.Record](v3.Query(ctx, txn.ID().AsUrl(), &api.ChainQuery{IncludeReceipt: true}))
	switch {
	case err == nil:
		for _, r := range r2.Records {
			res.Receipts = append(res.Receipts, txReceiptV3(r))
		}
	case errors.Is(err, errors.NotFound):
		// Ignore
	default:
		return nil, errors.UnknownError.Wrap(err)
	}
	return res, nil
}

func sigAndReceiptV3(v3 api.Querier, ctx context.Context, r *api.MessageRecord[messaging.Message], sig *messaging.SignatureMessage) (*TransactionQueryResponse, error) {
	res, err := signatureV3(r, sig)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if res.Transaction.Header.Principal == nil {
		return res, nil
	}

	if sig.Signature.Type().IsSystem() {
		return res, nil // System signatures do not support getting receipts
	}

	// Get a receipt from the signature chain
	r2, err := api.Querier2{Querier: v3}.QuerySignatureChainEntry(ctx, sig.Signature.GetSigner(), &api.ChainQuery{Name: "signature", Entry: sig.Signature.Hash()})
	switch {
	case err == nil:
		if r2.Receipt != nil {
			res.Receipts = []*TxReceipt{txReceiptV3(r2)}
		}
	case errors.Is(err, errors.NotFound):
		// Ignore
	default:
		return nil, errors.UnknownError.Wrap(err)
	}
	return res, nil
}

func chainTxnV3(r *api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]]) (*TransactionQueryResponse, error) {
	res, err := transactionV3(r.Value, r.Value.Message)
	if err != nil {
		return nil, err
	}

	// res.MainChain = ms
	// res.Receipts = qrResp.Receipts

	// This is only one receipt, is that OK?
	if r.Receipt != nil {
		res.Receipts = []*TxReceipt{txReceiptV3(r)}
	}

	return res, nil
}

func txnOrSigV3(v3 api.Querier, ctx context.Context, r *api.ChainEntryRecord[*api.MessageRecord[messaging.Message]], prove bool) (*TransactionQueryResponse, error) {
	var res *TransactionQueryResponse
	var err error
	switch msg := r.Value.Message.(type) {
	case *messaging.TransactionMessage:
		res, err = transactionV3(r.Value, msg)
	case *messaging.SignatureMessage:
		res, err = queryTx(v3, ctx, msg.TxID, prove, false, true)
	default:
		return nil, fmt.Errorf("invalid value type %v", msg.Type())
	}
	if err != nil {
		return nil, err
	}
	if r.Receipt != nil {
		res.Receipts = []*TxReceipt{txReceiptV3(r)}
	}
	return res, nil
}

func (m *JrpcMethods) QueryKeyPageIndex(ctx context.Context, params json.RawMessage) any {
	req := new(KeyPageIndexQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	r, err := m.netq().SearchForPublicKeyHash(ctx, req.Url, &api.PublicKeyHashSearchQuery{
		PublicKeyHash: req.Key,
	})
	if err == nil && len(r.Records) == 0 {
		r, err = m.netq().SearchForPublicKey(ctx, req.Url, &api.PublicKeySearchQuery{
			PublicKey: req.Key,
			Type:      protocol.SignatureTypeED25519,
		})
	}
	if err != nil {
		return accumulateError(err)
	}
	if len(r.Records) == 0 {
		return accumulateError(errors.UnknownError.WithFormat("no authority of %s holds %X", req.Url, req.Key))
	}

	qr := new(ResponseKeyPageIndex)
	qr.Authority = r.Records[0].Authority
	qr.Signer = r.Records[0].Signer
	_, qr.Index, _ = protocol.ParseKeyPageUrl(qr.Signer)
	qr.Index--

	res := new(ChainQueryResponse)
	res.Data = qr
	res.Type = "key-page-index"
	return res
}

func (m *JrpcMethods) QueryData(ctx context.Context, params json.RawMessage) any {
	req := new(DataEntryQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	// List chains
	r1, err := m.netq().QueryAccountChains(ctx, req.Url, nil)
	if err != nil {
		return accumulateError(err)
	}

	q := new(api.DataQuery)
	if req.EntryHash != [32]byte{} {
		q.Entry = req.EntryHash[:]
	}

	// Get the data entry
	r2, err := m.netq().QueryDataEntry(ctx, req.Url, q)
	if err != nil {
		return accumulateError(err)
	}

	qr := chainRespV3(nil, r1)
	qr.ChainId = req.Url.AccountID()
	qr.Type = "dataEntry"
	qr.Data = dataEntryV3(r2)
	return qr
}

func (m *JrpcMethods) QueryDataSet(ctx context.Context, params json.RawMessage) any {
	req := new(DataEntrySetQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	r, err := m.netq().QueryDataEntries(ctx, req.Url, &api.DataQuery{Range: rangeOptsV3(&req.QueryPagination, &req.QueryOptions)})
	if err != nil {
		return accumulateError(err)
	}

	res := new(MultiResponse)
	res.Type = "dataSet"
	res.Start = r.Start
	res.Count = req.Count
	res.Total = r.Total
	for _, entry := range r.Records {
		res.Items = append(res.Items, dataEntryV3(entry))
	}
	return res
}

func (m *JrpcMethods) QueryTxHistory(ctx context.Context, params json.RawMessage) any {
	req := new(TxHistoryQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	q := new(api.ChainQuery)
	expand := true
	q.Range = &api.RangeOptions{Start: req.Start, Count: &req.Count, Expand: &expand}
	if req.Scratch {
		q.Name = "scratch"
	} else {
		q.Name = "main"
	}

	r, err := m.netq().QueryMainChainEntries(ctx, req.Url, q)
	if err != nil {
		return accumulateError(err)
	}

	res := new(MultiResponse)
	res.Type = "txHistory"
	res.Items = make([]any, len(r.Records))
	res.Start = req.Start
	res.Count = req.Count
	res.Total = r.Total
	for i, r := range r.Records {
		if r.Value == nil {
			continue
		}
		res.Items[i], err = chainTxnV3(r)
		if err != nil {
			return accumulateError(err)
		}
	}
	return res
}

func (m *JrpcMethods) QueryTxLocal(ctx context.Context, params json.RawMessage) any {
	req := new(TxnQuery)
	err := m.parse(params, req)
	if err != nil {
		return accumulateError(err)
	}

	var txid *url.TxID
	switch {
	case len(req.Txid) == 32:
		txid = (&url.URL{Authority: protocol.Unknown}).WithTxID(*(*[32]byte)(req.Txid))
	case req.TxIdUrl != nil:
		txid = req.TxIdUrl
	case len(req.Txid) != 0:
		return accumulateError(errors.BadRequest.WithFormat("invalid transaction hash length: want 32, got %d", len(req.Txid)))
	default:
		return accumulateError(errors.BadRequest.WithFormat("no transaction ID present in request"))
	}

	return jrpcFormatResponse(waitFor(func() (*TransactionQueryResponse, error) {
		return queryTx(m.LocalV3, ctx, txid, req.Prove, req.IgnorePending, true)
	}, req.Wait, m.TxMaxWaitTime))
}

// Query queries an account or account chain by URL.
func (m *JrpcMethods) Query(ctx context.Context, params json.RawMessage) any {
	req := new(GeneralQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	qv := req.Url.QueryValues()
	switch {
	case qv.Get("txid") != "":
		// Query by transaction ID
		txid, err := hex.DecodeString(qv.Get("txid"))
		if err != nil {
			return accumulateError(fmt.Errorf("invalid txid %q: %v", qv.Get("txid"), err))
		}
		if len(txid) != 32 {
			return accumulateError(fmt.Errorf("invalid txid %q: not 32 bytes", qv.Get("txid")))
		}

		return jrpcFormatResponse(queryTx(m.Querier, ctx, req.Url.WithTxID(*(*[32]byte)(txid)), req.Prove, false, true))

	case req.Url.Fragment == "":
		txid, err := req.Url.AsTxID()
		if err == nil {
			return jrpcFormatResponse(queryTx(m.Querier, ctx, txid, req.Prove, false, false))
		}

		acct, err := m.netq().QueryAccount(ctx, req.Url, &api.DefaultQuery{IncludeReceipt: req.Prove})
		if err != nil {
			return accumulateError(err)
		}

		chains, err := m.netq().QueryAccountChains(ctx, req.Url, nil)
		if err != nil {
			return accumulateError(err)
		}

		return chainRespV3(acct, chains)
	}

	var chainName, chainArg string
	var chainTx bool
	fragment := strings.Split(req.Url.Fragment, "/")
	switch fragment[0] {
	case "anchor":
		if len(fragment) < 2 {
			return accumulateError(fmt.Errorf("invalid fragment"))
		}

		entryHash, err := hex.DecodeString(fragment[1])
		if err != nil {
			return accumulateError(fmt.Errorf("invalid entry: %q is not a hash", fragment[1]))
		}

		r, err := m.netq().SearchForAnchor(ctx, req.Url, &api.AnchorSearchQuery{Anchor: entryHash, IncludeReceipt: true})
		if err != nil {
			return accumulateError(err)
		}

		return chainEntryV3(r.Records[0])

	case "tx", "txn", "transaction":
		chainTx = true
		if req.Scratch {
			chainName = "scratch"
		} else {
			chainName = "main"
		}
		switch len(fragment) {
		case 1:
			goto chain_query
		case 2:
			chainArg = fragment[1]
			goto chain_query
		}

	case "signature":
		chainTx = true
		chainName = "signature"
		switch len(fragment) {
		case 1:
			chainName = "main" // Replicate API v2 bug
			goto chain_query
		case 2:
			chainArg = fragment[1]
			goto chain_query
		}

	case "chain":
		switch len(fragment) {
		case 3:
			chainArg = fragment[2]
			fallthrough
		case 2:
			chainName = fragment[1]
			goto chain_query
		}

	case "pending":
		switch len(fragment) {
		case 1:
			r, err := m.netq().QueryPendingIds(ctx, req.Url, &api.PendingQuery{Range: &api.RangeOptions{Count: uint64p(100)}})
			if err != nil {
				return accumulateError(err)
			}

			res := new(MultiResponse)
			res.Type = "pending"
			res.Total = r.Total
			res.Items = make([]any, len(r.Records))
			for i, txid := range r.Records {
				txid := txid.Value.Hash()
				res.Items[i] = hex.EncodeToString(txid[:])
			}
			return res

		case 2:
			chainTx = true
			chainName = "signature"
			chainArg = fragment[1]
			goto chain_query
		}

	case "data":
		if len(fragment) > 2 {
			goto invalid
		}

		var err error
		q := new(api.DataQuery)
		if qv.Has("start") || qv.Has("count") {
			q.Range, q.Index, q.Entry, err = chainQueryV3(qv, "")
		} else if len(fragment) > 1 {
			q.Range, q.Index, q.Entry, err = chainQueryV3(qv, fragment[1])
		}
		if err != nil {
			return accumulateError(err)
		}

		if q.Range == nil {
			r, err := m.netq().QueryDataEntry(ctx, req.Url, q)
			if err != nil {
				return accumulateError(err)
			}

			res := new(ChainQueryResponse)
			res.Type = "dataEntry"
			res.Data = dataEntryV3(r)
			return res
		}

		r, err := m.netq().QueryDataEntries(ctx, req.Url, q)
		if err != nil {
			return accumulateError(err)
		}

		res := new(ResponseDataEntrySet)
		res.Total = r.Total
		for _, r := range r.Records {
			res.DataEntries = append(res.DataEntries, *dataEntryV3(r))
		}

		return &ChainQueryResponse{Data: res, Type: "dataEntry"}
	}

invalid:
	return accumulateError(fmt.Errorf("invalid fragment"))

chain_query:
	q := new(api.ChainQuery)
	q.Name = chainName
	q.IncludeReceipt = req.Prove
	q.Range, q.Index, q.Entry, err = chainQueryV3(qv, chainArg)
	if err != nil {
		return accumulateError(err)
	}

	if q.Range != nil {
		q.Range.Expand = &chainTx
	}

	r, err := m.Querier.Query(ctx, req.Url, q)
	if err != nil {
		return accumulateError(err)
	}

	if q.Range == nil {
		cr, ok := r.(*api.ChainEntryRecord[api.Record])
		if !ok {
			return accumulateError(fmt.Errorf("rpc returned unexpected type: want %T, got %T", (*api.ChainEntryRecord[api.Record])(nil), cr))
		}
		if !chainTx {
			return chainEntryV3(cr)
		}
		cr2, err := api.ChainEntryRecordAs[*api.MessageRecord[messaging.Message]](cr)
		if err != nil {
			return accumulateError(err)
		}
		res, err := txnOrSigV3(m.Querier, ctx, cr2, req.Prove)
		if err != nil {
			return accumulateError(err)
		}
		res.MainChain = &MerkleState{Height: cr.Index, Roots: cr.State}
		return res
	}

	rr, ok := r.(*api.RecordRange[api.Record])
	if !ok {
		return accumulateError(fmt.Errorf("rpc returned unexpected type: want %T, got %T", (*api.RecordRange[api.Record])(nil), r))
	}
	rs, err := api.RecordRangeAs[*api.ChainEntryRecord[api.Record]](rr)
	if err != nil {
		return accumulateError(err)
	}

	resp := new(MultiResponse)
	resp.Start = q.Range.Start
	resp.Count = *q.Range.Count
	resp.Total = rr.Total

	if chainTx {
		resp.Type = "txHistory"
		for _, cr := range rs.Records {
			cr, err := api.ChainEntryRecordAs[*api.MessageRecord[messaging.Message]](cr)
			if err != nil {
				return accumulateError(err)
			}
			txres, err := txnOrSigV3(m.Querier, ctx, cr, req.Prove)
			if err != nil {
				return accumulateError(err)
			}
			resp.Items = append(resp.Items, txres)
		}
		return resp
	}

	resp.Type = "chainEntrySet"

	for _, entry := range rs.Records {
		qr := new(ChainQueryResponse)
		qr.Type = "hex"
		qr.Data = hex.EncodeToString(entry.Entry[:])
		resp.Items = append(resp.Items, qr)
	}

	if len(rs.Records) > 0 && rs.Records[0].Type == merkle.ChainTypeIndex {
		resp.OtherItems = make([]interface{}, len(rs.Records))
		for i, entry := range rs.Records {
			v := new(protocol.IndexEntry)
			err := v.UnmarshalBinary(entry.Entry[:])
			if err == nil {
				resp.Items[i] = v
			}
		}
	}
	return resp
}

func (m *JrpcMethods) QueryMinorBlocks(ctx context.Context, params json.RawMessage) interface{} {
	req := new(MinorBlocksQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	// API v2 and v3 have different definitions of what block counts, so we
	// can't use req.Count
	q := new(api.BlockQuery)
	q.MinorRange = &api.RangeOptions{Start: req.Start, Count: uint64p(100)}
	if req.BlockFilterMode == BlockFilterModeExcludeEmpty {
		q.OmitEmpty = true
	}
	r, err := m.netq().QueryMinorBlocks(ctx, req.Url, q)
	if err != nil {
		return accumulateError(err)
	}

	var wantIds, wantCount, wantTx bool
	switch req.TxFetchMode {
	case TxFetchModeExpand:
		wantTx = true
		fallthrough
	case TxFetchModeIds:
		wantIds = true
		fallthrough
	case TxFetchModeCountOnly:
		wantCount = true
	}

	mres := new(MultiResponse)
	mres.Type = "minorBlock"
	mres.Items = make([]interface{}, 0)
	mres.Start = req.Start
	mres.Count = req.Count
	mres.Total = r.Total
	seen := map[[32]byte]bool{}
	for _, block := range r.Records {
		resp := new(MinorQueryResponse)
		resp.BlockIndex = block.Index
		resp.BlockTime = block.Time

		if block.Entries == nil {
			// This circumvents the count check, but that's how the original API
			// v2 worked...
			if req.BlockFilterMode != BlockFilterModeExcludeEmpty {
				mres.Items = append(mres.Items, resp)
			}
			continue
		}

		for _, entry := range block.Entries.Records {
			// Only include main chain entries
			if entry.Type != merkle.ChainTypeTransaction || entry.Name != "main" {
				continue
			}

			// Only include each transaction once
			if seen[entry.Entry] {
				continue
			} else {
				seen[entry.Entry] = true
			}

			txr, err := api.ChainEntryRecordAsMessage[*messaging.TransactionMessage](entry)
			if err != nil {
				continue
			}
			if wantCount {
				resp.TxCount++
			}
			if wantIds {
				h := txr.Value.ID.Hash()
				resp.TxIds = append(resp.TxIds, h[:])
			}
			if wantTx {
				txres, err := chainTxnV3(txr)
				if err != nil {
					return accumulateError(err)
				}
				resp.Transactions = append(resp.Transactions, txres)
			}
		}

		// Omit empty blocks
		if resp.TxCount == 0 && req.BlockFilterMode == BlockFilterModeExcludeEmpty {
			continue
		}

		mres.Items = append(mres.Items, resp)

		// Stop once count is reached
		if len(mres.Items) >= int(req.Count) {
			break
		}
	}

	return mres
}

func (m *JrpcMethods) QueryMajorBlocks(ctx context.Context, params json.RawMessage) interface{} {
	req := new(MajorBlocksQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	q := new(api.BlockQuery)
	q.MajorRange = &api.RangeOptions{Start: req.Start, Count: &req.Count}
	r, err := m.netq().QueryMajorBlocks(ctx, req.Url, q)
	if err != nil {
		return accumulateError(err)
	}

	mres := new(MultiResponse)
	mres.Type = "majorBlock"
	mres.Items = make([]interface{}, 0)
	mres.Start = req.Start
	mres.Count = req.Count
	mres.Total = r.Total
	for _, major := range r.Records {
		resp := new(MajorQueryResponse)
		resp.MajorBlockIndex = major.Index
		resp.MajorBlockTime = &major.Time

		q := new(api.BlockQuery)
		q.OmitEmpty = true
		q.Major = &major.Index
		q.MinorRange = &api.RangeOptions{Count: uint64p(100)}
		r, err := m.netq().QueryMajorBlock(ctx, req.Url, q)
		if err != nil {
			return accumulateError(err)
		}
		for _, minor := range r.MinorBlocks.Records {
			minres := new(MinorBlock)
			minres.BlockIndex = minor.Index
			minres.BlockTime = minor.Time
			resp.MinorBlocks = append(resp.MinorBlocks, minres)
		}
		mres.Items = append(mres.Items, resp)
	}

	return mres
}

func (m *JrpcMethods) QuerySynth(ctx context.Context, params json.RawMessage) interface{} {
	req := new(SyntheticTransactionRequest)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	var src *url.URL
	if req.Anchor {
		src = req.Source.JoinPath(protocol.AnchorPool)
	} else {
		src = req.Source.JoinPath(protocol.Synthetic)
	}

	r, err := recordIs[*api.MessageRecord[messaging.Message]](m.Sequencer.Sequence(ctx, src, req.Destination, req.SequenceNumber))
	if err != nil {
		return accumulateError(err)
	}

	switch msg := r.Message.(type) {
	case *messaging.TransactionMessage:
		return jrpcFormatResponse(transactionV3(r, msg))
	case *messaging.SignatureMessage:
		return jrpcFormatResponse(signatureV3(r, msg))
	default:
		return accumulateError(fmt.Errorf("%v is a %v, not a transaction or signature", msg.ID(), msg.Type()))
	}
}
