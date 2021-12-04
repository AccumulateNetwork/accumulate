package testing

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/api"
	apitypes "github.com/AccumulateNetwork/accumulate/types/api"
	querytypes "github.com/AccumulateNetwork/accumulate/types/api/query"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
)

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

func WaitForTxidV1(query *api.Query, txid []byte) error {
	var resp *coretypes.ResultABCIQuery
	var err error
	for {
		resp, err = query.QueryByTxId(txid)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "not found") {
			return err
		}
		time.Sleep(time.Second / 10)
	}
	if resp.Response.Code != 0 {
		return fmt.Errorf("query failed with code %d: %s", resp.Response.Code, resp.Response.Info)
	}

	res := new(querytypes.ResponseByTxId)
	err = res.UnmarshalBinary(resp.Response.Value)
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
