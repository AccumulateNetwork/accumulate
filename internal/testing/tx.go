package testing

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/api"
	apitypes "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
	querytypes "github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
)

func WaitForTxHashV1(query *api.Query, routing uint64, txid []byte) error {
	var txid32 [32]byte
	copy(txid32[:], txid)

	start := time.Now()
	var r *coretypes.ResultTx
	var err error
	for {
		r, err = query.GetTx(routing, txid32)
		if err == nil {
			break
		}

		if !strings.Contains(err.Error(), "not found") {
			return err
		}

		if time.Since(start) > 10*time.Second {
			return fmt.Errorf("tx (%X) not found: timed out", txid)
		}

		time.Sleep(time.Second / 10)
	}

	if r.TxResult.Code != 0 {
		return fmt.Errorf("txn failed with code %d: %s", r.TxResult.Code, r.TxResult.Log)
	}

	tx := new(transactions.Envelope)
	err = tx.UnmarshalBinary(r.Tx)
	if err != nil {
		return err
	}

	return WaitForTxidV1(query, tx.Transaction.Hash())
}

func WaitForTxV1(query *api.Query, txResp *apitypes.APIDataResponse) error {
	var data struct{ Txid string }
	err := json.Unmarshal(*txResp.Data, &data)
	if err != nil {
		return err
	}

	txid, err := hex.DecodeString(data.Txid)
	if err != nil {
		return err
	}

	return WaitForTxidV1(query, txid)
}

func WaitForTxidV1(q *api.Query, txid []byte) error {
	start := time.Now()
	var resp *coretypes.ResultABCIQuery
	var err error
	for {
		resp, err = q.QueryByTxId(txid)
		if err == nil {
			break
		}

		if !strings.Contains(err.Error(), "not found") {
			return err
		}

		if time.Since(start) > 10*time.Second {
			return fmt.Errorf("tx (%X) not found: timed out", txid)
		}

		time.Sleep(time.Second / 10)
	}
	if resp.Response.Code != 0 {
		return fmt.Errorf("query failed with code %d: %s", resp.Response.Code, resp.Response.Info)
	}
	if string(resp.Response.Key) != "tx" {
		return fmt.Errorf("wrong type: wanted tx, got %q", resp.Response.Key)
	}
	qr := new(query.ResponseByTxId)
	err = qr.UnmarshalBinary(resp.Response.Value)
	if err != nil {
		return fmt.Errorf("invalid response: %v", err)
	}

	return WaitForSynthTxnsV1(q, resp)
}

func WaitForSynthTxnsV1(query *api.Query, resp *coretypes.ResultABCIQuery) error {
	res := new(querytypes.ResponseByTxId)
	err := res.UnmarshalBinary(resp.Response.Value)
	if err != nil {
		return fmt.Errorf("invalid TX response: %v", err)
	}

	if len(res.TxSynthTxIds)%32 != 0 {
		panic("bad synth IDs")
	}

	count := len(res.TxSynthTxIds) / 32
	for i := 0; i < count; i++ {
		id := res.TxSynthTxIds[i*32 : (i+1)*32]
		err = WaitForTxidV1(query, id)
		if err != nil {
			return err
		}
	}

	return nil
}
