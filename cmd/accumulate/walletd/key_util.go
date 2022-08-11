package walletd

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"runtime/debug"

	btc "github.com/btcsuite/btcd/btcec"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run ../../../tools/cmd/gen-types --package walletd --out key_info_gen.go key_info.yml

type Key struct {
	PublicKey  []byte
	PrivateKey []byte
	KeyInfo    KeyInfo
}

func (k *Key) PublicKeyHash() []byte {
	switch k.KeyInfo.Type {
	case protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519:
		hash := sha256.Sum256(k.PublicKey)
		return hash[:]

	case protocol.SignatureTypeRCD1:
		return protocol.GetRCDHashFromPublicKey(k.PublicKey, 1)

	case protocol.SignatureTypeBTC, protocol.SignatureTypeBTCLegacy:
		return protocol.BTCHash(k.PublicKey)

	case protocol.SignatureTypeETH:
		return protocol.ETHhash(k.PublicKey)

	default:
		debug.PrintStack()
		panic(fmt.Errorf("cannot hash key for unsupported signature type %v(%d)", k.KeyInfo.Type, k.KeyInfo.Type.GetEnumValue()))
	}
}

func (k *Key) Save(label, liteLabel string) error {
	if k.KeyInfo.Type == protocol.SignatureTypeUnknown {
		return fmt.Errorf("signature type is was not specified")
	}

	err := GetWallet().Put(BucketKeys, k.PublicKey, k.PrivateKey)
	if err != nil {
		return err
	}

	err = GetWallet().Put(BucketLabel, []byte(label), k.PublicKey)
	if err != nil {
		return err
	}

	err = GetWallet().Put(BucketLite, []byte(liteLabel), []byte(label))
	if err != nil {
		return err
	}

	data, err := k.KeyInfo.MarshalBinary()
	if err != nil {
		return err
	}

	err = GetWallet().Put(BucketKeyInfo, k.PublicKey, data)
	if err != nil {
		return err
	}

	return nil
}

func (k *Key) LoadByLabel(label string) error {
	label, _ = LabelForLiteTokenAccount(label)

	pubKey, err := GetWallet().Get(BucketLabel, []byte(label))
	if err != nil {
		return fmt.Errorf("valid key not found for %s", label)
	}

	return k.LoadByPublicKey(pubKey)
}

func (k *Key) LoadByPublicKey(publicKey []byte) error {
	k.PublicKey = publicKey

	var err error
	k.PrivateKey, err = GetWallet().Get(BucketKeys, k.PublicKey)
	if err != nil {
		return fmt.Errorf("private key not found for %x", publicKey)
	}

	b, err := GetWallet().Get(BucketKeyInfo, k.PublicKey)
	if err != nil {
		return fmt.Errorf("key type info not found for key %x", k.PublicKey)
	}

	err = k.KeyInfo.UnmarshalBinary(b)
	if err != nil {
		return fmt.Errorf("cannot unmarshal key information for key %x", k.PublicKey)
	}

	return nil
}

func (k *Key) Initialize(seed []byte, signatureType protocol.SignatureType) error {
	k.KeyInfo.Type = signatureType
	switch k.KeyInfo.Type {
	case protocol.SignatureTypeLegacyED25519, protocol.SignatureTypeED25519, protocol.SignatureTypeRCD1:
		if len(seed) != ed25519.SeedSize && len(seed) != ed25519.PrivateKeySize {
			return fmt.Errorf("invalid private key length, expected %d or %d bytes", ed25519.SeedSize, ed25519.PrivateKeySize)
		}
		pk := ed25519.NewKeyFromSeed(seed[:ed25519.SeedSize])
		k.PrivateKey = pk
		k.PublicKey = pk[ed25519.SeedSize:]
	case protocol.SignatureTypeBTC:
		if len(seed) != btc.PrivKeyBytesLen {
			return fmt.Errorf("invalid private key length, expected %d bytes", btc.PrivKeyBytesLen)
		}
		pvkey, pubKey := btc.PrivKeyFromBytes(btc.S256(), seed)
		k.PrivateKey = pvkey.Serialize()
		k.PublicKey = pubKey.SerializeCompressed()
	case protocol.SignatureTypeBTCLegacy, protocol.SignatureTypeETH:
		if len(seed) != btc.PrivKeyBytesLen {
			return fmt.Errorf("invalid private key length, expected %d bytes", btc.PrivKeyBytesLen)
		}
		pvkey, pubKey := btc.PrivKeyFromBytes(btc.S256(), seed)
		k.PrivateKey = pvkey.Serialize()
		k.PublicKey = pubKey.SerializeUncompressed()
	default:
		return fmt.Errorf("unsupported signature type %v", k.KeyInfo.Type)
	}

	return nil
}
