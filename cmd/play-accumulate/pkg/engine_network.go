// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pkg

import (
	"context"
	"encoding/json"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NetEngine struct {
	*client.Client
}

func (s *Session) UseNetwork(client *client.Client) {
	s.Engine = NetEngine{client}
}

func (e NetEngine) GetAccount(accountUrl *URL) (protocol.Account, error) {
	req := new(api.GeneralQuery)
	req.Url = accountUrl
	var raw json.RawMessage
	var resp api.ChainQueryResponse
	resp.Data = &raw
	err := e.RequestAPIv2(context.Background(), "query", req, &resp)
	if err != nil {
		return nil, err
	}

	return protocol.UnmarshalAccountJSON(raw)
}

func (e NetEngine) GetDirectory(account *URL) ([]*URL, error) {
	req := new(api.DirectoryQuery)
	req.Url = account
	req.Count = 1000
	resp, err := e.QueryDirectory(context.Background(), req)
	if err != nil {
		return nil, err
	}

	urls := make([]*URL, len(resp.Items))
	for i, str := range resp.Items {
		urls[i], err = url.Parse(str.(string))
		if err != nil {
			return nil, err
		}
	}
	return urls, nil
}

func (e NetEngine) GetTransaction(txid [32]byte) (*protocol.Transaction, error) {
	req := new(api.TxnQuery)
	req.Txid = txid[:]
	resp, err := e.QueryTx(context.Background(), req)
	if err != nil {
		return nil, err
	}

	return resp.Transaction, nil
}

func (e NetEngine) Submit(envelope *messaging.Envelope) (*protocol.TransactionStatus, error) {
	var err error
	req := new(api.ExecuteRequest)
	req.Envelope = envelope
	resp, err := e.ExecuteDirect(context.Background(), req)
	if err != nil {
		return nil, err
	}

	status := new(protocol.TransactionStatus)
	data, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(data, &status) == nil {
		return status, nil
	}

	status = new(protocol.TransactionStatus)
	status.Set(errors.UnknownError.With("unknown"))
	return status, nil
}

func (e NetEngine) WaitFor(hash [32]byte, delivered bool) ([]*protocol.TransactionStatus, []*protocol.Transaction, error) {
	req := new(api.TxnQuery)
	req.Txid = hash[:]
	req.Wait = 10 * time.Second
	req.IgnorePending = delivered
	resp, err := e.QueryTx(context.Background(), req)
	if err != nil {
		return nil, nil, err
	}

	resp.Status.TxID = resp.Transaction.ID()
	statuses := []*protocol.TransactionStatus{resp.Status}
	transactions := []*protocol.Transaction{resp.Transaction}
	for _, hash := range resp.Produced {
		st, txn, err := e.WaitFor(hash.Hash(), true)
		if err != nil {
			return nil, nil, err
		}
		statuses = append(statuses, st...)
		transactions = append(transactions, txn...)
	}

	return statuses, transactions, nil
}
