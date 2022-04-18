package pkg

import (
	"context"
	"encoding/json"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
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

func (e NetEngine) Submit(envelope *protocol.Envelope) (*protocol.TransactionStatus, error) {
	var err error
	req := new(api.TxRequest)
	req.IsEnvelope = true
	req.Origin = envelope.Transaction[0].Header.Principal
	req.Payload, err = envelope.MarshalBinary()
	if err != nil {
		return nil, err
	}

	resp, err := e.Execute(context.Background(), req)
	if err != nil {
		return nil, err
	}

	status := new(protocol.TransactionStatus)
	status.Code = resp.Code
	status.Message = resp.Message
	return status, nil
}

func (e NetEngine) WaitFor(hash [32]byte) (*protocol.TransactionStatus, *protocol.Transaction, error) {
	req := new(api.TxnQuery)
	req.Txid = hash[:]
	req.Wait = 10 * time.Second
	resp, err := e.QueryTx(context.Background(), req)
	if err != nil {
		return nil, nil, err
	}

	for _, hash := range resp.SyntheticTxids {
		_, _, err := e.WaitFor(hash)
		if err != nil {
			return nil, nil, err
		}
	}

	return resp.Status, resp.Transaction, nil
}
