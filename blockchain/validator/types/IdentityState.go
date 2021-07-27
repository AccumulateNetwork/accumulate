package types

import (
	"crypto/sha256"
	"fmt"
)

type IdentityState struct {
	Publickey [32]byte
	Adi       string
}

func (app *IdentityState) GetPublicKey() []byte {
	return app.Publickey[:]
}

func (app *IdentityState) GetAdi() string {
	return app.Adi
}

func (app *IdentityState) GetIdentityChainId() []byte {
	h := sha256.Sum256([]byte(app.Adi))
	return h[:]
}

func (app *IdentityState) GetIdentityAddress() uint64 {
	return GetAddressFromIdentityChain(app.GetIdentityChainId())
}

func (app *IdentityState) MarshalBinary() ([]byte, error) {
	badi := []byte(app.Adi)
	data := make([]byte, len(badi)+32)
	i := copy(data[:], app.Publickey[:])
	copy(data[i:], badi)
	return data, nil
}

func (app *IdentityState) UnmarshalBinary(data []byte) error {
	if len(data) < 32+1 {
		return fmt.Errorf("insufficent data")
	}
	i := copy(app.Publickey[:], data[:32])
	app.Adi = string([]byte(data[i:]))

	return nil
}

//
//func (app *IdentityState) MarshalEntry() (*Entry, error) {
//	return nil, nil
//}
//func (app *IdentityState) UnmarshalEntry(entry *Entry) error {
//	return nil
//}
