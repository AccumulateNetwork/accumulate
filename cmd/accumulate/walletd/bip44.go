// Copyright 2016 Factom Foundation
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.

package walletd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"strconv"
	"strings"

	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

const Purpose uint32 = 0x8000002C

//https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki
//https://github.com/satoshilabs/slips/blob/master/slip-0044.md
//https://github.com/FactomProject/FactomDocs/blob/master/wallet_info/wallet_test_vectors.md

// DefaultRootDerivationPath is the root path to which custom derivation endpoints
// are appended. As such, the first account will be at m/44'/60'/0'/0, the second
// at m/44'/60'/0'/1, etc.
//var DefaultAccumulateRootDerivationPath = Derivation{TypeEther, 0x80000000 + 0, 0, 0}

// DefaultAccumulateBaseDerivationPath is the base path from which custom derivation endpoints
// are incremented. As such, the first account will be at m/44'/60'/0'/0/0, the second
// at m/44'/281'/0'/0/1, etc.
var DefaultAccumulateBaseDerivationPath = Derivation{TypeAccumulate, 0x80000000 + 0, 0, 0}

// DefaultEtherBaseDerivationPath is the base path from which custom derivation endpoints
// are incremented. As such, the first account will be at m/44'/60'/0'/0/0, the second
// at m/44'/281'/0'/0/1, etc.
var DefaultEtherBaseDerivationPath = Derivation{TypeEther, 0x80000000 + 0, 0, 0}

// DefaultFactoidBaseDerivationPath is the base path from which custom derivation endpoints
// are incremented. As such, the first account will be at m/44'/60'/0'/0/0, the second
// at m/44'/281'/0'/0/1, etc.
var DefaultFactoidBaseDerivationPath = Derivation{TypeFactomFactoids, 0x80000000 + 0, 0, 0}

// DefaultBitcoinBaseDerivationPath is the base path from which custom derivation endpoints
// are incremented. As such, the first account will be at m/44'/60'/0'/0/0, the second
// at m/44'/281'/0'/0/1, etc.
var DefaultBitcoinBaseDerivationPath = Derivation{TypeBitcoin, 0x80000000 + 0, 0, 0}

const (
	TypeBitcoin               uint32 = 0x80000000
	TypeTestnet               uint32 = 0x80000001
	TypeLitecoin              uint32 = 0x80000002
	TypeDogecoin              uint32 = 0x80000003
	TypeReddcoin              uint32 = 0x80000004
	TypeDash                  uint32 = 0x80000005
	TypePeercoin              uint32 = 0x80000006
	TypeNamecoin              uint32 = 0x80000007
	TypeFeathercoin           uint32 = 0x80000008
	TypeCounterparty          uint32 = 0x80000009
	TypeBlackcoin             uint32 = 0x8000000a
	TypeNuShares              uint32 = 0x8000000b
	TypeNuBits                uint32 = 0x8000000c
	TypeMazacoin              uint32 = 0x8000000d
	TypeViacoin               uint32 = 0x8000000e
	TypeClearingHouse         uint32 = 0x8000000f
	TypeRubycoin              uint32 = 0x80000010
	TypeGroestlcoin           uint32 = 0x80000011
	TypeDigitalcoin           uint32 = 0x80000012
	TypeCannacoin             uint32 = 0x80000013
	TypeDigiByte              uint32 = 0x80000014
	TypeOpenAssets            uint32 = 0x80000015
	TypeMonacoin              uint32 = 0x80000016
	TypeClams                 uint32 = 0x80000017
	TypePrimecoin             uint32 = 0x80000018
	TypeNeoscoin              uint32 = 0x80000019
	TypeJumbucks              uint32 = 0x8000001a
	TypeziftrCOIN             uint32 = 0x8000001b
	TypeVertcoin              uint32 = 0x8000001c
	TypeNXT                   uint32 = 0x8000001d
	TypeBurst                 uint32 = 0x8000001e
	TypeMonetaryUnit          uint32 = 0x8000001f
	TypeZoom                  uint32 = 0x80000020
	TypeVpncoin               uint32 = 0x80000021
	TypeCanadaeCoin           uint32 = 0x80000022
	TypeShadowCash            uint32 = 0x80000023
	TypeParkByte              uint32 = 0x80000024
	TypePandacoin             uint32 = 0x80000025
	TypeStartCOIN             uint32 = 0x80000026
	TypeMOIN                  uint32 = 0x80000027
	TypeArgentum              uint32 = 0x8000002D
	TypeGlobalCurrencyReserve uint32 = 0x80000031
	TypeNovacoin              uint32 = 0x80000032
	TypeAsiacoin              uint32 = 0x80000033
	TypeBitcoindark           uint32 = 0x80000034
	TypeDopecoin              uint32 = 0x80000035
	TypeTemplecoin            uint32 = 0x80000036
	TypeAIB                   uint32 = 0x80000037
	TypeEDRCoin               uint32 = 0x80000038
	TypeSyscoin               uint32 = 0x80000039
	TypeSolarcoin             uint32 = 0x8000003a
	TypeSmileycoin            uint32 = 0x8000003b
	TypeEther                 uint32 = 0x8000003c
	TypeEtherClassic          uint32 = 0x8000003d
	TypeOpenChain             uint32 = 0x80000040
	TypeOKCash                uint32 = 0x80000045
	TypeDogecoinDark          uint32 = 0x8000004d
	TypeElectronicGulden      uint32 = 0x8000004e
	TypeClubCoin              uint32 = 0x8000004f
	TypeRichCoin              uint32 = 0x80000050
	TypePotcoin               uint32 = 0x80000051
	TypeQuarkcoin             uint32 = 0x80000052
	TypeTerracoin             uint32 = 0x80000053
	TypeGridcoin              uint32 = 0x80000054
	TypeAuroracoin            uint32 = 0x80000055
	TypeIXCoin                uint32 = 0x80000056
	TypeGulden                uint32 = 0x80000057
	TypeBitBean               uint32 = 0x80000058
	TypeBata                  uint32 = 0x80000059
	TypeMyriadcoin            uint32 = 0x8000005a
	TypeBitSend               uint32 = 0x8000005b
	TypeUnobtanium            uint32 = 0x8000005c
	TypeMasterTrader          uint32 = 0x8000005d
	TypeGoldBlocks            uint32 = 0x8000005e
	TypeSaham                 uint32 = 0x8000005f
	TypeChronos               uint32 = 0x80000060
	TypeUbiquoin              uint32 = 0x80000061
	TypeEvotion               uint32 = 0x80000062
	TypeSaveTheOcean          uint32 = 0x80000063
	TypeBigUp                 uint32 = 0x80000064
	TypeGameCredits           uint32 = 0x80000065
	TypeDollarcoins           uint32 = 0x80000066
	TypeZayedcoin             uint32 = 0x80000067
	TypeDubaicoin             uint32 = 0x80000068
	TypeStratis               uint32 = 0x80000069
	TypeShilling              uint32 = 0x8000006a
	TypePiggyCoin             uint32 = 0x80000076
	TypeMonero                uint32 = 0x80000080
	TypeNavCoin               uint32 = 0x80000082
	TypeFactomFactoids        uint32 = 0x80000083
	TypeFactomEntryCredits    uint32 = 0x80000084
	TypeZcash                 uint32 = 0x80000085
	TypeLisk                  uint32 = 0x80000086
	TypeAccumulate            uint32 = 0x80000119
)

func NewKeyFromMnemonic(mnemonic string, coin, account, chain, address uint32) (*bip32.Key, error) {
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, "")
	if err != nil {
		return nil, err
	}

	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		return nil, err
	}

	return NewKeyFromMasterKey(masterKey, coin, account, chain, address)
}

func NewKeyFromMasterKey(masterKey *bip32.Key, coin, account, chain, address uint32) (*bip32.Key, error) {
	child, err := masterKey.NewChildKey(Purpose)
	if err != nil {
		return nil, err
	}

	child, err = child.NewChildKey(coin)
	if err != nil {
		return nil, err
	}

	child, err = child.NewChildKey(account)
	if err != nil {
		return nil, err
	}

	child, err = child.NewChildKey(chain)
	if err != nil {
		return nil, err
	}

	child, err = child.NewChildKey(address)
	if err != nil {
		return nil, err
	}

	return child, nil
}

//Derivation BIP44 hierarchical derivation path
type Derivation struct {
	CoinType uint32
	Account  uint32
	Chain    uint32
	Address  uint32
}

func (d *Derivation) Validate() error {
	if d.CoinType < TypeBitcoin {
		return fmt.Errorf("invalid coin type")
	}

	if d.Account < bip32.FirstHardenedChild {
		return fmt.Errorf("account not hardened, %d", d.Account)
	}

	return nil
}

func (d *Derivation) ToPath() (string, error) {
	err := d.Validate()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("m/44'/%d'/%d'/%d/%d",
		TypeBitcoin^d.CoinType,
		bip32.FirstHardenedChild^d.Account,
		d.Chain, d.Address), nil
}

func (d *Derivation) String() string {
	s, _ := d.ToPath()
	return s
}

func (d *Derivation) Parse(path string) error {
	return d.FromPath(path)
}

func ParseDerivationPath(path string) (Derivation, error) {
	d := Derivation{}
	err := d.Parse(path)
	return d, err
}

func NewDerivationPath(signatureType protocol.SignatureType) (d Derivation, e error) {
	switch signatureType {
	case protocol.SignatureTypeBTC:
		d = DefaultBitcoinBaseDerivationPath
	case protocol.SignatureTypeRCD1:
		d = DefaultFactoidBaseDerivationPath
	case protocol.SignatureTypeETH:
		d = DefaultEtherBaseDerivationPath
	case protocol.SignatureTypeED25519:
		d = DefaultAccumulateBaseDerivationPath
	default:
		e = fmt.Errorf("unsupported derivation path")
	}
	return d, e
}

func (d *Derivation) FromPath(path string) error {
	hd := strings.Split(path, "/")
	if len(hd) != 6 {
		return fmt.Errorf("insufficent parameters in bip44 derivation path")
	}

	//identify purpose
	if hd[0] != "m" && hd[1] != "44'" {
		return fmt.Errorf("invalid purpose, expecting bip44 HD derivation path, but received %s", path)
	}

	t := [4]uint32{}
	for i, s := range hd[2:6] {
		if strings.HasSuffix(s, "'") {
			t[i] = bip32.FirstHardenedChild
			s = strings.TrimSuffix(s, "'")
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("malformed bip44 HD derivation path, %v", err)
		}
		t[i] += uint32(n)
	}
	d.CoinType = t[0]
	d.Account = t[1]
	d.Chain = t[2]
	d.Address = t[3]

	return d.Validate()
}

func (d *Derivation) SignatureType() protocol.SignatureType {
	t := protocol.SignatureTypeED25519
	//derivationPath
	switch d.CoinType {
	case TypeBitcoin:
		t = protocol.SignatureTypeBTC
	case TypeAccumulate:
		t = protocol.SignatureTypeED25519
	case TypeEther:
		t = protocol.SignatureTypeETH
	case TypeFactomFactoids:
		t = protocol.SignatureTypeRCD1
	default:
		return protocol.SignatureTypeUnknown
	}
	return t
}

func (d *Derivation) MarshalBinary() ([]byte, error) {
	buffer := bytes.Buffer{}

	err := d.Validate()
	if err != nil {
		return nil, err
	}

	b := [4]byte{}
	binary.BigEndian.PutUint32(b[:], 0x80000000+44)
	buffer.Write(b[:])

	binary.BigEndian.PutUint32(b[:], d.CoinType)
	buffer.Write(b[:])

	binary.BigEndian.PutUint32(b[:], d.Account)
	buffer.Write(b[:])

	binary.BigEndian.PutUint32(b[:], d.Chain)
	buffer.Write(b[:])

	binary.BigEndian.PutUint32(b[:], d.Address)
	buffer.Write(b[:])

	return buffer.Bytes(), nil
}

func (d *Derivation) UnmarshalBinary(buffer []byte) error {

	if len(buffer) < 16 {
		return fmt.Errorf("derivation path too short")
	}

	m := binary.BigEndian.Uint32(buffer)

	if m != 0x80000000+44 {
		return fmt.Errorf("encoded binary is not a derivation path")
	}

	d.CoinType = binary.BigEndian.Uint32(buffer[4:])
	d.Account = binary.BigEndian.Uint32(buffer[8:])
	d.Chain = binary.BigEndian.Uint32(buffer[12:])
	if len(buffer) >= 20 {
		d.Address = binary.BigEndian.Uint32(buffer[16:])
	}
	return d.Validate()
}
