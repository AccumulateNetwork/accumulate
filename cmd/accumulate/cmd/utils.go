package cmd

import (
	"fmt"
	"log"
	"math"
	"math/big"

	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func IsLiteAccount(url string) bool {
	u, err := url2.Parse(url)
	if err != nil {
		log.Fatal(err)
	}
	key, _, _ := protocol.ParseLiteTokenAddress(u)
	return key != nil
}

func amountToBigInt(tokenUrl string, amount string) (*big.Int, error) {
	t := new(protocol.TokenIssuer)
	_, _, err := queryAccount(tokenUrl, t)
	if err != nil {
		return nil, fmt.Errorf("error retrieving token url, %v", err)
	}

	amt, _ := big.NewFloat(0).SetPrec(128).SetString(amount)
	if amt == nil {
		return nil, fmt.Errorf("invalid amount %s", amount)
	}
	oneToken := big.NewFloat(math.Pow(10.0, float64(t.Precision)))
	amt.Mul(amt, oneToken)
	iAmt, _ := amt.Int(big.NewInt(0))
	return iAmt, nil
}
