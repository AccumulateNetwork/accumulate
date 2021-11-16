package synthetic

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/types"
)

// Deprecated: Use protocol.SyntheticCreateChain
type TokenAccountCreate struct {
	Header
	TokenURL types.String `json:"tokenURL" form:"tokenURL" query:"tokenURL" validate:"required,uri"`
}

func (tac *TokenAccountCreate) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer

	buf.Write(common.Uint64Bytes(types.TxSyntheticTokenAccountCreate.AsUint64()))

	data, err := tac.Header.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshalling header, %v", err)
	}
	buf.Write(data)

	data, err = tac.TokenURL.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshalling TokenURL, %v", err)
	}
	buf.Write(data)

	return buf.Bytes(), nil
}

func (tac *TokenAccountCreate) UnmarshalBinary(data []byte) error {
	txType, data := common.BytesUint64(data)
	if txType != types.TxSyntheticTokenAccountCreate.AsUint64() {
		return fmt.Errorf("expected %v, got %v", types.TxSyntheticTokenAccountCreate, txType)
	}

	err := tac.Header.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("error unmarshalling header, %v", err)
	}
	data = data[tac.Header.Size():]

	err = tac.TokenURL.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("error unmarshalling TokenURL, %v", err)
	}

	return nil
}
