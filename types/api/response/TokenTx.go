package response

import (
	"bytes"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

const (
	StatusTokenTxExpired    types.String = "expired"    //StatusTokenTxExpired transaction status is no longer available
	StatusTokenTxInvalid                 = "invalid"    //StatusTokenTxInvalid transaction is failed validation and returned
	StatusTokenTxPending                 = "pending"    //StatusTokenTxPending transaction is pending validation
	StatusTokenTxProcessing              = "processing" //StatusTokenTxProcessing transaction is currently being validated
	StatusTokenTxComplete                = "complete"   //StatusTokenTxComplete
	StatusTokenTxUnknown                 = "unknown"    //StatusTokenTxUnknown transaction not found or not yet received
)

type TokenTx struct {
	TxId      types.Bytes32          `json:"txid"`
	FromUrl   types.String           `json:"accountUrl"`
	ToAccount []TokenTxAccountStatus `json:"to"`
}

type TokenTxAccountStatus struct {
	AccountUrl types.String `json:"accountUrl"`
	Status     types.String `json:"status"`
}

func (t *TokenTxAccountStatus) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer
	data, err := t.AccountUrl.MarshalBinary()
	if err != nil {
		return nil, err
	}

	buffer.Write(data)

	data, err = t.Status.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)
	return buffer.Bytes(), nil
}

func (t *TokenTxAccountStatus) UnmarshalBinary(data []byte) error {
	dLen := len(data)
	if dLen < 1 {
		return fmt.Errorf("insufficient data to unmarshal AccountUrl for TokenTxAccountStatus")
	}

	err := t.AccountUrl.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("unable to unmarshal AccountUrl from TokenTxAccountStatus, %v", err)
	}
	i := t.AccountUrl.Size(nil)
	if dLen < i {
		return fmt.Errorf("insufficient data to unmarshal Status for TokenTxAccountStatus")
	}

	err = t.Status.UnmarshalBinary(data[i:])
	if err != nil {
		return fmt.Errorf("unable to unmarshal Status from TokenTxAccountStatus, %v", err)
	}

	return nil
}
