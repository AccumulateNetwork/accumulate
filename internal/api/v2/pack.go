package api

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func packStateResponse(obj *protocol.Object, chain protocol.Account) (*ChainQueryResponse, error) {
	res := new(ChainQueryResponse)
	res.Type = chain.GetType().String()
	res.MainChain = new(MerkleState)
	res.MainChain.Height = obj.Height
	res.MainChain.Roots = obj.Roots
	res.Data = chain
	res.ChainId = chain.Header().Url.AccountID()
	return res, nil
}

func packTxResponse(txid [32]byte, synth []byte, ms *MerkleState, envelope *protocol.Envelope, status *protocol.TransactionStatus) (*TransactionQueryResponse, error) {

	res := new(TransactionQueryResponse)
	res.Type = envelope.Transaction.Body.GetType().String()
	res.Data = envelope.Transaction.Body
	res.TransactionHash = txid[:]
	res.MainChain = ms
	res.Transaction = envelope.Transaction
	res.KeyPage = new(KeyPage)
	res.KeyPage.Height = envelope.Transaction.KeyPageHeight
	res.KeyPage.Index = envelope.Transaction.KeyPageIndex

	if len(synth)%32 != 0 {
		return nil, fmt.Errorf("invalid synthetic transaction information, not divisible by 32")
	}

	if synth != nil {
		res.SyntheticTxids = make([][32]byte, len(synth)/32)
		for i := range res.SyntheticTxids {
			copy(res.SyntheticTxids[i][:], synth[i*32:(i+1)*32])
		}
	}

	switch payload := envelope.Transaction.Body.(type) {
	case *protocol.SendTokens:
		if synth != nil && len(res.SyntheticTxids) != len(payload.To) {
			return nil, fmt.Errorf("not enough synthetic TXs: want %d, got %d", len(payload.To), len(res.SyntheticTxids))
		}

		res.Origin = envelope.Transaction.Origin
		data := new(TokenSend)
		data.From = envelope.Transaction.Origin
		data.To = make([]TokenDeposit, len(payload.To))
		for i, to := range payload.To {
			data.To[i].Url = to.Url
			data.To[i].Amount = to.Amount
			if synth != nil {
				data.To[i].Txid = synth[i*32 : (i+1)*32]
			}
		}

		res.Origin = envelope.Transaction.Origin
		res.Data = data

	case *protocol.SyntheticDepositTokens:
		res.Origin = envelope.Transaction.Origin
		res.Data = payload

	default:
		res.Origin = envelope.Transaction.Origin
		res.Data = payload
	}

	res.Signatures = envelope.Signatures
	res.Status = status

	return res, nil
}
