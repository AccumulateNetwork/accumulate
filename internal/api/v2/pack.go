package api

import (
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/AccumulateNetwork/accumulate/types/synthetic"
)

func packStateResponse(obj *state.Object, chain state.Chain) (*QueryResponse, error) {
	b, err := json.Marshal(chain)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %v", err)
	}

	res := new(QueryResponse)
	res.Type = chain.Header().Type.Name()
	res.MerkleState = new(MerkleState)
	res.MerkleState.Count = obj.Height
	res.MerkleState.Roots = obj.Roots
	res.Data = (*json.RawMessage)(&b)
	return res, nil
}

func packTxResponse(txid [32]byte, synth []byte, main *state.Transaction, pend *state.PendingTransaction, payload protocol.TransactionPayload) (*QueryResponse, error) {
	var tx *state.TxState
	if main != nil {
		tx = &main.TxState
	} else {
		tx = pend.TransactionState
	}

	res := new(QueryResponse)
	res.Type = payload.GetType().String()
	res.Data = payload
	res.Txid = txid[:]
	res.KeyPage.Height = tx.SigInfo.MSHeight
	res.KeyPage.Index = tx.SigInfo.PriorityIdx

	switch payload := payload.(type) {
	case *api.TokenTx:
		if len(synth) != len(payload.To)*32 {
			return nil, fmt.Errorf("not enough synthetic TXs: wanted %d*32 bytes, got %d", len(payload.To), len(synth))
		}

		res.Sponsor = tx.SigInfo.URL
		data := new(TokenSend)
		data.From = *payload.From.AsString()
		data.To = make([]TokenDeposit, len(payload.To))
		for i, to := range payload.To {
			data.To[i].Url = *to.URL.AsString()
			data.To[i].Amount = to.Amount
			data.To[i].Txid = synth[i*32 : (i+1)*32]
		}

		res.Sponsor = *payload.From.AsString()
		res.Data = data

	case *synthetic.TokenTransactionDeposit:
		res.Sponsor = *payload.FromUrl.AsString()
		res.Data = payload

	default:
		res.Sponsor = tx.SigInfo.URL
		res.Data = payload
	}

	if pend != nil && len(pend.Signature) > 0 {
		sig := pend.Signature[0]
		res.Status = pend.Status
		res.Signer.PublicKey = sig.PublicKey
		res.Signer.Nonce = sig.Nonce
		res.Sig = sig.Signature
	}

	return res, nil
}
