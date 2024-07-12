// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"io"

	btc "github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcutil/base58"
	eth "github.com/ethereum/go-ethereum/crypto"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"golang.org/x/crypto/ripemd160" //nolint:staticcheck
)

var ErrCannotInitiate = errors.BadRequest.With("signature cannot initiate a transaction: values are missing")

type ethSignatureV1 struct {
	*ETHSignature
}

// VerifyUserSignatureV1 verifies that the user signature signs the given message using version 1 logic
func VerifyUserSignatureV1(sig UserSignature, msg Signable) bool {
	if sig.Type() == SignatureTypeETH {
		//recast ETHSignature type to use V1 signature format
		sig = &ethSignatureV1{sig.(*ETHSignature)}
	}

	return VerifyUserSignature(sig, msg)
}

// VerifyUserSignature verifies that the user signature signs the given message.
func VerifyUserSignature(sig UserSignature, msg Signable) bool {
	return sig.Verify(sig, msg)
}

type Signature interface {
	encoding.UnionValue
	Type() SignatureType
	RoutingLocation() *url.URL

	GetVote() VoteType
	GetSigner() *url.URL
	GetTransactionHash() [32]byte

	Hash() []byte
	Metadata() Signature
}

type Signable interface{ Hash() [32]byte }

// UserSignature is a type of signature that can initiate transactions with a
// special initiator hash. This type of initiator hash has been deprecated.
type UserSignature interface {
	Signature
	Initiator() (hash.Hasher, error)
	Verify(Signature, Signable) bool
}

type KeySignature interface {
	UserSignature
	GetSignature() []byte
	GetPublicKeyHash() []byte
	GetPublicKey() []byte
	GetSignerVersion() uint64
	GetTimestamp() uint64
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
		SignatureTypeLegacyED25519,
		SignatureTypeRsaSha256,
		SignatureTypeEcdsaSha256:
		return doSha256(key), nil

	case SignatureTypeRCD1:
		return GetRCDHashFromPublicKey(key, 1), nil

	case SignatureTypeBTC,
		SignatureTypeBTCLegacy:
		return BTCHash(key), nil

	case SignatureTypeETH,
		SignatureTypeTypedData:
		return ETHhash(key), nil

	case SignatureTypeReceipt,
		SignatureTypePartition,
		SignatureTypeSet,
		SignatureTypeRemote,
		SignatureTypeDelegated,
		SignatureTypeInternal:
		return nil, errors.BadRequest.WithFormat("%v is not a key type", typ)

	default:
		return nil, errors.NotAllowed.WithFormat("unknown key type %v", typ)
	}
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
	p, err := eth.UnmarshalPubkey(pubKey)
	if err != nil {
		p, err = eth.DecompressPubkey(pubKey)
		if err != nil {
			return nil
		}
	}
	a := eth.PubkeyToAddress(*p)
	return a.Bytes()
}

func ETHaddress(pubKey []byte) (string, error) {
	h := ETHhash(pubKey)
	if h == nil {
		return "", fmt.Errorf("invalid eth public key")
	}
	return fmt.Sprintf("0x%x", h), nil
}

func SignatureDidInitiate(sig Signature, txnInitHash []byte, initiator *Signature) (ok, merkle bool) {
	for _, sig := range unpackSignature(sig) {
		if bytes.Equal(txnInitHash, sig.Metadata().Hash()) {
			if initiator != nil {
				*initiator = sig
			}
			return true, false
		}

		init, ok := sig.(UserSignature)
		if !ok {
			continue
		}

		hash, err := init.Initiator()
		if err != nil {
			return false, false
		}
		if bytes.Equal(txnInitHash, hash.MerkleHash()) {
			if initiator != nil {
				*initiator = sig
			}
			return true, true
		}
	}
	return false, false
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
func (e *LegacyED25519Signature) Verify(sig Signature, msg Signable) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	return verifySigSplit(e, sig, true, msg, func(sig, msg []byte) bool {
		hash := doSha256(sig, common.Uint64Bytes(e.Timestamp), msg)
		return ed25519.Verify(e.PublicKey, hash, e.Signature)
	})
}

/*
 * ED25519 Signature
 */

func SignED25519(sig *ED25519Signature, privateKey, sigMdHash, txnHash []byte) {
	sig.Signature = ed25519.Sign(privateKey, signingHash(sig, doSha256, sigMdHash, txnHash))
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
func (e *ED25519Signature) Verify(sig Signature, msg Signable) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	return verifySig(e, sig, true, msg, func(msg []byte) bool {
		return ed25519.Verify(e.PublicKey, msg, e.Signature)
	})
}

/*
 * RCD1 Signature
 */

func SignRCD1(sig *RCD1Signature, privateKey, sigMdHash, txnHash []byte) {
	sig.Signature = ed25519.Sign(privateKey, signingHash(sig, doSha256, sigMdHash, txnHash))
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
func (e *RCD1Signature) Verify(sig Signature, msg Signable) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	return verifySig(e, sig, true, msg, func(msg []byte) bool {
		return ed25519.Verify(e.PublicKey, msg, e.Signature)
	})
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
	pvkey, _ := btc.PrivKeyFromBytes(btc.S256(), privateKey)
	sign, err := pvkey.Sign(signingHash(sig, doSha256, sigMdHash, txnHash))
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
func (e *BTCSignature) Verify(sig Signature, msg Signable) bool {
	bsig, err := btc.ParseSignature(e.Signature, btc.S256())
	if err != nil {
		return false
	}
	pbkey, err := btc.ParsePubKey(e.PublicKey, btc.S256())
	if err != nil {
		return false
	}
	return verifySig(e, sig, true, msg, func(msg []byte) bool {
		return bsig.Verify(msg, pbkey)
	})
}

/*
 * BTCLegacy Signature
 */

func SignBTCLegacy(sig *BTCLegacySignature, privateKey, sigMdHash, txnHash []byte) error {
	pvkey, _ := btc.PrivKeyFromBytes(btc.S256(), privateKey)
	sign, err := pvkey.Sign(signingHash(sig, doSha256, sigMdHash, txnHash))
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
func (e *BTCLegacySignature) Verify(sig Signature, msg Signable) bool {
	bsig, err := btc.ParseSignature(e.Signature, btc.S256())
	if err != nil {
		return false
	}
	pbkey, err := btc.ParsePubKey(e.PublicKey, btc.S256())
	if err != nil {
		return false
	}
	return verifySig(e, sig, true, msg, func(msg []byte) bool {
		return bsig.Verify(msg, pbkey)
	})
}

/*
 * ETH Signature in Distinguished Encoding Rules (DER) format
 */

func SignEthAsDer(sig *ETHSignature, privateKey, sigMdHash, txnHash []byte) (err error) {
	pvkey, pub := btc.PrivKeyFromBytes(btc.S256(), privateKey)
	pk2, err := btc.ParsePubKey(sig.PublicKey, btc.S256())
	if err != nil {
		return err
	}
	if !pub.IsEqual(pk2) {
		return fmt.Errorf("public key is not what is expected")
	}
	sign, err := pvkey.Sign(signingHash(sig, doSha256, sigMdHash, txnHash))
	if err != nil {
		return err
	}
	sig.Signature = sign.Serialize()
	return nil
}

/*
 * ETH Signature
 */

func SignETH(sig *ETHSignature, privateKey, sigMdHash, txnHash []byte) (err error) {
	priv, err := eth.ToECDSA(privateKey)
	if err != nil {
		return err
	}

	sig.PublicKey = eth.FromECDSAPub(&priv.PublicKey)
	sig.Signature, err = eth.Sign(signingHash(sig, doSha256, sigMdHash, txnHash), priv)
	return err
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

// Deprecated: Verify returns true if this signature is a valid signature in DER format of the hash.
func (e *ethSignatureV1) Verify(sig Signature, msg Signable) bool {
	//process signature as DER format
	bsig, err := btc.ParseSignature(e.Signature, btc.S256())
	if err != nil {
		return false
	}
	pbkey, err := btc.ParsePubKey(e.PublicKey, btc.S256())
	if err != nil {
		return false
	}
	return verifySig(e, sig, true, msg, func(msg []byte) bool {
		return bsig.Verify(msg, pbkey)
	})
}

// Verify returns true if this signature is a valid RSV signature of the hash, with V2 we drop support for DER format
func (e *ETHSignature) Verify(sig Signature, msg Signable) bool {
	s := e.Signature
	if len(s) == 65 {
		//extract RS of the RSV format
		s = s[:64]
	}
	return verifySig(e, sig, true, msg, func(msg []byte) bool {
		return eth.VerifySignature(e.PublicKey, msg, s)
	})
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

// GetVote returns how the signer votes on a particular transaction
func (s *ReceiptSignature) GetVote() VoteType {
	return VoteTypeAccept
}

// Verify returns true if this receipt is a valid receipt of the hash.
func (s *ReceiptSignature) Verify(hash []byte) bool {
	return bytes.Equal(s.Proof.Start, hash) && s.Proof.Validate(nil)
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

	init, ok := s.Signature.(UserSignature)
	if !ok {
		return nil, ErrCannotInitiate
	}

	hasher, err := init.Initiator()
	if err != nil {
		return nil, err
	}

	hasher.AddUrl(s.Delegator)
	return hasher, nil
}

func (s *DelegatedSignature) Verify(sig Signature, msg Signable) bool {
	us, ok := s.Signature.(UserSignature)
	if !ok {
		return false
	}
	if sig == nil {
		sig = s
	}
	return us.Verify(sig, msg)
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

// GetVote returns VoteTypeAccept.
func (s *InternalSignature) GetVote() VoteType {
	return VoteTypeAccept
}

func UnmarshalKeySignatureFrom(rd io.Reader) (KeySignature, error) {
	s, err := UnmarshalSignatureFrom(rd)
	if err != nil {
		return nil, err
	}
	ks, ok := s.(KeySignature)
	if !ok {
		return nil, errors.Conflict.WithFormat("%v is not a key signature", s.Type())
	}
	return ks, nil
}

/*
 * Authority Signature
 */

func (s *AuthoritySignature) GetVote() VoteType            { return s.Vote }
func (s *AuthoritySignature) GetSigner() *url.URL          { return s.RoutingLocation() }
func (s *AuthoritySignature) GetTransactionHash() [32]byte { return s.TxID.Hash() }
func (s *AuthoritySignature) Hash() []byte                 { return signatureHash(s) }
func (s *AuthoritySignature) Metadata() Signature          { return s }

func (s *AuthoritySignature) RoutingLocation() *url.URL {
	if len(s.Delegator) > 0 {
		return s.Delegator[0]
	}
	return s.TxID.Account()
}

/*
 * RSA Signature
 * privateKey must be in PKCS #1, ASN.1 DER format
 */
func SignRsaSha256(sig *RsaSha256Signature, privateKeyDer, sigMdHash, txnHash []byte) error {
	//private key is expected to be in PKCS #1, ASN.1 DER format
	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyDer)
	if err != nil {
		return err
	}

	// Sign the signing hash
	sig.Signature, err = rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, signingHash(sig, doSha256, sigMdHash, txnHash))
	if err != nil {
		return err
	}
	return nil
}

// GetSigner returns Signer.
func (s *RsaSha256Signature) GetSigner() *url.URL { return s.Signer }

// RoutingLocation returns Signer.
func (s *RsaSha256Signature) RoutingLocation() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *RsaSha256Signature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *RsaSha256Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns the hash of PublicKey.
func (s *RsaSha256Signature) GetPublicKeyHash() []byte { return doSha256(s.PublicKey) }

// GetPublicKey returns PublicKey.
func (s *RsaSha256Signature) GetPublicKey() []byte { return s.PublicKey }

// GetSignature returns Signature.
func (s *RsaSha256Signature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *RsaSha256Signature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *RsaSha256Signature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *RsaSha256Signature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // And the transaction hash
	return r
}

// Initiator returns [ErrCannotInitiate]. [RsaSha256Signature] only supports simple hashes.
func (s *RsaSha256Signature) Initiator() (hash.Hasher, error) {
	return nil, ErrCannotInitiate
}

// GetVote returns how the signer votes on a particular transaction
func (s *RsaSha256Signature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid RSA signature of the
// hash. The public key is expected to be in PKCS#1 ASN.1 DER format
func (e *RsaSha256Signature) Verify(sig Signature, msg Signable) bool {
	//Convert public DER key into and rsa public key struct
	pubKey, err := x509.ParsePKCS1PublicKey(e.PublicKey)
	if err != nil {
		return false
	}

	//The length of the signature should be the size of the public key's modulus
	if pubKey.Size() != len(e.Signature) {
		return false
	}

	// Verify signature
	return verifySig(e, sig, false, msg, func(b []byte) bool {
		err = rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, b, e.Signature)
		return err == nil
	})
}

/*
 * SignEcdsaSha256: Multipurpose ECDSA signature types used in PKIs
 * ecdsa privateKey must be in SEC, ASN.1 DER format,
 * returned signature is in the common encoding format for that particular signature type
 */
func SignEcdsaSha256(sig *EcdsaSha256Signature, privateKeyDer, sigMdHash, txnHash []byte) error {
	//private key is expected to be in SEC, ASN.1 DER format
	privateKey, err := x509.ParseECPrivateKey(privateKeyDer)
	if err != nil {
		return err
	}

	// Sign the signing hash and set ASN.1 DER encoded signature
	sig.Signature, err = ecdsa.SignASN1(rand.Reader, privateKey, signingHash(sig, doSha256, sigMdHash, txnHash))
	if err != nil {
		return err
	}
	return nil
}

// GetSigner returns Signer.
func (s *EcdsaSha256Signature) GetSigner() *url.URL { return s.Signer }

// RoutingLocation returns Signer.
func (s *EcdsaSha256Signature) RoutingLocation() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *EcdsaSha256Signature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *EcdsaSha256Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns the hash of PublicKey.
func (s *EcdsaSha256Signature) GetPublicKeyHash() []byte { return doSha256(s.PublicKey) }

// GetPublicKey returns PublicKey.
func (s *EcdsaSha256Signature) GetPublicKey() []byte { return s.PublicKey }

// GetSignature returns Signature.
func (s *EcdsaSha256Signature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *EcdsaSha256Signature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *EcdsaSha256Signature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *EcdsaSha256Signature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // And the transaction hash
	return r
}

// Initiator returns [ErrCannotInitiate]. [EcdsaSha256Signature] only supports simple hashes.
func (s *EcdsaSha256Signature) Initiator() (hash.Hasher, error) {
	return nil, ErrCannotInitiate
}

// GetVote returns how the signer votes on a particular transaction
func (s *EcdsaSha256Signature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid ECDSA ANS.1 encoded signature of the
// hash. The public key is expected to be in PKCS#1 ASN.1 DER format
func (e *EcdsaSha256Signature) Verify(sig Signature, msg Signable) bool {
	//Convert public ANS.1 encoded key into and associated public key struct
	pubKey, err := x509.ParsePKIXPublicKey(e.PublicKey)
	if err != nil {
		return false
	}

	pub, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return false
	}

	return verifySig(e, sig, false, msg, func(msg []byte) bool {
		return ecdsa.VerifyASN1(pub, msg, e.Signature)
	})
}

/*
 * EIP-712 Typed Data Signature
 * privateKey must be ecdsa
 */
func SignEip712TypedData(sig *TypedDataSignature, privateKey []byte, outer Signature, txn *Transaction) error {
	if outer == nil {
		outer = sig
	}
	hash, err := EIP712Hash(txn, outer)
	if err != nil {
		return err
	}

	priv, err := eth.ToECDSA(privateKey)
	if err != nil {
		return err
	}

	sig.TransactionHash = txn.Hash()
	sig.Signature, err = eth.Sign(hash, priv)
	return err
}

// GetSigner returns Signer.
func (s *TypedDataSignature) GetSigner() *url.URL { return s.Signer }

// RoutingLocation returns Signer.
func (s *TypedDataSignature) RoutingLocation() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *TypedDataSignature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *TypedDataSignature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns the hash of PublicKey.
func (s *TypedDataSignature) GetPublicKeyHash() []byte { return ETHhash(s.PublicKey) }

// GetPublicKey returns PublicKey.
func (s *TypedDataSignature) GetPublicKey() []byte { return s.PublicKey }

// GetSignature returns Signature.
func (s *TypedDataSignature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *TypedDataSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *TypedDataSignature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata.
func (s *TypedDataSignature) Metadata() Signature {
	r := s.Copy()                  // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // And the transaction hash
	return r
}

// Initiator returns a Hasher that calculates the Merkle hash of the signature.
func (s *TypedDataSignature) Initiator() (hash.Hasher, error) {
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
func (s *TypedDataSignature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid EIP-712 signature following
// the spec.
func (e *TypedDataSignature) Verify(sig Signature, msg Signable) bool {
	txn, ok := msg.(*Transaction)
	if !ok {
		// EIP-712 cannot be used to sign something that isn't a transaction
		return false
	}

	if sig == nil {
		sig = e
	}
	typedDataTxnHash, err := EIP712Hash(txn, sig)
	if err != nil {
		return false
	}

	s := e.Signature
	if len(s) == 65 {
		//extract RS of the RSV format
		s = s[:64]
	}
	return eth.VerifySignature(e.PublicKey, typedDataTxnHash, s)
}
