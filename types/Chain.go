package types

import "crypto/sha256"

//ChainSpec enumerates the latest versions of the Accumulate IMprovements (AIM)
// which define the validator specification
type ChainSpec string

const (
	ChainSpecDB    string = "AIM/0/0.1"
	ChainSpecAdi          = "AIM/1/0.1"
	ChainSpecToken        = "AIM/2/0.1"
)

//ChainType is the hash of the chain spec.
type ChainType Bytes32

var (
	//Define the Directory Block Chain Validator type
	ChainTypeDB ChainType = sha256.Sum256([]byte("AIM/0/0.1"))

	//Define the ADI chain validator type
	ChainTypeAdi ChainType = sha256.Sum256([]byte("AIM/1/0.1"))

	//Define the Token chain validator type
	ChainTypeToken ChainType = sha256.Sum256([]byte("AIM/2/0.1"))
)

// Chain will define a new chain to be registered. It will be initialized to the default state
// as defined by validator referenced by the ChainType
type Chain struct {
	URL       String    `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	ChainType ChainType `json:"chainType" form:"publicKeyHash" query:"publicKeyHash" validate:"required"` //",hexadecimal"`
}
