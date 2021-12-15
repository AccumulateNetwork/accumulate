package goaccumulate

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
)

func AddCredits(actor string, args []string) (string, error) {

	u, err := url2.Parse(actor)
	if err != nil {
		return "", err
	}

	args, si, privKey, err := prepareSigner(u, args)
	if err != nil {
		return "", err
	}

	if len(args) < 2 {
		return "", err
	}

	u2, err := url2.Parse(args[0])
	if err != nil {
		return "", err
	}

	amt, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("amount must be an integer %v", err)
	}
	var res api2.QueryResponse
	credits := protocol.AddCredits{}
	credits.Recipient = u2.String()
	credits.Amount = uint64(amt)

	data, err := json.Marshal(credits)
	if err != nil {
		return "", err
	}

	dataBinary, err := credits.MarshalBinary()
	if err != nil {
		return "", err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, u, si, privKey, nonce)
	if err != nil {
		return "", err
	}

	if err := Client.RequestV2(context.Background(), "add-credits", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	ar := ActionResponse{}
	//	err = json.Unmarshal(*res.Data, &ar)
	//	if err != nil {
	//		resData, err := json.Marshal(&res)
	//		var out string
	//		if err != nil {
	//			out = fmt.Sprintf("%v", err)
	//		} else {
	//			out = string(resData)
	//		}
	//		return "", fmt.Errorf("error unmarshalling add credits result %s", out)
	//	}
	return ar.Print()
}
