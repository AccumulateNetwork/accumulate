package signing

import "github.com/AccumulateNetwork/accumulate/types/api/transactions"

type SignatureData interface {
	isSignatureData()
}

type SingleSignatureData struct {
	Signature *transactions.ED25519Sig
}

type MultiSignatureData struct {
	Signatures []*transactions.ED25519Sig
}

var _, _ SignatureData = &SingleSignatureData{}, &MultiSignatureData{}

func (s *SingleSignatureData) isSignatureData() {}

func (m *MultiSignatureData) isSignatureData() {}