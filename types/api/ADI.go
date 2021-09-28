package api

import (
	"bytes"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
)

// ADI structure holds the identity name in the URL.  The name can be stored as acc://<name> or simply <name>
// all chain paths following the ADI domain will be ignored
type ADI struct {
	URL           types.String  `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKeyHash types.Bytes32 `json:"publicKeyHash" form:"publicKeyHash" query:"publicKeyHash" validate:"required"`
}

func NewADI(name *string, keyHash *types.Bytes32) *ADI {
	ic := &ADI{}
	_ = ic.SetAdi(name)
	ic.SetKeyHash(keyHash)
	return ic
}

func (ic *ADI) SetAdi(name *string) error {
	adi, _, err := types.ParseIdentityChainPath(name)

	if err != nil {
		return err
	}
	ic.URL = types.String(adi)
	return nil
}

func (ic *ADI) SetKeyHash(hash *types.Bytes32) {
	if hash != nil {
		copy(ic.PublicKeyHash[:], hash[:])
	}
}

func (ic *ADI) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer
	buffer.Write(common.Int64Bytes(int64(types.TxTypeIdentityCreate)))

	idn, err := ic.URL.MarshalBinary()
	if err != nil {
		return nil, err
	}

	buffer.Write(idn)
	buffer.Write(ic.PublicKeyHash.Bytes())

	return buffer.Bytes(), nil
}

func (ic *ADI) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("insufficent data to unmarshal MultiSigTx %v", err)
		}
	}()
	txType, data := common.BytesInt64(data)
	if txType != int64(types.TxTypeIdentityCreate) {
		return fmt.Errorf("unable to unmarshal ADI, expecting identity type")
	}
	err = ic.URL.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	l := ic.URL.Size(nil)

	copy(ic.PublicKeyHash[:], data[l:])

	return nil
}
