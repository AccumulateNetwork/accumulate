package synthetic

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
)

type Header struct {
	Txid    types.Bytes32 `json:"txid" form:"txid" query:"txid" validate:"required"`
	FromUrl types.String  `json:"from" form:"from" query:"from" validate:"required"`
	ToUrl   types.String  `json:"to" form:"to" query:"to" validate:"required"`
}

// SetHeader helper function to make sure all information is set for the header
func (h *Header) SetHeader(txId types.Bytes, from *types.String, to *types.String) {
	if txId != nil {
		copy(h.Txid[:], txId)
	}

	if from != nil {
		h.FromUrl = *from
	}

	if to != nil {
		h.ToUrl = *to
	}
}

// Valid checks to see if the header is valid, returns error if not
func (h *Header) Valid() error {

	if len(h.FromUrl) == 0 {
		return fmt.Errorf("from URL not specified for token transaction deposit")
	}

	if len(h.ToUrl) == 0 {
		return fmt.Errorf("to URL not specified for token transaction deposit")
	}

	return nil
}

// Size returns the length of the header if it were marshalled
func (h *Header) Size() int {
	l := 32
	l += h.FromUrl.Size(nil)
	l += h.ToUrl.Size(nil)
	return l
}

// MarshalBinary serialize the header
func (h *Header) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer

	from, err := h.FromUrl.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshalling FromUrl, %v", err)
	}

	to, err := h.ToUrl.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshalling ToUrl, %v", err)
	}

	buf.Write(h.Txid[:])
	buf.Write(from)
	buf.Write(to)
	return buf.Bytes(), nil
}

// UnmarshalBinary deserialize the header
func (h *Header) UnmarshalBinary(data []byte) error {
	length := len(data)
	if length < 32 {
		return fmt.Errorf("insufficient data to unmarshal synthetic transaction header")
	}

	i := copy(h.Txid[:], data[:])

	if length < i {
		return fmt.Errorf("insufficient data to after txid for heaer")
	}

	err := h.FromUrl.UnmarshalBinary(data[i:])
	if err != nil {
		return fmt.Errorf("insufficient data to umarshal fromUrl in synthetic transaction header, %v", err)
	}

	i += h.FromUrl.Size(nil)

	if length < i {
		return fmt.Errorf("insufficient data to after fromUrl for heaer")
	}

	err = h.ToUrl.UnmarshalBinary(data[i:])
	if err != nil {
		return fmt.Errorf("insufficient data to umarshal toUrl in synthetic transaction header, %v", err)
	}

	return nil
}
