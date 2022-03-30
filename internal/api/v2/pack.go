package api

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func packStateResponse(account protocol.Account, chains []query.ChainState) (*ChainQueryResponse, error) {
	res := new(ChainQueryResponse)
	res.Type = account.GetType().String()
	res.Data = account
	res.Chains = chains
	res.ChainId = account.Header().Url.AccountID()

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
	res.Type = envelope.Transaction.Body.GetType().String()
	res.Data = envelope.Transaction.Body
	res.TransactionHash = qrResp.TxId[:]
	res.MainChain = ms
	res.Transaction = envelope.Transaction

	if len(qrResp.TxSynthTxIds)%32 != 0 {
		return nil, fmt.Errorf("invalid synthetic transaction information, not divisible by 32")
	}

	if qrResp.TxSynthTxIds != nil {
		res.SyntheticTxids = make([][32]byte, len(qrResp.TxSynthTxIds)/32)
		for i := range res.SyntheticTxids {
			copy(res.SyntheticTxids[i][:], qrResp.TxSynthTxIds[i*32:(i+1)*32])
		}
	}

	switch payload := envelope.Transaction.Body.(type) {
	case *protocol.SendTokens:
		if qrResp.TxSynthTxIds != nil && len(res.SyntheticTxids) != len(payload.To) {
			return nil, fmt.Errorf("not enough synthetic TXs: want %d, got %d", len(payload.To), len(res.SyntheticTxids))
		}

		res.Origin = envelope.Transaction.Header.Principal
		data := new(TokenSend)
		data.From = envelope.Transaction.Header.Principal
		data.To = make([]TokenDeposit, len(payload.To))
		for i, to := range payload.To {
			data.To[i].Url = to.Url
			data.To[i].Amount = to.Amount
			if qrResp.TxSynthTxIds != nil {
				data.To[i].Txid = qrResp.TxSynthTxIds[i*32 : (i+1)*32]
			}
		}

		res.Origin = envelope.Transaction.Header.Principal
		res.Data = data

	case *protocol.SyntheticDepositTokens:
		res.Origin = envelope.Transaction.Header.Principal
		res.Data = payload

	default:
		res.Origin = envelope.Transaction.Header.Principal
		res.Data = payload
	}

	res.Status = status
	res.Signers = qrResp.Signers
	res.Receipts = qrResp.Receipts
	for _, signer := range qrResp.Signers {
		res.Signatures = append(res.Signatures, signer.Signatures...)
	}

	return res, nil
}
