package api

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func packStateResponse(obj *state.Object, chain state.Chain) (*ChainQueryResponse, error) {
	res := new(ChainQueryResponse)
	res.Type = chain.Header().Type.Name()
	res.MainChain = new(MerkleState)
	res.MainChain.Height = obj.Height
	res.MainChain.Roots = obj.Roots
	res.Data = chain
	return res, nil
}

func packTxResponse(txid [32]byte, synth []byte, main *state.Transaction, pend *state.PendingTransaction, payload protocol.TransactionPayload) (*TransactionQueryResponse, error) {
	var tx *state.TxState
	if main != nil {
		tx = &main.TxState
	} else {
		tx = pend.TransactionState
	}

	res := new(TransactionQueryResponse)
	res.Type = payload.GetType().String()
	res.Data = payload
	res.Txid = txid[:]
	res.KeyPage = new(KeyPage)
	res.KeyPage.Height = tx.SigInfo.KeyPageHeight
	res.KeyPage.Index = tx.SigInfo.KeyPageIndex

	if len(synth)%32 != 0 {
		return nil, fmt.Errorf("invalid synthetic transaction information, not divisible by 32")
	}
	res.SyntheticTxids = make([][32]byte, len(synth)/32)
	for i := range res.SyntheticTxids {
		copy(res.SyntheticTxids[i][:], synth[i*32:(i+1)*32])
	}

	switch payload := payload.(type) {
	case *protocol.SendTokens:
		if len(res.SyntheticTxids) != len(payload.To) {
			return nil, fmt.Errorf("not enough synthetic TXs: want %d, got %d", len(payload.To), len(res.SyntheticTxids))
		}

		res.Origin = tx.SigInfo.URL
		data := new(TokenSend)
		data.From = main.SigInfo.URL
		data.To = make([]TokenDeposit, len(payload.To))
		for i, to := range payload.To {
			data.To[i].Url = to.Url
			data.To[i].Amount = to.Amount
			data.To[i].Txid = synth[i*32 : (i+1)*32]
		}

		res.Origin = main.SigInfo.URL
		res.Data = data

	case *protocol.SyntheticDepositTokens:
		res.Origin = main.SigInfo.URL
		res.Data = payload

	default:
		res.Origin = tx.SigInfo.URL
		res.Data = payload
	}

	if pend != nil && len(pend.Signature) > 0 {
		sig := pend.Signature[0]
		res.Status = pend.Status
		res.Signatures = append(res.Signatures, sig.Signature)
	}

	return res, nil
}

func packTxAsChainResponse(txid [32]byte, synth []byte, main *state.Transaction, pend *state.PendingTransaction, payload protocol.TransactionPayload) (*ChainQueryResponse, error) {
	var tx *state.TxState
	if main != nil {
		tx = &main.TxState
	} else {
		tx = pend.TransactionState
	}

	res := new(ChainQueryResponse)
	res.Type = payload.GetType().String()
	res.Data = payload
	res.Txid = txid[:]
	res.KeyPage = new(KeyPage)
	res.KeyPage.Height = tx.SigInfo.KeyPageHeight
	res.KeyPage.Index = tx.SigInfo.KeyPageIndex

	if len(synth)%32 != 0 {
		return nil, fmt.Errorf("invalid synthetic transaction information, not divisible by 32")
	}
	res.SyntheticTxids = make([][32]byte, len(synth)/32)
	for i := range res.SyntheticTxids {
		copy(res.SyntheticTxids[i][:], synth[i*32:(i+1)*32])
	}

	switch payload := payload.(type) {
	case *protocol.SendTokens:
		if len(res.SyntheticTxids) != len(payload.To) {
			return nil, fmt.Errorf("not enough synthetic TXs: want %d, got %d", len(payload.To), len(res.SyntheticTxids))
		}

		res.Origin = tx.SigInfo.URL
		data := new(TokenSend)
		data.From = main.SigInfo.URL
		data.To = make([]TokenDeposit, len(payload.To))
		for i, to := range payload.To {
			data.To[i].Url = to.Url
			data.To[i].Amount = to.Amount
			data.To[i].Txid = synth[i*32 : (i+1)*32]
		}

		res.Origin = main.SigInfo.URL
		res.Data = data

	case *protocol.SyntheticDepositTokens:
		res.Origin = main.SigInfo.URL
		res.Data = payload

	default:
		res.Origin = tx.SigInfo.URL
		res.Data = payload
	}

	if pend != nil && len(pend.Signature) > 0 {
		res.Status = pend.Status
	}

	return res, nil
}
