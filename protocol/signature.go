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

	btc "github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcutil/base58"
	"github.com/ethereum/go-ethereum/crypto"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"golang.org/x/crypto/ripemd160" //nolint:staticcheck
)

var ErrCannotInitiate = errors.New(errors.StatusBadRequest, "signature cannot initiate a transaction: values are missing")

type Signature interface {
	encoding.UnionValue
	Type() SignatureType
	RoutingLocation() *url.URL

	GetVote() VoteType
	GetSigner() *url.URL
	GetTransactionHash() [32]byte

	Hash() []byte
	Metadata() Signature
	Initiator() (hash.Hasher, error)
}

type KeySignature interface {
	Signature
	GetSignature() []byte
	GetPublicKeyHash() []byte
	GetPublicKey() []byte
	GetSignerVersion() uint64
	GetTimestamp() uint64
	Verify(sigMdHash, hash []byte) bool
}

// IsSystem returns true if the signature type is a system signature type.
func (s SignatureType) IsSystem() bool {
	switch s {
	case SignatureTypePartition,
		SignatureTypeReceipt,
		SignatureTypeInternal:
		return true
	default:
		return false
	}
}

func PublicKeyHash(key []byte, typ SignatureType) ([]byte, error) {
	switch typ {
	case SignatureTypeED25519,
		SignatureTypeLegacyED25519:
		return doSha256(key), nil

	case SignatureTypeRCD1:
		return GetRCDHashFromPublicKey(key, 1), nil

	case SignatureTypeBTC,
		SignatureTypeBTCLegacy:
		return BTCHash(key), nil

	case SignatureTypeETH:
		return ETHhash(key), nil

	case SignatureTypeReceipt,
		SignatureTypePartition,
		SignatureTypeSet,
		SignatureTypeRemote,
		SignatureTypeDelegated,
		SignatureTypeInternal:
		return nil, errors.Format(errors.StatusBadRequest, "%v is not a key type", typ)

	default:
		return nil, errors.Format(errors.StatusNotAllowed, "unknown key type %v", typ)
	}
}

func signatureHash(sig Signature) []byte {
	// This should never fail unless the signature uses bigints
	data, _ := sig.MarshalBinary()
	return doSha256(data)
}

func doSha256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

// generates privatekey and compressed public key
func SECP256K1Keypair() (privKey []byte, pubKey []byte) {
	priv, _ := btc.NewPrivateKey(btc.S256())

	privKey = priv.Serialize()
	_, pub := btc.PrivKeyFromBytes(btc.S256(), privKey)
	pubKey = pub.SerializeCompressed()
	return privKey, pubKey
}

// generates privatekey and Un-compressed public key
func SECP256K1UncompressedKeypair() (privKey []byte, pubKey []byte) {
	priv, _ := btc.NewPrivateKey(btc.S256())

	privKey = priv.Serialize()
	_, pub := btc.PrivKeyFromBytes(btc.S256(), privKey)
	pubKey = pub.SerializeUncompressed()
	return privKey, pubKey
}

func BTCHash(pubKey []byte) []byte {
	hasher := ripemd160.New()
	hash := sha256.Sum256(pubKey[:])
	hasher.Write(hash[:])
	pubRip := hasher.Sum(nil)
	return pubRip[:]
}

func BTCaddress(pubKey []byte) string {
	pubRip := BTCHash(pubKey)
	versionedPayload := append([]byte{0x00}, pubRip...)
	newhash := sha256.Sum256(versionedPayload)
	newhash = sha256.Sum256(newhash[:])
	checkSum := newhash[:4]
	fullpayload := append(versionedPayload, checkSum...)
	address := base58.Encode(fullpayload)
	return address
}

// ETHhash returns the truncated hash (i.e. binary ethereum address)
func ETHhash(pubKey []byte) []byte {
	p, err := crypto.UnmarshalPubkey(pubKey)
	if err != nil {
		p, err = crypto.DecompressPubkey(pubKey)
		if err != nil {
			return nil
		}
	}
	a := crypto.PubkeyToAddress(*p)
	return a.Bytes()
}

func ETHaddress(pubKey []byte) (string, error) {
	h := ETHhash(pubKey)
	if h == nil {
		return "", fmt.Errorf("invalid eth public key")
	}
	return fmt.Sprintf("0x%x", h), nil
}

func SignatureDidInitiate(sig Signature, txnInitHash []byte, initiator *Signature) bool {
	for _, sig := range unpackSignature(sig) {
		if bytes.Equal(txnInitHash, sig.Metadata().Hash()) {
			if initiator != nil {
				*initiator = sig
			}
			return true
		}

		init, err := sig.Initiator()
		if err != nil {
			return false
		}
		if bytes.Equal(txnInitHash, init.MerkleHash()) {
			if initiator != nil {
				*initiator = sig
			}
			return true
		}
	}
	return false
}

func unpackSignature(sig Signature) []Signature {
	switch s := sig.(type) {
	case *SignatureSet:
		signatures := make([]Signature, 0, len(s.Signatures))
		for _, s := range s.Signatures {
			signatures = append(signatures, unpackSignature(s)...)
		}
		return signatures

	case *RemoteSignature:
		return unpackSignature(s.Signature)

	case *DelegatedSignature:
		// Unpack the inner signature
		signatures := unpackSignature(s.Signature)

		// Convert each unpacked signature back into a delegated signature
		for i, signature := range signatures {
			// Copy the delegated signature and set the inner signature to the
			// unpacked signature
			s := *s
			s.Signature = signature
			signatures[i] = &s
		}

		return signatures

	default:
		return []Signature{s}
	}
}

func CopyKeySignature(v KeySignature) KeySignature {
	return v.CopyAsInterface().(KeySignature)
}

func EqualKeySignature(a, b KeySignature) bool {
	return EqualSignature(a, b)
}

func UnmarshalKeySignature(data []byte) (KeySignature, error) {
	sig, err := UnmarshalSignature(data)
	if err != nil {
		return nil, err
	}

	keySig, ok := sig.(KeySignature)
	if !ok {
		return nil, fmt.Errorf("signature type %v is not a KeySignature", sig.Type())
	}

	return keySig, nil
}

func UnmarshalKeySignatureJSON(data []byte) (KeySignature, error) {
	sig, err := UnmarshalSignatureJSON(data)
	if err != nil {
		return nil, err
	}

	if sig == nil {
		return nil, nil
	}

	keySig, ok := sig.(KeySignature)
	if !ok {
		return nil, fmt.Errorf("signature type %v is not a KeySignature", sig.Type())
	}

	return keySig, nil
}

/*
 * Legacy ED25519 Signature
 */

func SignLegacyED25519(sig *LegacyED25519Signature, privateKey, sigMdHash, txnHash []byte) {
	if sigMdHash == nil {
		sigMdHash = sig.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, common.Uint64Bytes(sig.Timestamp)...)
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig.Signature = ed25519.Sign(privateKey, hash[:])
}

// GetSigner returns Signer.
func (s *LegacyED25519Signature) GetSigner() *url.URL { return s.Signer }

// RoutingLocation returns Signer.
func (s *LegacyED25519Signature) RoutingLocation() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *LegacyED25519Signature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *LegacyED25519Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns the hash of PublicKey.
func (s *LegacyED25519Signature) GetPublicKeyHash() []byte { return doSha256(s.PublicKey) }

// GetPublicKey returns PublicKey.
func (s *LegacyED25519Signature) GetPublicKey() []byte { return s.PublicKey }

// GetSignature returns Signature.
func (s *LegacyED25519Signature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *LegacyED25519Signature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *LegacyED25519Signature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *LegacyED25519Signature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // And the transaction hash
	return r
}

// Initiator returns a Hasher that calculates the Merkle hash of the signature.
func (s *LegacyED25519Signature) Initiator() (hash.Hasher, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher, nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *LegacyED25519Signature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid legacy ED25519 signature of
// the hash.
func (e *LegacyED25519Signature) Verify(sigMdHash, txnHash []byte) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	if sigMdHash == nil {
		sigMdHash = e.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, common.Uint64Bytes(e.Timestamp)...)
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	return ed25519.Verify(e.PublicKey, hash[:], e.Signature)
}

/*
 * ED25519 Signature
 */

func SignED25519(sig *ED25519Signature, privateKey, sigMdHash, txnHash []byte) {
	if sigMdHash == nil {
		sigMdHash = sig.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig.Signature = ed25519.Sign(privateKey, hash[:])
}

// GetSigner returns Signer.
func (s *ED25519Signature) GetSigner() *url.URL { return s.Signer }

// RoutingLocation returns Signer.
func (s *ED25519Signature) RoutingLocation() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *ED25519Signature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *ED25519Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns the hash of PublicKey.
func (s *ED25519Signature) GetPublicKeyHash() []byte { return doSha256(s.PublicKey) }

// GetPublicKey returns PublicKey.
func (s *ED25519Signature) GetPublicKey() []byte { return s.PublicKey }

// GetSignature returns Signature.
func (s *ED25519Signature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *ED25519Signature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *ED25519Signature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *ED25519Signature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // And the transaction hash
	return r
}

// Initiator returns a Hasher that calculates the Merkle hash of the signature.
func (s *ED25519Signature) Initiator() (hash.Hasher, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher, nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *ED25519Signature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid ED25519 signature of the
// hash.
func (e *ED25519Signature) Verify(sigMdHash, txnHash []byte) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	if sigMdHash == nil {
		sigMdHash = e.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	return ed25519.Verify(e.PublicKey, hash[:], e.Signature)
}

/*
 * RCD1 Signature
 */

func SignRCD1(sig *RCD1Signature, privateKey, sigMdHash, txnHash []byte) {
	if sigMdHash == nil {
		sigMdHash = sig.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig.Signature = ed25519.Sign(privateKey, hash[:])
}

// GetSigner returns Signer.
func (s *RCD1Signature) GetSigner() *url.URL { return s.Signer }

// RoutingLocation returns Signer.
func (s *RCD1Signature) RoutingLocation() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *RCD1Signature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *RCD1Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns RCD1 hash of PublicKey.
func (s *RCD1Signature) GetPublicKeyHash() []byte { return GetRCDHashFromPublicKey(s.PublicKey, 1) }

// GetPublicKey returns PublicKey.
func (s *RCD1Signature) GetPublicKey() []byte { return s.PublicKey }

// Verify returns true if this signature is a valid RCD1 signature of the hash.
func (e *RCD1Signature) Verify(sigMdHash, txnHash []byte) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	if sigMdHash == nil {
		sigMdHash = e.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	return ed25519.Verify(e.PublicKey, hash[:], e.Signature)
}

// GetSignature returns Signature.
func (s *RCD1Signature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *RCD1Signature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *RCD1Signature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *RCD1Signature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // And the transaction hash
	return r
}

// Initiator returns a Hasher that calculates the Merkle hash of the signature.
func (s *RCD1Signature) Initiator() (hash.Hasher, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher, nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *RCD1Signature) GetVote() VoteType {
	return s.Vote
}

/*
 * BTC Signature
 */

func SignBTC(sig *BTCSignature, privateKey, sigMdHash, txnHash []byte) error {
	if sigMdHash == nil {
		sigMdHash = sig.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	pvkey, pubKey := btc.PrivKeyFromBytes(btc.S256(), privateKey)
	sig.PublicKey = pubKey.SerializeCompressed()
	sign, err := pvkey.Sign(hash[:])
	if err != nil {
		return err
	}
	sig.Signature = sign.Serialize()
	return nil
}

// GetSigner returns Signer.
func (s *BTCSignature) GetSigner() *url.URL { return s.Signer }

// RoutingLocation returns Signer.
func (s *BTCSignature) RoutingLocation() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *BTCSignature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *BTCSignature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns the hash of PublicKey.
func (s *BTCSignature) GetPublicKeyHash() []byte { return BTCHash(s.PublicKey) }

// GetPublicKey returns PublicKey.
func (s *BTCSignature) GetPublicKey() []byte { return s.PublicKey }

// GetSignature returns Signature.
func (s *BTCSignature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *BTCSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *BTCSignature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *BTCSignature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // Clear the transaction hash
	return r
}

// Initiator returns a Hasher that calculates the Merkle hash of the signature.
func (s *BTCSignature) Initiator() (hash.Hasher, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher, nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *BTCSignature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid SECP256K1 signature of the
// hash.
func (e *BTCSignature) Verify(sigMdHash, txnHash []byte) bool {
	if sigMdHash == nil {
		sigMdHash = e.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig, err := btc.ParseSignature(e.Signature, btc.S256())
	if err != nil {
		return false
	}
	pbkey, err := btc.ParsePubKey(e.PublicKey, btc.S256())
	if err != nil {
		return false
	}
	return sig.Verify(hash[:], pbkey)
}

/*
 * BTCLegacy Signature
 */

func SignBTCLegacy(sig *BTCLegacySignature, privateKey, sigMdHash, txnHash []byte) error {
	if sigMdHash == nil {
		sigMdHash = sig.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	pvkey, pubKey := btc.PrivKeyFromBytes(btc.S256(), privateKey)
	sig.PublicKey = pubKey.SerializeUncompressed()
	sign, err := pvkey.Sign(hash[:])
	if err != nil {
		return err
	}
	sig.Signature = sign.Serialize()
	return nil
}

// GetSigner returns Signer.
func (s *BTCLegacySignature) GetSigner() *url.URL { return s.Signer }

// RoutingLocation returns Signer.
func (s *BTCLegacySignature) RoutingLocation() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *BTCLegacySignature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *BTCLegacySignature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns the hash of PublicKey.
func (s *BTCLegacySignature) GetPublicKeyHash() []byte { return BTCHash(s.PublicKey) }

// GetPublicKey returns PublicKey.
func (s *BTCLegacySignature) GetPublicKey() []byte { return s.PublicKey }

// GetSignature returns Signature.
func (s *BTCLegacySignature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *BTCLegacySignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *BTCLegacySignature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *BTCLegacySignature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // Clear the transaction hash
	return r
}

// Initiator returns a Hasher that calculates the Merkle hash of the signature.
func (s *BTCLegacySignature) Initiator() (hash.Hasher, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher, nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *BTCLegacySignature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid SECP256K1 signature of the
// hash.
func (e *BTCLegacySignature) Verify(sigMdHash, txnHash []byte) bool {
	if sigMdHash == nil {
		sigMdHash = e.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig, err := btc.ParseSignature(e.Signature, btc.S256())
	if err != nil {
		return false
	}
	pbkey, err := btc.ParsePubKey(e.PublicKey, btc.S256())
	if err != nil {
		return false
	}
	return sig.Verify(hash[:], pbkey)
}

/*
 * ETH Signature
 */

func SignETH(sig *ETHSignature, privateKey, sigMdHash, txnHash []byte) error {
	if sigMdHash == nil {
		sigMdHash = sig.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	pvkey, pubKey := btc.PrivKeyFromBytes(btc.S256(), privateKey)
	sig.PublicKey = pubKey.SerializeUncompressed()
	sign, err := pvkey.Sign(hash[:])
	if err != nil {
		return err
	}
	sig.Signature = sign.Serialize()
	return nil
}

// GetSigner returns Signer.
func (s *ETHSignature) GetSigner() *url.URL { return s.Signer }

// RoutingLocation returns Signer.
func (s *ETHSignature) RoutingLocation() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *ETHSignature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *ETHSignature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns the hash of PublicKey.
func (s *ETHSignature) GetPublicKeyHash() []byte { return ETHhash(s.PublicKey) }

// GetPublicKey returns PublicKey.
func (s *ETHSignature) GetPublicKey() []byte { return s.PublicKey }

// GetSignature returns Signature.
func (s *ETHSignature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *ETHSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *ETHSignature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *ETHSignature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // Clear the transaction hash
	return r
}

// Initiator returns a Hasher that calculates the Merkle hash of the signature.
func (s *ETHSignature) Initiator() (hash.Hasher, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher, nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *ETHSignature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid SECP256K1 signature of the
// hash.
func (e *ETHSignature) Verify(sigMdHash, txnHash []byte) bool {
	if sigMdHash == nil {
		sigMdHash = e.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig, err := btc.ParseSignature(e.Signature, btc.S256())
	if err != nil {
		return false
	}
	pbkey, err := btc.ParsePubKey(e.PublicKey, btc.S256())
	if err != nil {
		return false
	}
	return sig.Verify(hash[:], pbkey)
}

/*
 * Receipt Signature
 */

// GetSigner panics.
func (s *ReceiptSignature) GetSigner() *url.URL {
	panic("a receipt signature does not have a signer")
}

// RoutingLocation panics.
func (s *ReceiptSignature) RoutingLocation() *url.URL {
	panic("a receipt signature does not have a routing location")
}

// GetSignerVersion returns 1.
func (s *ReceiptSignature) GetSignerVersion() uint64 { return 1 }

// GetTimestamp returns 1.
func (s *ReceiptSignature) GetTimestamp() uint64 { return 1 }

// GetPublicKey returns nil.
func (s *ReceiptSignature) GetPublicKeyHash() []byte { return nil }

// GetSignature returns the marshalled receipt.
func (s *ReceiptSignature) GetSignature() []byte {
	b, _ := s.Proof.MarshalBinary()
	return b
}

// GetTransactionHash returns TransactionHash.
func (s *ReceiptSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *ReceiptSignature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *ReceiptSignature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.TransactionHash = [32]byte{} // Clear the transaction hash
	return r
}

// InitiatorHash returns an error.
func (s *ReceiptSignature) Initiator() (hash.Hasher, error) {
	return nil, fmt.Errorf("a receipt signature cannot initiate a transaction")
}

// GetVote returns how the signer votes on a particular transaction
func (s *ReceiptSignature) GetVote() VoteType {
	return VoteTypeAccept
}

// Verify returns true if this receipt is a valid receipt of the hash.
func (s *ReceiptSignature) Verify(hash []byte) bool {
	return bytes.Equal(s.Proof.Start, hash) && s.Proof.Validate()
}

/*
 * Synthetic Signature
 */

// GetSigner panics.
func (s *PartitionSignature) GetSigner() *url.URL {
	panic("a synthetic signature does not have a signer")
}

// RoutingLocation returns the URL of the destination network's identity.
func (s *PartitionSignature) RoutingLocation() *url.URL { return s.DestinationNetwork }

// GetSignerVersion returns 1.
func (s *PartitionSignature) GetSignerVersion() uint64 { return 1 }

// GetTimestamp returns 1.
func (s *PartitionSignature) GetTimestamp() uint64 { return 1 }

// GetPublicKey returns nil.
func (s *PartitionSignature) GetPublicKeyHash() []byte { return nil }

// GetSignature returns nil.
func (s *PartitionSignature) GetSignature() []byte { return nil }

// GetTransactionHash returns TransactionHash.
func (s *PartitionSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *PartitionSignature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *PartitionSignature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.TransactionHash = [32]byte{} // Clear the transaction hash
	return r
}

// Initiator returns an error.
func (s *PartitionSignature) Initiator() (hash.Hasher, error) {
	return nil, errors.New(errors.StatusBadRequest, "use of the initiator hash for a synthetic signature is not supported")
}

// GetVote returns VoteTypeAccept.
func (s *PartitionSignature) GetVote() VoteType {
	return VoteTypeAccept
}

/*
 * Signature Set
 */

// GetVote returns Vote.
func (s *SignatureSet) GetVote() VoteType { return s.Vote }

// GetSigner returns Signer.
func (s *SignatureSet) GetSigner() *url.URL { return s.Signer }

// GetTransactionHash returns TransactionHash.
func (s *SignatureSet) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *SignatureSet) Hash() []byte { return signatureHash(s) }

// RoutingLocation returns Signer.
func (s *SignatureSet) RoutingLocation() *url.URL { return s.Signer }

// Initiator returns an error.
func (s *SignatureSet) Initiator() (hash.Hasher, error) {
	return nil, fmt.Errorf("cannot initate a transaction")
}

// Metadata panics.
func (s *SignatureSet) Metadata() Signature { panic("not supported") }

/*
 * Remote Signature
 */

func (s *RemoteSignature) GetVote() VoteType            { return s.Signature.GetVote() }
func (s *RemoteSignature) GetSigner() *url.URL          { return s.Signature.GetSigner() }
func (s *RemoteSignature) GetTransactionHash() [32]byte { return s.Signature.GetTransactionHash() }

// Hash returns the hash of the signature.
func (s *RemoteSignature) Hash() []byte { return signatureHash(s) }

// RoutingLocation returns Destination.
func (s *RemoteSignature) RoutingLocation() *url.URL { return s.Destination }

// Initiator returns an error.
func (s *RemoteSignature) Initiator() (hash.Hasher, error) {
	return nil, fmt.Errorf("cannot initate a transaction")
}

// Metadata panics.
func (s *RemoteSignature) Metadata() Signature { panic("not supported") }

/*
 * Delegated Signature
 */

func (s *DelegatedSignature) GetVote() VoteType            { return s.Signature.GetVote() }
func (s *DelegatedSignature) RoutingLocation() *url.URL    { return s.Signature.RoutingLocation() }
func (s *DelegatedSignature) GetSigner() *url.URL          { return s.Signature.GetSigner() }
func (s *DelegatedSignature) GetTransactionHash() [32]byte { return s.Signature.GetTransactionHash() }

func (s *DelegatedSignature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *DelegatedSignature) Metadata() Signature {
	r := s.Copy()
	r.Signature = r.Signature.Metadata()
	return r
}

// Initiator returns a Hasher that calculates the Merkle hash of the signature.
func (s *DelegatedSignature) Initiator() (hash.Hasher, error) {
	if s.Delegator == nil {
		return nil, ErrCannotInitiate
	}

	hasher, err := s.Signature.Initiator()
	if err != nil {
		return nil, err
	}

	hasher.AddUrl(s.Delegator)
	return hasher, nil
}

func (s *DelegatedSignature) Verify(sigMdHash, hash []byte) bool {
	switch sig := s.Signature.(type) {
	case KeySignature:
		return sig.Verify(sigMdHash, hash)
	case *DelegatedSignature:
		return sig.Verify(sigMdHash, hash)
	}
	return false
}

/*
 * Internal Signature
 */

// GetSigner panics.
func (s *InternalSignature) GetSigner() *url.URL {
	panic("a receipt signature does not have a signer")
}

// RoutingLocation panics.
func (s *InternalSignature) RoutingLocation() *url.URL {
	panic("a receipt signature does not have a routing location")
}

// GetTransactionHash returns TransactionHash.
func (s *InternalSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *InternalSignature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *InternalSignature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.TransactionHash = [32]byte{} // Clear the transaction hash
	return r
}

// InitiatorHash returns an error.
func (s *InternalSignature) Initiator() (hash.Hasher, error) {
	return nil, errors.New(errors.StatusBadRequest, "use of the initiator hash for an internal signature is not supported")
}

// GetVote returns VoteTypeAccept.
func (s *InternalSignature) GetVote() VoteType {
	return VoteTypeAccept
}
