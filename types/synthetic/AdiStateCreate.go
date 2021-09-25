package synthetic

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/smt/common"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/proto"
)

type AdiStateCreate struct {
	Header
	PublicKeyHash types.Bytes32 `json:"publicKeyHash" form:"publicKeyHash" query:"publicKeyHash" validate:"required"`
}

func NewAdiStateCreate(txid types.Bytes, from *types.String, to *types.String, keyHash *types.Bytes32) *AdiStateCreate {
	ctas := &AdiStateCreate{}
	ctas.SetHeader(txid, from, to)
	ctas.PublicKeyHash = *keyHash

	return ctas
}

func (a *AdiStateCreate) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer

	buf.Write(common.Uint64Bytes(uint64(proto.AccInstruction_Synthetic_Identity_Creation)))
	data, err := a.Header.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal header for Adi State Create message, %v", err)
	}

	buf.Write(data)
	buf.Write(a.PublicKeyHash[:])
	return buf.Bytes(), nil
}

func (a *AdiStateCreate) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("error marshaling Pending Transaction State %v", err)
		}
	}()

	txType, data := common.BytesUint64(data)
	if txType != uint64(proto.AccInstruction_Synthetic_Identity_Creation) {
		return fmt.Errorf("data is not of a identity creation type")
	}
	err = a.Header.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("insufficient data to unmarshal Adi State Create message header, %v", err)
	}
	i := 1 + a.Header.Size()
	if len(data) < i+32 {
		return fmt.Errorf("insufficient data to unmarshal Adi State Create message key hash")
	}
	copy(a.PublicKeyHash[:], data[i:])

	return err
}
