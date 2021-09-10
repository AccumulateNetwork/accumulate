package api

import (
	"crypto/sha256"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
)

//ChainSpec enumerates the latest versions of the Accumulate IMprovements (AIM)
// which define the validator specification
type ChainSpec string

const (
	ChainSpecDC               string = "AIM/0/0.1"
	ChainSpecAdi                     = "AIM/1/0.1"
	ChainSpecToken                   = "AIM/2/0.1"
	ChainSpecTokenAccount            = "AIM/3/0.1"
	ChainSpecAnonTokenAccount        = "AIM/4/0.1"
)

//ChainType is the hash of the chain spec.
type ChainType types.Bytes32

func (c *ChainType) AsBytes32() *types.Bytes32 {
	return (*types.Bytes32)(c)
}

var (
	//ChainTypeDC Define the Directory Block Chain Validator type
	ChainTypeDC ChainType = sha256.Sum256([]byte(ChainSpecDC))

	//ChainTypeAdi Define the ADI chain validator type
	ChainTypeAdi ChainType = sha256.Sum256([]byte(ChainSpecAdi))

	//ChainTypeToken Define the Token chain validator type
	ChainTypeToken ChainType = sha256.Sum256([]byte(ChainSpecToken))

	//ChainTypeTokenAccount Define an Anonymous Token chain validator type
	ChainTypeTokenAccount ChainType = sha256.Sum256([]byte(ChainSpecTokenAccount))

	//ChainTypeAnonTokenAccount Define an Anonymous Token chain validator type
	ChainTypeAnonTokenAccount ChainType = sha256.Sum256([]byte(ChainSpecAnonTokenAccount))
)

var ChainTypeSpecMap = map[types.Bytes32]string{
	*ChainTypeDC.AsBytes32():               ChainSpecDC,
	*ChainTypeAdi.AsBytes32():              ChainSpecAdi,
	*ChainTypeToken.AsBytes32():            ChainSpecToken,
	*ChainTypeTokenAccount.AsBytes32():     ChainSpecTokenAccount,
	*ChainTypeAnonTokenAccount.AsBytes32(): ChainSpecAnonTokenAccount,
}

// Chain will define a new chain to be registered. It will be initialized to the default state
// as defined by validator referenced by the ChainType
type Chain struct {
	ChainType ChainType    `json:"chainType" form:"chainType" query:"chainType" validate:"required"`
	URL       types.String `json:"url" form:"url" query:"url" validate:"required,alphanum"`
}

func (c *Chain) Size() int {
	return 32 + c.URL.Size(nil)
}

func (c *Chain) MarshalBinary() ([]byte, error) {
	idn, err := c.URL.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data := make([]byte, len(idn)+32)
	i := copy(data[:], c.ChainType[:])
	copy(data[i:], idn)
	return data, nil
}

func (c *Chain) UnmarshalBinary(data []byte) error {

	if len(data) < 33 {
		return fmt.Errorf("insufficient data to decode chain header")
	}
	copy(c.ChainType[:], data[32:])

	err := c.URL.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	return nil
}
