package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"

	btc "github.com/btcsuite/btcd/btcec"
	"golang.org/x/crypto/ripemd160"
	"golang.org/x/crypto/sha3"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

var ErrCannotInitiate = errors.New("signature cannot initiate a transaction: values are missing")

type KeySignature interface {
	Signature
	GetPublicKey() []byte
}

// IsSystem returns true if the signature type is a system signature type.
func (s SignatureType) IsSystem() bool {
	switch s {
	case SignatureTypeSynthetic,
		SignatureTypeReceipt,
		SignatureTypeInternal:
		return true
	default:
		return false
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

func SECP256K1Keypair() (privKey []byte, pubKey []byte) {
	priv, _ := btc.NewPrivateKey(btc.S256())

	privKey = priv.Serialize()
	_, pub := btc.PrivKeyFromBytes(btc.S256(), privKey)
	pubKey = pub.SerializeUncompressed()
	return privKey, pubKey
}

func BTCaddress(pubKey []byte) []byte {
	hasher := ripemd160.New()
	hash := sha256.Sum256(pubKey[1:])
	hasher.Write(hash[:])
	pubRip := hasher.Sum(nil)
	versionedPayload := append([]byte{0x00}, pubRip...)
	newhash := sha256.Sum256(versionedPayload)
	newhash = sha256.Sum256(newhash[:])
	checkSum := newhash[:4]
	fullpayload := append(versionedPayload, checkSum...)
	address := Base58Encode(fullpayload)
	return address
}

func ETHaddress(pubKey []byte) []byte {
	hash := sha3.New256()
	hash.Write(pubKey[1:])
	address := hash.Sum(nil)
	return address[12:]
}

func netVal(u *url.URL) *url.URL {
	return FormatKeyPageUrl(u.JoinPath(ValidatorBook), 0)
}

func SignatureDidInitiate(sig Signature, txnInitHash []byte) bool {
	sigInitHash, _ := sig.InitiatorHash()
	sigMetaHash := sig.MetadataHash()
	return bytes.Equal(txnInitHash, sigInitHash) || bytes.Equal(txnInitHash, sigMetaHash)
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

	keySig, ok := sig.(KeySignature)
	if !ok {
		return nil, fmt.Errorf("signature type %v is not a KeySignature", sig.Type())
	}

	return keySig, nil
}

/*
 * Legacy ED25519 Signature
 */

func SignLegacyED25519(sig *LegacyED25519Signature, privateKey, txnHash []byte) {
	data := sig.MetadataHash()
	data = append(data, common.Uint64Bytes(sig.Timestamp)...)
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig.Signature = ed25519.Sign(privateKey, hash[:])
}

// GetSigner returns Signer.
func (s *LegacyED25519Signature) GetSigner() *url.URL { return s.Signer }

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

// MetadataHash hashes the signature metadata.
func (s *LegacyED25519Signature) MetadataHash() []byte {
	r := *s                        // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // And the transaction hash
	return r.Hash()                // Hash it
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *LegacyED25519Signature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *LegacyED25519Signature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid legacy ED25519 signature of
// the hash.
func (e *LegacyED25519Signature) Verify(txnHash []byte) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	data := e.MetadataHash()
	data = append(data, common.Uint64Bytes(e.Timestamp)...)
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	return ed25519.Verify(e.PublicKey, hash[:], e.Signature)
}

/*
 * ED25519 Signature
 */

func SignED25519(sig *ED25519Signature, privateKey, txnHash []byte) {
	data := sig.MetadataHash()
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig.Signature = ed25519.Sign(privateKey, hash[:])
}

// GetSigner returns Signer.
func (s *ED25519Signature) GetSigner() *url.URL { return s.Signer }

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

// MetadataHash hashes the signature metadata.
func (s *ED25519Signature) MetadataHash() []byte {
	r := *s                        // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // And the transaction hash
	return r.Hash()                // Hash it
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *ED25519Signature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *ED25519Signature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid ED25519 signature of the
// hash.
func (e *ED25519Signature) Verify(txnHash []byte) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	data := e.MetadataHash()
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	return ed25519.Verify(e.PublicKey, hash[:], e.Signature)
}

/*
 * RCD1 Signature
 */

func SignRCD1(sig *RCD1Signature, privateKey, txnHash []byte) {
	data := sig.MetadataHash()
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig.Signature = ed25519.Sign(privateKey, hash[:])
}

// GetSigner returns Signer.
func (s *RCD1Signature) GetSigner() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *RCD1Signature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *RCD1Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns RCD1 hash of PublicKey.
func (s *RCD1Signature) GetPublicKeyHash() []byte { return GetRCDHashFromPublicKey(s.PublicKey, 1) }

// GetPublicKey returns PublicKey.
func (s *RCD1Signature) GetPublicKey() []byte { return s.PublicKey }

// Verify returns true if this signature is a valid RCD1 signature of the hash.
func (e *RCD1Signature) Verify(txnHash []byte) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	data := e.MetadataHash()
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

// MetadataHash hashes the signature metadata.
func (s *RCD1Signature) MetadataHash() []byte {
	r := *s                        // Copy the struct
	r.Signature = nil              // Clear the signature
	r.TransactionHash = [32]byte{} // And the transaction hash
	return r.Hash()                // Hash it
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *RCD1Signature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *RCD1Signature) GetVote() VoteType {
	return s.Vote
}

/*
 * BTC Signature
 */

func SignBTC(sig *BTCSignature, privateKey, txnHash []byte) {
	data := sig.MetadataHash()
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	pvkey, pubKey := btc.PrivKeyFromBytes(btc.S256(), privateKey)
	sig.PublicKey = pubKey.SerializeUncompressed()
	sign, err := pvkey.Sign(hash[:])
	if err != nil {
		fmt.Println("Unable to sign the txn invalid privatekey")
	}
	sig.Signature = sign.Serialize()
}

// GetSigner returns Signer.
func (s *BTCSignature) GetSigner() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *BTCSignature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *BTCSignature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns the hash of PublicKey.
func (s *BTCSignature) GetPublicKeyHash() []byte { return BTCaddress(s.PublicKey) }

// GetPublicKey returns PublicKey.
func (s *BTCSignature) GetPublicKey() []byte { return s.PublicKey }

// GetSignature returns Signature.
func (s *BTCSignature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *BTCSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *BTCSignature) Hash() []byte { return signatureHash(s) }

// MetadataHash hashes the signature metadata.
func (s *BTCSignature) MetadataHash() []byte {
	r := *s           // Copy the struct
	r.Signature = nil // Clear the signature
	return r.Hash()   // Hash it
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *BTCSignature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *BTCSignature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid SECP256K1 signature of the
// hash.
func (e *BTCSignature) Verify(txnHash []byte) bool {

	data := e.MetadataHash()
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

func SignETH(sig *ETHSignature, privateKey, txnHash []byte) {
	data := sig.MetadataHash()
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	pvkey, pubKey := btc.PrivKeyFromBytes(btc.S256(), privateKey)
	sig.PublicKey = pubKey.SerializeUncompressed()
	sign, err := pvkey.Sign(hash[:])
	if err != nil {
		fmt.Println("Unable to sign the txn invalid privatekey")
	}
	sig.Signature = sign.Serialize()
}

// GetSigner returns Signer.
func (s *ETHSignature) GetSigner() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *ETHSignature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *ETHSignature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKeyHash returns the hash of PublicKey.
func (s *ETHSignature) GetPublicKeyHash() []byte { return ETHaddress(s.PublicKey) }

// GetPublicKey returns PublicKey.
func (s *ETHSignature) GetPublicKey() []byte { return s.PublicKey }

// GetSignature returns Signature.
func (s *ETHSignature) GetSignature() []byte { return s.Signature }

// GetTransactionHash returns TransactionHash.
func (s *ETHSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *ETHSignature) Hash() []byte { return signatureHash(s) }

// MetadataHash hashes the signature metadata.
func (s *ETHSignature) MetadataHash() []byte {
	r := *s           // Copy the struct
	r.Signature = nil // Clear the signature
	return r.Hash()   // Hash it
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *ETHSignature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *ETHSignature) GetVote() VoteType {
	return s.Vote
}

// Verify returns true if this signature is a valid SECP256K1 signature of the
// hash.
func (e *ETHSignature) Verify(txnHash []byte) bool {

	data := e.MetadataHash()
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

// GetSigner returns SourceNetwork.
func (s *ReceiptSignature) GetSigner() *url.URL { return netVal(s.SourceNetwork) }

// GetSignerVersion returns 1.
func (s *ReceiptSignature) GetSignerVersion() uint64 { return 1 }

// GetTimestamp returns 1.
func (s *ReceiptSignature) GetTimestamp() uint64 { return 1 }

// GetPublicKey returns nil.
func (s *ReceiptSignature) GetPublicKeyHash() []byte { return nil }

// GetSignature returns the marshalled receipt.
func (s *ReceiptSignature) GetSignature() []byte {
	b, _ := s.Receipt.MarshalBinary()
	return b
}

// GetTransactionHash returns TransactionHash.
func (s *ReceiptSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *ReceiptSignature) Hash() []byte { return signatureHash(s) }

// MetadataHash hashes the signature metadata.
func (s *ReceiptSignature) MetadataHash() []byte {
	r := *s                        // Copy the struct
	r.TransactionHash = [32]byte{} // Clear the transaction hash
	return r.Hash()                // Hash it
}

// InitiatorHash returns an error.
func (s *ReceiptSignature) InitiatorHash() ([]byte, error) {
	return nil, fmt.Errorf("a receipt signature cannot initiate a transaction")
}

// GetVote returns how the signer votes on a particular transaction
func (s *ReceiptSignature) GetVote() VoteType {
	return VoteTypeAccept
}

// Verify returns true if this receipt is a valid receipt of the hash.
func (s *ReceiptSignature) Verify(hash []byte) bool {
	return bytes.Equal(s.Start, hash) && s.Receipt.Convert().Validate()
}

/*
 * Synthetic Signature
 */

// GetSigner returns the URL of the destination network's validator key page.
func (s *SyntheticSignature) GetSigner() *url.URL { return netVal(s.DestinationNetwork) }

// GetSignerVersion returns 1.
func (s *SyntheticSignature) GetSignerVersion() uint64 { return 1 }

// GetTimestamp returns 1.
func (s *SyntheticSignature) GetTimestamp() uint64 { return 1 }

// GetPublicKey returns nil.
func (s *SyntheticSignature) GetPublicKeyHash() []byte { return nil }

// GetSignature returns nil.
func (s *SyntheticSignature) GetSignature() []byte { return nil }

// GetTransactionHash returns TransactionHash.
func (s *SyntheticSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *SyntheticSignature) Hash() []byte { return signatureHash(s) }

// MetadataHash hashes the signature metadata.
func (s *SyntheticSignature) MetadataHash() []byte {
	r := *s                        // Copy the struct
	r.TransactionHash = [32]byte{} // Clear the transaction hash
	return r.Hash()                // Hash it
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *SyntheticSignature) InitiatorHash() ([]byte, error) {
	if s.SourceNetwork == nil || s.DestinationNetwork == nil || s.SequenceNumber == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 3)
	hasher.AddUrl(s.SourceNetwork)
	hasher.AddUrl(s.DestinationNetwork)
	hasher.AddUint(s.SequenceNumber)
	return hasher.MerkleHash(), nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *SyntheticSignature) GetVote() VoteType {
	return VoteTypeAccept
}

// Verify returns true.
func (s *SyntheticSignature) Verify(hash []byte) bool {
	return true
}

/*
 * Internal Signature
 */

// GetSigner returns SourceNetwork.
func (s *InternalSignature) GetSigner() *url.URL { return netVal(s.Network) }

// GetSignerVersion returns 1.
func (s *InternalSignature) GetSignerVersion() uint64 { return 1 }

// GetTimestamp returns 1.
func (s *InternalSignature) GetTimestamp() uint64 { return 1 }

// GetPublicKey returns nil
func (s *InternalSignature) GetPublicKeyHash() []byte { return nil }

// GetSignature returns nil.
func (s *InternalSignature) GetSignature() []byte { return nil }

// GetTransactionHash returns TransactionHash.
func (s *InternalSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// Hash returns the hash of the signature.
func (s *InternalSignature) Hash() []byte { return signatureHash(s) }

// MetadataHash hashes the signature metadata.
func (s *InternalSignature) MetadataHash() []byte {
	r := *s                        // Copy the struct
	r.TransactionHash = [32]byte{} // Clear the transaction hash
	return r.Hash()                // Hash it
}

// InitiatorHash returns the network account ID.
func (s *InternalSignature) InitiatorHash() ([]byte, error) {
	if s.Network == nil {
		return nil, ErrCannotInitiate
	}

	return s.Network.AccountID(), nil
}

// GetVote returns how the signer votes on a particular transaction
func (s *InternalSignature) GetVote() VoteType {
	return VoteTypeAccept
}

// Verify returns true.
func (s *InternalSignature) Verify(hash []byte) bool {
	return true
}

/*
 * Forwarded Signature
 */

func (s *ForwardedSignature) GetVote() VoteType              { return s.Signature.GetVote() }
func (s *ForwardedSignature) GetSigner() *url.URL            { return s.Signature.GetSigner() }
func (s *ForwardedSignature) GetSignerVersion() uint64       { return s.Signature.GetSignerVersion() }
func (s *ForwardedSignature) GetTimestamp() uint64           { return s.Signature.GetTimestamp() }
func (s *ForwardedSignature) GetSignature() []byte           { return s.Signature.GetSignature() }
func (s *ForwardedSignature) GetPublicKeyHash() []byte       { return s.Signature.GetPublicKeyHash() }
func (s *ForwardedSignature) GetTransactionHash() [32]byte   { return s.Signature.GetTransactionHash() }
func (s *ForwardedSignature) Hash() []byte                   { return s.Signature.Hash() }
func (s *ForwardedSignature) MetadataHash() []byte           { return s.Signature.MetadataHash() }
func (s *ForwardedSignature) InitiatorHash() ([]byte, error) { return s.Signature.InitiatorHash() }
func (s *ForwardedSignature) Verify(hash []byte) bool        { return s.Signature.Verify(hash) }
func (s *ForwardedSignature) GetPublicKey() []byte           { return s.Signature.GetPublicKey() }
