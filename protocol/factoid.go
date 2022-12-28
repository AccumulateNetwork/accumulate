// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/btcsuite/btcutil/base58"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type RCD struct {
	Version byte
	PubKey  [32]byte
}

func (r *RCD) Hash() []byte {
	var buffer bytes.Buffer
	buffer.WriteByte(r.Version)
	buffer.Write(r.PubKey[:])
	h := sha256.Sum256(buffer.Bytes())
	h = sha256.Sum256(h[:])
	return h[:]
}

func GetRCDHashFromPublicKey(pubKey []byte, version byte) []byte {
	r := RCD{}
	r.Version = version
	copy(r.PubKey[:], pubKey)
	return r.Hash()
}

func GetRCDFromFactoidAddress(fa string) ([]byte, error) {

	if len(fa) != 52 {
		return nil, fmt.Errorf("invalid factoid address length")
	}

	if !strings.HasPrefix(fa, "FA") {
		return nil, fmt.Errorf("invalid factoid address prefix, expecting FA but received %s", fa[:2])
	}

	faPrefix := []byte{0x5f, 0xb1}
	decodedFA := base58.Decode(fa)

	if !bytes.HasPrefix(decodedFA[:2], faPrefix) {
		return nil, fmt.Errorf("invalid factoid base58 encoding prefix")
	}

	checksum := sha256.Sum256(decodedFA[:34])
	checksum = sha256.Sum256(checksum[:])
	if !bytes.HasSuffix(decodedFA[34:], checksum[:4]) {
		return nil, fmt.Errorf("invalid checksum on factoid address")
	}

	rcdHash := decodedFA[2:34]
	return rcdHash, nil
}

func GetLiteAccountFromFactoidAddress(fa string) (*url.URL, error) {
	rcdHash, err := GetRCDFromFactoidAddress(fa)
	if err != nil {
		return nil, err
	}
	return LiteTokenAddressFromHash(rcdHash, ACME)
}

func GetFactoidAddressFromRCDHash(rcd []byte) (string, error) {
	if len(rcd) != 32 {
		return "", fmt.Errorf("invalid RCH Hash length must be 32 bytes")
	}

	hash := make([]byte, 34)
	faPrefix := []byte{0x5f, 0xb1}
	copy(hash[:2], faPrefix)
	copy(hash[2:], rcd[:])
	checkSum := sha256.Sum256(hash[:])
	checkSum = sha256.Sum256(checkSum[:])
	fa := make([]byte, 38)
	copy(fa[:34], hash)
	copy(fa[34:], checkSum[:4])
	FA := base58.Encode(fa)
	if !strings.HasPrefix(FA, "FA") {
		return "", fmt.Errorf("invalid factoid address prefix, expecting FA but received %s", fa[:2])
	}

	return FA, nil
}

// this function takes in the Factoid Private Key Fs and returns Factoid Address FA, RCDHash ,privatekey(64bits),error
func GetFactoidAddressRcdHashPkeyFromPrivateFs(Fs string) (string, []byte, []byte, error) {
	if len(Fs) != 52 {
		return "", nil, nil, fmt.Errorf("invalid factoid address length")
	}

	if !strings.HasPrefix(Fs, "Fs") {
		return "", nil, nil, fmt.Errorf("invalid factoid address prefix, expecting FA but received %s", Fs[:2])
	}

	fsprefix := []byte{0x64, 0x78}
	decodedFs := base58.Decode(Fs)

	if !bytes.HasPrefix(decodedFs[:2], fsprefix) {
		return "", nil, nil, fmt.Errorf("invalid factoid base58 encoding prefix")
	}

	checksum := sha256.Sum256(decodedFs[:34])
	checksum = sha256.Sum256(checksum[:])
	if !bytes.HasSuffix(decodedFs[34:], checksum[:4]) {
		return "", nil, nil, fmt.Errorf("invalid checksum on factoid address")
	}

	seed := decodedFs[2:34]
	privKey := ed25519.NewKeyFromSeed(seed)
	rcdHash := GetRCDHashFromPublicKey(privKey[32:], 0x01)
	FA, err := GetFactoidAddressFromRCDHash(rcdHash)
	if err != nil {
		return "", nil, nil, err
	}
	return FA, rcdHash, privKey, nil
}

func GetFactoidSecretFromPrivKey(pk []byte) (string, error) {
	if len(pk) != 64 {
		return "", fmt.Errorf("invalid private key must be 64 bytes long")
	}
	faprefix := []byte{0x64, 0x78}

	hash := make([]byte, 34)
	copy(hash[:2], faprefix)
	copy(hash[2:], pk[:32])
	checksum := sha256.Sum256(hash[:])
	checksum = sha256.Sum256(checksum[:])
	fsraw := make([]byte, 38)
	copy(fsraw[:34], hash[:])
	copy(fsraw[34:], checksum[:])
	Fs := base58.Encode(fsraw)
	return Fs, nil
}
