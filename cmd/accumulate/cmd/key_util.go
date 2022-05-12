package cmd

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Key struct {
	PublicKey  []byte
	PrivateKey []byte
	Type       protocol.SignatureType
}

func (k *Key) PublicKeyHash() []byte {
	switch k.Type {
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
		panic(fmt.Errorf("unsupported signature type %v", k.Type))
	}
}

func (k *Key) Save(label, liteLabel string) error {
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

	err = GetWallet().Put(BucketSigType, k.PublicKey, encoding.UvarintMarshalBinary(k.Type.GetEnumValue()))
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

	b, err := GetWallet().Get(BucketSigType, k.PublicKey)
	switch {
	case err == nil:
		v, err := encoding.UvarintUnmarshalBinary(b)
		if err != nil {
			return err
		}
		if !k.Type.SetEnumValue(v) {
			return fmt.Errorf("invalid key type for %x", publicKey)
		}

	case errors.Is(err, db.ErrNotFound),
		errors.Is(err, db.ErrNoBucket):
		k.Type = protocol.SignatureTypeED25519

	default:
		return err
	}

	return nil
}
