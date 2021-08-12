package synthetic

import (
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"math/big"
)

type TokenTransactionDeposit struct {
	Txid           types.Bytes32   `json:"txid"`            //txid of original tx
	DepositAmount  big.Int         `json:"amount"`          //amount
	SenderIdentity types.Bytes32   `json:"sender-id"`       //sender of amount
	SenderChainId  types.Bytes32   `json:"sender-chain-id"` // sender's token chain
	IssuerIdentity types.Bytes32   `json:"issuer-identity"` //token issuer's identity chain
	IssuerChainId  types.Bytes32   `json:"issuer-chain-id"` //token issuer's chain id
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

const tokenTransactionDepositMinLen = 32 + 32 + 32 + 32 + 32 + 32

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
	copy(tx.IssuerIdentity[:], senderidentity)
	copy(tx.IssuerChainId[:], senderchainid)
	return nil
}

func (tx *TokenTransactionDeposit) SetTokenInfo(issueridentity []byte, issuerchainid []byte) error {
	if len(issueridentity) != 32 {
		return fmt.Errorf("Issuer identity invalid")
	}

	if len(issuerchainid) != 32 {
		return fmt.Errorf("Issuer chain id invalid")
	}
	copy(tx.IssuerIdentity[:], issueridentity)
	copy(tx.IssuerChainId[:], issuerchainid)
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
	md, err := tx.Metadata.MarshalJSON()
	if err != nil {
		return nil, err
	}

	txLen := tokenTransactionDepositMinLen
	txLen += len(md)
	ret := make([]byte, txLen)

	i := copy(ret[:], tx.Txid[:])
	tx.DepositAmount.FillBytes(ret[i : i+32])
	i += 32
	i += copy(ret[i:], tx.SenderIdentity[:])
	i += copy(ret[i:], tx.SenderChainId[:])
	i += copy(ret[i:], tx.IssuerIdentity[:])
	i += copy(ret[i:], tx.IssuerChainId[:])
	if md != nil {
		i += copy(ret[i:], md)
	}
	return ret, nil
}

func (tx *TokenTransactionDeposit) UnmarshalBinary(data []byte) error {
	if tokenTransactionDepositMinLen > len(data) {
		return fmt.Errorf("Insufficient data to unmarshal for transaction deposit")
	}

	i := copy(tx.Txid[:], data[:])
	tx.DepositAmount.SetBytes(data[i : i+32])
	i += 32
	i += copy(tx.SenderIdentity[:], data[i:])
	i += copy(tx.SenderChainId[:], data[i:])
	i += copy(tx.IssuerIdentity[:], data[i:])
	i += copy(tx.IssuerChainId[:], data[i:])

	if i < len(data) {
		err := tx.Metadata.UnmarshalJSON(data[i:])
		if err != nil {
			return err
		}
	}

	return nil
}
