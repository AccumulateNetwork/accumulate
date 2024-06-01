// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package address

import (
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"

	"github.com/btcsuite/btcutil/base58"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Address interface {
	String() string
	GetType() protocol.SignatureType
	GetPublicKeyHash() ([]byte, bool)
	GetPublicKey() ([]byte, bool)
	GetPrivateKey() ([]byte, bool)
}

type Unknown struct {
	Value    []byte
	Encoding rune
}

func (u *Unknown) GetType() protocol.SignatureType  { return protocol.SignatureTypeUnknown }
func (u *Unknown) GetPublicKeyHash() ([]byte, bool) { return nil, false }
func (u *Unknown) GetPublicKey() ([]byte, bool)     { return nil, false }
func (u *Unknown) GetPrivateKey() ([]byte, bool)    { return nil, false }

func (u *Unknown) String() string {
	switch u.Encoding {
	case multibase.Base58BTC:
		return base58.Encode(u.Value)
	default:
		return hex.EncodeToString(u.Value)
	}
}

type UnknownHash struct {
	Hash []byte
}

func (u *UnknownHash) String() string                   { return FormatMH(u.Hash, multihash.IDENTITY) }
func (u *UnknownHash) GetType() protocol.SignatureType  { return protocol.SignatureTypeUnknown }
func (u *UnknownHash) GetPublicKeyHash() ([]byte, bool) { return u.Hash, true }
func (u *UnknownHash) GetPublicKey() ([]byte, bool)     { return nil, false }
func (u *UnknownHash) GetPrivateKey() ([]byte, bool)    { return nil, false }

type UnknownMultihash multihash.DecodedMultihash

func (u *UnknownMultihash) String() string                   { return FormatMH(u.Digest, u.Code) }
func (u *UnknownMultihash) GetType() protocol.SignatureType  { return protocol.SignatureTypeUnknown }
func (u *UnknownMultihash) GetPublicKeyHash() ([]byte, bool) { return u.Digest, true }
func (u *UnknownMultihash) GetPublicKey() ([]byte, bool)     { return nil, false }
func (u *UnknownMultihash) GetPrivateKey() ([]byte, bool)    { return nil, false }

type PublicKeyHash struct {
	Type protocol.SignatureType
	Hash []byte
}

func (p *PublicKeyHash) String() string                   { return formatAddr(p.Type, p.Hash) }
func (p *PublicKeyHash) GetType() protocol.SignatureType  { return p.Type }
func (p *PublicKeyHash) GetPublicKeyHash() ([]byte, bool) { return p.Hash, true }
func (p *PublicKeyHash) GetPublicKey() ([]byte, bool)     { return nil, false }
func (p *PublicKeyHash) GetPrivateKey() ([]byte, bool)    { return nil, false }

type PublicKey struct {
	Type protocol.SignatureType
	Key  []byte
}

func (p *PublicKey) GetType() protocol.SignatureType { return p.Type }
func (p *PublicKey) GetPublicKey() ([]byte, bool)    { return p.Key, true }
func (p *PublicKey) GetPrivateKey() ([]byte, bool)   { return nil, false }

func (p *PublicKey) String() string {
	hash, ok := p.GetPublicKeyHash()
	if !ok {
		return "<invalid address>"
	}
	return formatAddr(p.Type, hash)
}

func (p *PublicKey) GetPublicKeyHash() ([]byte, bool) {
	b, err := protocol.PublicKeyHash(p.Key, p.Type)
	if err != nil {
		return nil, false
	}
	return b, true
}

type PrivateKey struct {
	PublicKey
	Key []byte
}

func (p *PrivateKey) GetPrivateKey() ([]byte, bool) { return p.Key, true }

func (p *PrivateKey) String() string {
	switch p.Type {
	case protocol.SignatureTypeED25519,
		protocol.SignatureTypeLegacyED25519:
		return FormatAS1(p.Key[:32])

	case protocol.SignatureTypeRCD1:
		return FormatFs(p.Key[:32])

	case protocol.SignatureTypeETH:
		return hex.EncodeToString(p.Key)

	case protocol.SignatureTypeBTCLegacy, protocol.SignatureTypeBTC:
		privKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), p.Key)

		wif, err := btcutil.NewWIF(privKey, &chaincfg.MainNetParams, true)
		if err != nil {
			return hex.EncodeToString(p.Key)
		}
		return wif.String()

	case protocol.SignatureTypeEcdsaSha256:
		return FormatAS2(p.Key)

	case protocol.SignatureTypeRsaSha256:
		return FormatAS3(p.Key)
	}

	//as a failsafe, just return the raw hex encoded bytes
	return fmt.Sprintf("%X", p.Key)
}

type Lite struct {
	Url   *url.URL
	Bytes []byte
}

func (l *Lite) String() string                   { return l.Url.String() }
func (l *Lite) GetType() protocol.SignatureType  { return protocol.SignatureTypeUnknown }
func (l *Lite) GetPublicKeyHash() ([]byte, bool) { return nil, false }
func (l *Lite) GetPublicKey() ([]byte, bool)     { return nil, false }
func (l *Lite) GetPrivateKey() ([]byte, bool)    { return nil, false }

func formatAddr(typ protocol.SignatureType, hash []byte) string {
	switch typ {
	case protocol.SignatureTypeED25519,
		protocol.SignatureTypeLegacyED25519:
		return FormatAC1(hash)

	case protocol.SignatureTypeRCD1:
		return FormatFA(hash)

	case protocol.SignatureTypeBTC,
		protocol.SignatureTypeBTCLegacy:
		return FormatBTC(hash)

	case protocol.SignatureTypeETH:
		return FormatETH(hash)

	case protocol.SignatureTypeEcdsaSha256:
		return FormatAC2(hash)

	case protocol.SignatureTypeRsaSha256:
		return FormatAC3(hash)

	case protocol.SignatureTypeReceipt,
		protocol.SignatureTypePartition,
		protocol.SignatureTypeSet,
		protocol.SignatureTypeRemote,
		protocol.SignatureTypeDelegated,
		protocol.SignatureTypeInternal:
		return "<invalid address>"

	default:
		return FormatMH(hash, multihash.IDENTITY)
	}
}
