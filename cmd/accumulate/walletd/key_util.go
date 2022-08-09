package walletd

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/tyler-smith/go-bip32"
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

func (k *Key) InitializeFromSeed(seed []byte, signatureType protocol.SignatureType, derivation string) error {
	k.KeyInfo.Type = signatureType
	k.KeyInfo.Derivation = derivation
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

func (k *Key) NativeAddress() (address string, err error) {
	switch k.KeyInfo.Type {
	case protocol.SignatureTypeRCD1:
		address, err = protocol.GetFactoidAddressFromRCDHash(k.PublicKeyHash())
	case protocol.SignatureTypeBTC, protocol.SignatureTypeBTCLegacy:
		address = protocol.BTCaddress(k.PublicKeyHash())
	case protocol.SignatureTypeETH:
		address = protocol.ETHaddress(k.PublicKeyHash())
	default:
		u := protocol.LiteAuthorityForKey(k.PublicKey, protocol.SignatureTypeED25519)
		address = u.Hostname()
	}
	return address, err
}

func GenerateKey(sigtype protocol.SignatureType) (*Key, error) {
	seed, err := lookupSeed()
	if err != nil {
		return nil, fmt.Errorf("wallet not created, please create a seeded wallet \"accumulate walleet init\"")
	}
	account := uint32(0)
	chain := uint32(0)
	address, err := getKeyCountAndIncrement()

	if err != nil {
		return nil, err
	}

	//if we do have a seed, then create a new key
	masterKey, _ := bip32.NewMasterKey(seed)

	keyType := TypeAccumulate
	switch sigtype {
	case protocol.SignatureTypeBTCLegacy, protocol.SignatureTypeBTC:
		keyType = TypeBitcoin
	case protocol.SignatureTypeETH:
		keyType = TypeEther
	}

	//create the derived key
	newKey, err := NewKeyFromMasterKey(masterKey, keyType, account, chain, address)
	if err != nil {
		return nil, err
	}
	derivationPath := fmt.Sprintf("m/44'/%d'/%d'/%d/%d", keyType, account, chain, address)
	key := new(Key)
	key.InitializeFromSeed(newKey.Key, sigtype, derivationPath)

	return key, nil
}

func getKeyCountAndIncrement() (count uint32, err error) {
	ct, _ := GetWallet().Get(BucketMnemonic, []byte("count"))
	if ct != nil {
		count = binary.LittleEndian.Uint32(ct)
	}

	ct = make([]byte, 8)
	binary.LittleEndian.PutUint32(ct, count+1)
	err = GetWallet().Put(BucketMnemonic, []byte("count"), ct)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func lookupSeed() (seed []byte, err error) {
	seed, err = GetWallet().Get(BucketMnemonic, []byte("seed"))
	if err != nil {
		return nil, fmt.Errorf("mnemonic seed doesn't exist")
	}

	return seed, nil
}
