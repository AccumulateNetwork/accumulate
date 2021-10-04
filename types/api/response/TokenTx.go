package response

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
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
	TxId      types.Bytes           `json:"txid"`
	From      types.String          `json:"from"`
	ToAccount []TokenTxOutputStatus `json:"to"`
}

type TokenTxOutputStatus struct {
	SyntheticTxId types.Bytes `json:"txid"`
	api.TokenTxOutput
}

func (t *TokenTxOutputStatus) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer
	data, err := t.SyntheticTxId.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	data, err = t.TokenTxOutput.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	return buffer.Bytes(), nil
}

func (t *TokenTxOutputStatus) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if rErr := recover(); rErr != nil {
			err = fmt.Errorf("insufficent data to unmarshal MultiSigTx %v", rErr)
		}
	}()

	dLen := len(data)
	if dLen < 1 {
		return fmt.Errorf("insufficient data to unmarshal AccountUrl for TokenTxAccountStatus")
	}
	err = t.SyntheticTxId.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("unable to unmarshal SyntheticTxId from TokenTxAccountStatus, %v", err)
	}
	i := t.SyntheticTxId.Size(nil)

	err = t.TokenTxOutput.UnmarshalBinary(data[i:])
	if err != nil {
		return fmt.Errorf("unable to unmarshal AccountUrl from TokenTxAccountStatus, %v", err)
	}
	return nil
}
