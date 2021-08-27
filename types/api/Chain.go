package api

import (
	"crypto/sha256"
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

// Chain will define a new chain to be registered. It will be initialized to the default state
// as defined by validator referenced by the ChainType
type Chain struct {
	URL       types.String `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	ChainType ChainType    `json:"chainType" form:"chainType" query:"chainType" validate:"required"`
}
