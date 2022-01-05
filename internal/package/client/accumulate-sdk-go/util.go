package sdk

import (
	 "github.com/AccumulateNetwork/accumulate/internal/url"
	 "github.com/AccumulateNetwork/accumulate/types/api/transactions"
)
type KeyManager interface {
	Sign(nonce uint64, privateKey []byte, hash []byte) error
}

type Signer struct {
	origin *url.URL
	header *transactions.Header
	privateKey []byte
	nonce uint64
}

type GenSigner struct {
	Nonce uint64
	PrivateKey []byte
	Signature []byte
}

/*
type GenTxn struct {
	Header *transactions.Header
	Body *transactions.Body
	txHash []byte
}

func NewGenTxn(origin string, height uint64) {}


func (tx GenTxn) GetSig
*/