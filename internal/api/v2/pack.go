// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"encoding/hex"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func packStateResponse(account protocol.Account, chains []query.ChainState, receipt *query.GeneralReceipt) (*ChainQueryResponse, error) {
	res := new(ChainQueryResponse)
	res.Type = account.Type().String()
	res.Data = account
	res.Chains = chains
	res.ChainId = account.GetUrl().AccountID()
	res.Receipt = receipt

	for _, chain := range chains {
		if chain.Name != protocol.MainChain {
			continue
		}

		res.MainChain = new(MerkleState)
		res.MainChain.Height = chain.Height
		res.MainChain.Roots = chain.Roots
	}
	return res, nil
}

func packTxResponse(qrResp *query.ResponseByTxId, ms *MerkleState, envelope *protocol.Envelope, status *protocol.TransactionStatus) (*TransactionQueryResponse, error) {
	res := new(TransactionQueryResponse)
	res.Type = envelope.Transaction[0].Body.Type().String()
	res.Data = envelope.Transaction[0].Body
	h := qrResp.TxId.Hash()
	res.Txid = qrResp.TxId
	res.TransactionHash = h[:]
	res.MainChain = ms
	res.Transaction = envelope.Transaction[0]
	res.Produced = qrResp.Produced

	switch payload := envelope.Transaction[0].Body.(type) {
	case *protocol.SendTokens:
		if qrResp.Produced != nil && len(qrResp.Produced) != len(payload.To) {
			return nil, fmt.Errorf("not enough synthetic TXs: want %d, got %d", len(payload.To), len(qrResp.Produced))
		}

		res.Origin = envelope.Transaction[0].Header.Principal
		data := new(TokenSend)
		data.From = envelope.Transaction[0].Header.Principal
		data.To = make([]TokenDeposit, len(payload.To))
		for i, to := range payload.To {
			data.To[i].Url = to.Url
			data.To[i].Amount = to.Amount
			if qrResp.Produced != nil {
				h := qrResp.Produced[i].Hash()
				data.To[i].Txid = h[:]
			}
		}

		res.Origin = envelope.Transaction[0].Header.Principal
		res.Data = data

	case *protocol.SyntheticDepositTokens:
		res.Origin = envelope.Transaction[0].Header.Principal
		res.Data = payload

	default:
		res.Origin = envelope.Transaction[0].Header.Principal
		res.Data = payload
	}

	res.Status = status
	res.Receipts = qrResp.Receipts
	res.PartitionUrl = qrResp.PartitionUrl

	books := map[string]*SignatureBook{}
	for _, signer := range qrResp.Signers {
		res.Signatures = append(res.Signatures, signer.Signatures...)

		var book *SignatureBook
		signerUrl := signer.Account.GetUrl()
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
		page.Signer.Type = signer.Account.Type()
		page.Signer.Url = signerUrl
		page.Signatures = signer.Signatures

		keyPage, ok := signer.Account.(*protocol.KeyPage)
		if ok {
			page.Signer.AcceptThreshold = keyPage.AcceptThreshold
		}
	}

	return res, nil
}

func packMinorQueryResponse(entry *query.ResponseMinorEntry) (*MinorQueryResponse, error) {
	var err error
	resp := new(MinorQueryResponse)
	resp.BlockIndex = entry.BlockIndex
	resp.BlockTime = entry.BlockTime
	resp.TxCount = entry.TxCount
	resp.TxIds = entry.TxIds
	for _, tx := range entry.Transactions {
		if tx.Envelope != nil {
			var txQryResp *TransactionQueryResponse
			txQryResp, err = packTxResponse(tx, nil, tx.Envelope, tx.Status)
			resp.Transactions = append(resp.Transactions, txQryResp)
		}
	}
	return resp, err
}

func packMajorQueryResponse(entry *query.ResponseMajorEntry) (*MajorQueryResponse, error) {
	var err error
	resp := new(MajorQueryResponse)
	resp.MajorBlockIndex = entry.MajorBlockIndex
	resp.MajorBlockTime = entry.MajorBlockTime
	for _, minBlk := range entry.MinorBlocks {
		minBlkResp := &MinorBlock{}
		minBlkResp.BlockIndex = minBlk.BlockIndex
		minBlkResp.BlockTime = minBlk.BlockTime
		resp.MinorBlocks = append(resp.MinorBlocks, minBlkResp)
	}
	return resp, err
}

func packChainValue(qr *query.ResponseChainEntry) *ChainQueryResponse {
	resp := new(ChainQueryResponse)
	resp.Type = "chainEntry"
	resp.MainChain = new(MerkleState)
	resp.MainChain.Height = uint64(qr.Height)
	resp.MainChain.Roots = qr.State
	resp.Receipt = qr.Receipt

	entry := new(ChainEntry)
	entry.Height = qr.Height
	entry.Entry = qr.Entry
	entry.State = qr.State
	resp.Data = entry

	switch qr.Type {
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

func packChainValues(qr *query.ResponseChainRange) *MultiResponse {
	resp := new(MultiResponse)
	resp.Type = "chainEntrySet"
	resp.Start = uint64(qr.Start)
	resp.Count = uint64(qr.End - qr.Start)
	resp.Total = uint64(qr.Total)
	resp.Items = make([]interface{}, len(qr.Entries))
	for i, entry := range qr.Entries {
		qr := new(ChainQueryResponse)
		resp.Items[i] = qr
		qr.Type = "hex"
		qr.Data = hex.EncodeToString(entry)
	}

	switch qr.Type {
	default:
		return resp
	case protocol.ChainTypeIndex:
		resp.OtherItems = make([]interface{}, len(qr.Entries))
		for i, entry := range qr.Entries {
			v := new(protocol.IndexEntry)
			err := v.UnmarshalBinary(entry)
			if err == nil {
				resp.Items[i] = v
			}
		}

		return resp
	}
}
