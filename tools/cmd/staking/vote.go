package main

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/staking"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func constructStakeEntry(ledger *url.URL, stake *staking.Account) (*protocol.Transaction, error) {
	b, err := stake.MarshalBinary()
	if err != nil {
		return nil, err
	}

	entry := new(protocol.AccumulateDataEntry)
	entry.Data = [][]byte{b}
	body := new(protocol.WriteData)
	body.Entry = entry
	// Scratch?
	txn := new(protocol.Transaction)
	txn.Header.Principal = ledger
	txn.Body = body
	return txn, nil
}

func signTransaction(txn *protocol.Transaction, signers ...*signing.Builder) ([]protocol.Signature, error) {
	sigs := make([]protocol.Signature, len(signers))
	for i, signer := range signers {
		var err error
		if txn.Header.Initiator == ([32]byte{}) {
			sigs[i], err = signer.Initiate(txn)
		} else {
			sigs[i], err = signer.Sign(txn.GetHash())
		}
		if err != nil {
			return nil, err
		}
	}
	return sigs, nil
}

func submitTransaction(client *client.Client, ctx context.Context, txn *protocol.Transaction, sigs []protocol.Signature) (*api.TxResponse, error) {
	req := new(api.ExecuteRequest)
	req.Envelope = new(protocol.Envelope)
	req.Envelope.Transaction = []*protocol.Transaction{txn}
	req.Envelope.Signatures = sigs
	return client.ExecuteDirect(ctx, req)
}
