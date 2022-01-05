package crypto

import (
	 tmcrypto "github.com/tendermint/tendermint/crypto"
)


type PubKey interface {
	tmcrypto.PubKey

}

type PrivKey interface {
	tmcrypto.PrivKey
}

