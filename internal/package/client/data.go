package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
)

func GetDataEntry(accountUrl string, args []string) (*api2.QueryResponse, error) {
	u, err := url.Parse(accountUrl)
	if err != nil {
		return nil, err
	}

	params := new(api2.DataEntryQuery)
	params.Url = u.String()
	if len(args) > 0 {
		n, err := hex.Decode(params.EntryHash[:], []byte(args[0]))
		if err != nil {
			return nil, err
		}
		if n != 32 {
			return nil, fmt.Errorf("entry hash must be 64 hex characters in length")
		}
	}

	var res api2.QueryResponse

	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	err = Client.RequestV2(context.Background(), "query-data", json.RawMessage(data), &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func GetDataEntrySet(accountUrl string, args []string) (*api2.QueryResponse, error) {
	u, err := url.Parse(accountUrl)
	if err != nil {
		return nil, err
	}

	if len(args) > 3 || len(args) < 2 {
		return nil, fmt.Errorf("expecting the start index and count parameters with optional expand")
	}

	params := new(api2.DataEntrySetQuery)
	params.Url = u.String()

	v, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid start argument %s, %v", args[1], err)
	}
	params.Start = uint64(v)

	v, err = strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid count argument %s, %v", args[1], err)
	}
	params.Count = uint64(v)

	if len(args) > 2 {
		if args[2] == "expand" {
			params.ExpandChains = true
		}
	}

	var res api2.QueryResponse
	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	err = Client.RequestV2(context.Background(), "query-data-set", json.RawMessage(data), &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func CreateDataAccount(actorUrl string, args []string) (*api2.TxResponse, error) {
	actor, err := url.Parse(actorUrl)
	if err != nil {
		return nil, err
	}

	args, si, privkey, err := prepareSigner(actor, args)
	if err != nil {
		return nil, fmt.Errorf("insufficient number of command line arguments")
	}

	if len(args) < 1 {
		return nil, fmt.Errorf("expecting account url")
	}

	accountUrl, err := url.Parse(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid account url %s", args[0])
	}
	if actor.Authority != accountUrl.Authority {
		return nil, fmt.Errorf("account url to create (%s) doesn't match the authority adi (%s)", accountUrl.Authority, actor.Authority)
	}

	var keybook string
	if len(args) > 1 {
		kbu, err := url.Parse(args[1])
		if err != nil {
			return nil, fmt.Errorf("invalid key book url")
		}
		keybook = kbu.String()
	}

	cda := protocol.CreateDataAccount{}
	cda.Url = accountUrl.String()
	cda.KeyBookUrl = keybook

	data, err := json.Marshal(cda)
	if err != nil {
		return nil, err
	}

	dataBinary, err := cda.MarshalBinary()
	if err != nil {
		return nil, err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, actor, si, privkey, nonce)
	if err != nil {
		return nil, err
	}

	res := new(api2.TxResponse)

	err = Client.RequestV2(context.Background(), "create-data-account", params, &res)
	if err != nil {
		return nil, err

	}

	return res, nil
}

func WriteData(accountUrl string, args []string) (*api2.DataEntry, error) {
	actor, err := url.Parse(accountUrl)
	if err != nil {
		return nil, err
	}

	args, si, privkey, err := prepareSigner(actor, args)
	if err != nil {
		return nil, fmt.Errorf("insufficient number of command line arguments")
	}

	if len(args) < 1 {
		return nil, fmt.Errorf("expecting account url")
	}

	wd := protocol.WriteData{}

	for i := 0; i < len(args); i++ {
		data := make([]byte, len(args[i]))

		//attempt to hex decode it
		n, err := hex.Decode(data, []byte(args[i]))
		if err != nil {
			//if it is not a hex string, then just store the data as-is
			copy(data, args[i])
		} else {
			//clip the padding
			data = data[:n]
		}
		if i == len(args)-1 {
			wd.Entry.Data = data
		} else {
			wd.Entry.ExtIds = append(wd.Entry.ExtIds, data)
		}
	}

	data, err := json.Marshal(&wd)
	if err != nil {
		return nil, err
	}

	dataBinary, err := wd.MarshalBinary()
	if err != nil {
		return nil, err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, actor, si, privkey, nonce)
	if err != nil {
		return nil, err
	}

	var res api.DataEntry

	if err := Client.RequestV2(context.Background(), "write-data", params, &res); err != nil {
		return nil, err
	}

	//return ActionResponseFromData(&res, wd.Entry.Hash()).Print()
	return &res, nil
}
