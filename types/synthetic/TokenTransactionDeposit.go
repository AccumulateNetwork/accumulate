package synthetic

import (
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"math/big"
)

type TokenTransactionDeposit struct {
	Header
	DepositAmount big.Int          `json:"amount"` //amount
	TokenUrl      types.String     `json:"tokenUrl"`
	Metadata      *json.RawMessage `json:"metadata,omitempty"`
}

const tokenTransactionDepositMinLen = HeaderLen + 32 + 32 + 32

func (tx *TokenTransactionDeposit) SetDeposit(txid []byte, amt *big.Int) error {
	if len(txid) != 32 {
		return fmt.Errorf("Invalid txid")
	}

	if amt == nil {
		return fmt.Errorf("No deposito amount specified")
	}

	if amt.Sign() <= 0 {
		return fmt.Errorf("Deposit amount must be greater than 0")
	}

	copy(tx.Txid[:], txid)
	tx.DepositAmount.Set(amt)

	return nil
}

func (tx *TokenTransactionDeposit) SetSenderInfo(senderidentity []byte, senderchainid []byte) error {
	if len(senderidentity) != 32 {
		return fmt.Errorf("Sender identity invalid")
	}

	if len(senderchainid) != 32 {
		return fmt.Errorf("Sender chain id invalid")
	}
	copy(tx.SourceIdentity[:], senderidentity)
	copy(tx.SourceChainId[:], senderchainid)
	return nil
}

func (tx *TokenTransactionDeposit) SetTokenInfo(tokenUrl types.UrlChain) error {
	//err := tokenUrl.MakeValid()
	//if err != nil {
	//	return err
	//}
	tx.TokenUrl = types.String(tokenUrl)
	return nil
}

func NewTokenTransactionDeposit() *TokenTransactionDeposit {
	tx := TokenTransactionDeposit{}
	return &tx
}

func (tx *TokenTransactionDeposit) Valid() error {
	if tx.DepositAmount.Sign() <= 0 {
		return fmt.Errorf("Invalid deposit amount, amount muct be greater than zero.")
	}

	return nil
}

func (tx *TokenTransactionDeposit) MarshalBinary() ([]byte, error) {
	err := tx.Valid()
	if err != nil {
		return nil, err
	}
	var md []byte
	if tx.Metadata != nil {
		bmd := types.Bytes(*tx.Metadata) //.MarshalJSON()
		md, err = bmd.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}

	txLen := tokenTransactionDepositMinLen
	txLen += len(md)
	ret := make([]byte, txLen)

	header, err := tx.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}

	i := copy(ret[:], header)

	tx.DepositAmount.FillBytes(ret[i : i+32])
	i += 32
	sdata, err := tx.TokenUrl.MarshalBinary()
	if err != nil {
		return nil, err
	}
	i += copy(ret[i:], sdata)

	if md != nil {
		i += copy(ret[i:], md)
	}
	return ret, nil
}

func (tx *TokenTransactionDeposit) UnmarshalBinary(data []byte) error {
	if tokenTransactionDepositMinLen > len(data) {
		return fmt.Errorf("Insufficient data to unmarshal for transaction deposit")
	}

	err := tx.Header.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	i := HeaderLen
	tx.DepositAmount.SetBytes(data[i : i+32])
	i += 32

	if len(data) < i {
		return fmt.Errorf("unable to unmarshal binary before token url")
	}

	err = tx.TokenUrl.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	i += tx.TokenUrl.Size(nil)

	if i < len(data) {
		var b types.Bytes
		err = b.UnmarshalBinary(data[i:])
		if err != nil {
			return err
		}
		tx.Metadata = &json.RawMessage{}
		copy(*tx.Metadata, b)
	}

	return nil
}
