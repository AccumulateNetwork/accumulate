package cmd

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"

	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/privval"
	"github.com/tyler-smith/go-bip32"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func resolvePrivateKey(s string) ([]byte, error) {
	pub, priv, err := parseKey(s)
	if err != nil {
		return nil, err
	}

	if priv != nil {
		return priv, nil
	}

	return LookupByPubKey(pub)
}

func resolvePublicKey(s string) ([]byte, error) {
	pub, _, err := parseKey(s)
	if err != nil {
		return nil, err
	}

	return pub, nil
}

func parseKey(s string) (pubKey, privKey []byte, err error) {
	pubKey, err = pubKeyFromString(s)
	if err == nil {
		return pubKey, nil, nil
	}

	privKey, err = LookupByLabel(s)
	if err == nil {
		// Assume ED25519
		return privKey[32:], privKey, nil
	}

	b, err := ioutil.ReadFile(s)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot resolve signing key, invalid key specifier: %q is not a label, key, or file", s)
	}

	var pvkey privval.FilePVKey
	if tmjson.Unmarshal(b, &pvkey) == nil {
		return pvkey.PubKey.Bytes(), pvkey.PrivKey.Bytes(), nil
	}

	return nil, nil, fmt.Errorf("cannot resolve signing key, invalid key specifier: %q is in an unsupported format", s)
}

func pubKeyFromString(s string) ([]byte, error) {
	var pubKey types.Bytes32
	if len(s) != 64 {
		return nil, fmt.Errorf("invalid public key or wallet key name")
	}
	i, err := hex.Decode(pubKey[:], []byte(s))

	if err != nil {
		return nil, err
	}

	if i != 32 {
		return nil, fmt.Errorf("invalid public key")
	}

	return pubKey[:], nil
}

func GeneratePrivateKey() (privKey []byte, err error) {
	seed, err := lookupSeed()

	if err != nil {
		//if private key seed doesn't exist, just create a key
		_, privKey, err = ed25519.GenerateKey(nil)
		if err != nil {
			return nil, err
		}
	} else {
		//if we do have a seed, then create a new key
		masterKey, _ := bip32.NewMasterKey(seed)

		ct, err := getKeyCountAndIncrement()
		if err != nil {
			return nil, err
		}

		newKey, err := masterKey.NewChildKey(ct)
		if err != nil {
			return nil, err
		}
		privKey = ed25519.NewKeyFromSeed(newKey.Key)
	}
	return
}

func getKeyCountAndIncrement() (count uint32, err error) {

	ct, err := Db.Get(BucketMnemonic, []byte("count"))
	if ct != nil {
		count = binary.LittleEndian.Uint32(ct)
	}

	ct = make([]byte, 8)
	binary.LittleEndian.PutUint32(ct, count+1)
	err = Db.Put(BucketMnemonic, []byte("count"), ct)
	if err != nil {
		return 0, err
	}

	return count, nil
}
