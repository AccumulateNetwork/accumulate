package types

import (
	"crypto/sha256"
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
	ChainSpecTransaction             = "AIM/5/0.1"
)

//ChainType is the hash of the chain spec.
type ChainType Bytes32

func (c *ChainType) AsBytes32() *Bytes32 {
	return (*Bytes32)(c)
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

	//ChainTypeTransaction Defines the transaction chains for pending and accepted transactions
	ChainTypeTransaction ChainType = sha256.Sum256([]byte(ChainSpecTransaction))
)

var ChainTypeSpecMap = map[Bytes32]string{
	*ChainTypeDC.AsBytes32():               ChainSpecDC,
	*ChainTypeAdi.AsBytes32():              ChainSpecAdi,
	*ChainTypeToken.AsBytes32():            ChainSpecToken,
	*ChainTypeTokenAccount.AsBytes32():     ChainSpecTokenAccount,
	*ChainTypeAnonTokenAccount.AsBytes32(): ChainSpecAnonTokenAccount,
	*ChainTypeTransaction.AsBytes32():      ChainSpecTransaction,
}
