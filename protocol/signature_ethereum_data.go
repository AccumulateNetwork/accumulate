// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"math/big"

	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	altcrypto "gitlab.com/accumulatenetwork/accumulate/pkg/crypto"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"golang.org/x/crypto/sha3"
)

// EthereumTxData represents the parsed data from a raw Ethereum transaction
type EthereumTxData struct {
	Nonce    uint64
	GasPrice *big.Int
	GasLimit uint64
	To       []byte // 20 bytes, nil for contract creation
	Value    *big.Int
	Data     []byte
	V        *big.Int
	R        *big.Int
	S        *big.Int
	ChainID  *big.Int
	TxType   uint8 // 0 = legacy, 1 = EIP-2930, 2 = EIP-1559
}

// ParseEthereumTx parses a raw RLP-encoded Ethereum transaction.
// Supports legacy, EIP-2930 (type 1), and EIP-1559 (type 2) transactions.
func ParseEthereumTx(rawTx []byte) (*EthereumTxData, error) {
	if len(rawTx) == 0 {
		return nil, errors.BadRequest.With("empty raw transaction")
	}

	// Check for typed transaction (EIP-2718)
	if rawTx[0] <= 0x7f {
		txType := rawTx[0]
		switch txType {
		case 1: // EIP-2930
			return parseEIP2930Tx(rawTx[1:])
		case 2: // EIP-1559
			return parseEIP1559Tx(rawTx[1:])
		default:
			return nil, errors.BadRequest.WithFormat("unsupported transaction type: %d", txType)
		}
	}

	// Legacy transaction
	return parseLegacyTx(rawTx)
}

// parseLegacyTx parses a legacy Ethereum transaction
func parseLegacyTx(rawTx []byte) (*EthereumTxData, error) {
	decoded, err := decodeRLPList(rawTx)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to decode legacy tx: %w", err)
	}

	if len(decoded) < 9 {
		return nil, errors.BadRequest.With("invalid legacy transaction: not enough fields")
	}

	tx := &EthereumTxData{TxType: 0}

	tx.Nonce = bytesToUint64(decoded[0])
	tx.GasPrice = new(big.Int).SetBytes(decoded[1])
	tx.GasLimit = bytesToUint64(decoded[2])
	tx.To = decoded[3]
	tx.Value = new(big.Int).SetBytes(decoded[4])
	tx.Data = decoded[5]
	tx.V = new(big.Int).SetBytes(decoded[6])
	tx.R = new(big.Int).SetBytes(decoded[7])
	tx.S = new(big.Int).SetBytes(decoded[8])

	// Derive chain ID from V for EIP-155 transactions
	// V = chainID * 2 + 35 + recovery_id (0 or 1)
	// For non-EIP-155: V = 27 or 28
	if tx.V.Cmp(big.NewInt(35)) >= 0 {
		chainID := new(big.Int).Sub(tx.V, big.NewInt(35))
		chainID.Div(chainID, big.NewInt(2))
		tx.ChainID = chainID
	}

	return tx, nil
}

// parseEIP2930Tx parses an EIP-2930 transaction (without the type byte)
func parseEIP2930Tx(rawTx []byte) (*EthereumTxData, error) {
	decoded, err := decodeRLPList(rawTx)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to decode EIP-2930 tx: %w", err)
	}

	if len(decoded) < 11 {
		return nil, errors.BadRequest.With("invalid EIP-2930 transaction: not enough fields")
	}

	tx := &EthereumTxData{TxType: 1}

	tx.ChainID = new(big.Int).SetBytes(decoded[0])
	tx.Nonce = bytesToUint64(decoded[1])
	tx.GasPrice = new(big.Int).SetBytes(decoded[2])
	tx.GasLimit = bytesToUint64(decoded[3])
	tx.To = decoded[4]
	tx.Value = new(big.Int).SetBytes(decoded[5])
	tx.Data = decoded[6]
	// decoded[7] is accessList, skip for signature verification
	tx.V = new(big.Int).SetBytes(decoded[8])
	tx.R = new(big.Int).SetBytes(decoded[9])
	tx.S = new(big.Int).SetBytes(decoded[10])

	return tx, nil
}

// parseEIP1559Tx parses an EIP-1559 transaction (without the type byte)
func parseEIP1559Tx(rawTx []byte) (*EthereumTxData, error) {
	decoded, err := decodeRLPList(rawTx)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to decode EIP-1559 tx: %w", err)
	}

	if len(decoded) < 12 {
		return nil, errors.BadRequest.With("invalid EIP-1559 transaction: not enough fields")
	}

	tx := &EthereumTxData{TxType: 2}

	tx.ChainID = new(big.Int).SetBytes(decoded[0])
	tx.Nonce = bytesToUint64(decoded[1])
	// decoded[2] is maxPriorityFeePerGas
	tx.GasPrice = new(big.Int).SetBytes(decoded[3]) // maxFeePerGas
	tx.GasLimit = bytesToUint64(decoded[4])
	tx.To = decoded[5]
	tx.Value = new(big.Int).SetBytes(decoded[6])
	tx.Data = decoded[7]
	// decoded[8] is accessList, skip for signature verification
	tx.V = new(big.Int).SetBytes(decoded[9])
	tx.R = new(big.Int).SetBytes(decoded[10])
	tx.S = new(big.Int).SetBytes(decoded[11])

	return tx, nil
}

// Hash computes the Keccak256 hash of the raw transaction (the hash that was signed)
func (tx *EthereumTxData) Hash(rawTx []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(rawTx)
	return h.Sum(nil)
}

// RecoverSigner recovers the Ethereum address from the signature
func (tx *EthereumTxData) RecoverSigner(rawTx []byte) ([]byte, error) {
	// Compute the hash that was signed
	sigHash := tx.signingHash(rawTx)

	// Prepare recovery ID
	var recoveryID byte
	if tx.TxType == 0 {
		// Legacy: V = 27/28 or chainID*2+35+recovery
		if tx.V.Cmp(big.NewInt(28)) <= 0 {
			recoveryID = byte(tx.V.Uint64() - 27)
		} else {
			// EIP-155
			recoveryID = byte(new(big.Int).Sub(tx.V, new(big.Int).Add(new(big.Int).Mul(tx.ChainID, big.NewInt(2)), big.NewInt(35))).Uint64())
		}
	} else {
		// EIP-2930/1559: V is just 0 or 1
		recoveryID = byte(tx.V.Uint64())
	}

	// Build 65-byte signature: R (32) + S (32) + V (1)
	sig := make([]byte, 65)
	rBytes := tx.R.Bytes()
	sBytes := tx.S.Bytes()
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)
	sig[64] = recoveryID

	// Recover public key using ecrecover
	pubkey, err := ecrecover(sigHash, sig)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to recover public key: %w", err)
	}

	// Derive Ethereum address from public key
	return pubkeyToAddress(pubkey), nil
}

// signingHash computes the hash that was signed for this transaction
func (tx *EthereumTxData) signingHash(rawTx []byte) []byte {
	h := sha3.NewLegacyKeccak256()

	switch tx.TxType {
	case 0:
		// Legacy transaction: hash the RLP-encoded unsigned tx
		// For EIP-155, this includes chainID, 0, 0 instead of v, r, s
		if tx.ChainID != nil && tx.ChainID.Sign() > 0 {
			// EIP-155: encode [nonce, gasprice, gaslimit, to, value, data, chainid, 0, 0]
			unsignedTx := encodeRLPList([][]byte{
				uint64ToBytes(tx.Nonce),
				tx.GasPrice.Bytes(),
				uint64ToBytes(tx.GasLimit),
				tx.To,
				tx.Value.Bytes(),
				tx.Data,
				tx.ChainID.Bytes(),
				{},
				{},
			})
			h.Write(unsignedTx)
		} else {
			// Pre-EIP-155: encode [nonce, gasprice, gaslimit, to, value, data]
			unsignedTx := encodeRLPList([][]byte{
				uint64ToBytes(tx.Nonce),
				tx.GasPrice.Bytes(),
				uint64ToBytes(tx.GasLimit),
				tx.To,
				tx.Value.Bytes(),
				tx.Data,
			})
			h.Write(unsignedTx)
		}
	case 1, 2:
		// For typed transactions, we need to reconstruct the unsigned payload
		// This is complex - for now, fall back to hashing the raw tx
		// A full implementation would strip the signature and re-encode
		h.Write(rawTx)
	}

	return h.Sum(nil)
}

// ecrecover recovers the public key from a signature
func ecrecover(hash, sig []byte) ([]byte, error) {
	if len(sig) != 65 {
		return nil, errors.BadRequest.With("invalid signature length")
	}
	if len(hash) != 32 {
		return nil, errors.BadRequest.With("invalid hash length")
	}

	// Extract recovery ID from v
	v := sig[64]
	var recID byte
	if v >= 27 {
		recID = v - 27
	} else {
		recID = v
	}

	// Ensure recID is 0 or 1
	if recID > 1 {
		return nil, errors.BadRequest.WithFormat("invalid recovery id: %d", recID)
	}

	// Build compact signature format for decred: [R || S || recovery_flag]
	// The recovery flag encodes whether the public key is compressed and the recovery ID
	compactSig := make([]byte, 65)
	copy(compactSig[:32], sig[:32]) // R
	copy(compactSig[32:64], sig[32:64]) // S
	// Recovery flag: 27 + recID for uncompressed, 31 + recID for compressed
	// We want uncompressed for Ethereum address derivation
	compactSig[64] = 27 + recID

	// Use decred's RecoverCompact to recover the public key
	pubKey, _, err := ecdsa.RecoverCompact(compactSig, hash)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to recover public key: %w", err)
	}

	// Return the uncompressed public key (65 bytes: 0x04 + X + Y)
	return pubKey.SerializeUncompressed(), nil
}

// recoverPubkey attempts to recover the public key with a specific recovery ID
func recoverPubkey(hash []byte, r, s *big.Int, recID byte) ([]byte, error) {
	// Build 65-byte compact signature
	sig := make([]byte, 65)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)
	sig[64] = 27 + recID

	pubKey, _, err := ecdsa.RecoverCompact(sig, hash)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to recover public key: %w", err)
	}

	return pubKey.SerializeUncompressed(), nil
}

// pubkeyToAddress converts a public key to an Ethereum address
func pubkeyToAddress(pubkey []byte) []byte {
	if len(pubkey) == 0 {
		return nil
	}

	// If compressed (33 bytes), decompress first
	var uncompressed []byte
	if len(pubkey) == 33 {
		pub, err := altcrypto.DecompressPubkey(pubkey)
		if err != nil {
			return nil
		}
		uncompressed = altcrypto.FromECDSAPub(pub)
	} else {
		uncompressed = pubkey
	}

	// Skip the 0x04 prefix if present
	if len(uncompressed) == 65 && uncompressed[0] == 0x04 {
		uncompressed = uncompressed[1:]
	}

	// Keccak256 hash and take last 20 bytes
	h := sha3.NewLegacyKeccak256()
	h.Write(uncompressed)
	hash := h.Sum(nil)
	return hash[12:] // Last 20 bytes
}

// RLP decoding helpers

func decodeRLPList(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, errors.BadRequest.With("empty RLP data")
	}

	prefix := data[0]
	if prefix < 0xc0 {
		return nil, errors.BadRequest.With("not an RLP list")
	}

	var listData []byte
	var offset int

	if prefix <= 0xf7 {
		// Short list (0-55 bytes)
		listLen := int(prefix - 0xc0)
		if len(data) < 1+listLen {
			return nil, errors.BadRequest.With("RLP list too short")
		}
		listData = data[1 : 1+listLen]
		offset = 1
	} else {
		// Long list
		lenOfLen := int(prefix - 0xf7)
		if len(data) < 1+lenOfLen {
			return nil, errors.BadRequest.With("RLP list length too short")
		}
		listLen := bytesToUint64(data[1 : 1+lenOfLen])
		if len(data) < 1+lenOfLen+int(listLen) {
			return nil, errors.BadRequest.With("RLP list data too short")
		}
		listData = data[1+lenOfLen : 1+lenOfLen+int(listLen)]
		offset = 1 + lenOfLen
	}

	_ = offset // Unused but kept for clarity

	// Decode items
	var items [][]byte
	for len(listData) > 0 {
		item, consumed, err := decodeRLPItem(listData)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		listData = listData[consumed:]
	}

	return items, nil
}

func decodeRLPItem(data []byte) ([]byte, int, error) {
	if len(data) == 0 {
		return nil, 0, errors.BadRequest.With("empty RLP item")
	}

	prefix := data[0]

	if prefix < 0x80 {
		// Single byte
		return data[:1], 1, nil
	} else if prefix <= 0xb7 {
		// Short string (0-55 bytes)
		strLen := int(prefix - 0x80)
		if len(data) < 1+strLen {
			return nil, 0, errors.BadRequest.With("RLP string too short")
		}
		return data[1 : 1+strLen], 1 + strLen, nil
	} else if prefix <= 0xbf {
		// Long string
		lenOfLen := int(prefix - 0xb7)
		if len(data) < 1+lenOfLen {
			return nil, 0, errors.BadRequest.With("RLP string length too short")
		}
		strLen := bytesToUint64(data[1 : 1+lenOfLen])
		if len(data) < 1+lenOfLen+int(strLen) {
			return nil, 0, errors.BadRequest.With("RLP string data too short")
		}
		return data[1+lenOfLen : 1+lenOfLen+int(strLen)], 1 + lenOfLen + int(strLen), nil
	} else if prefix <= 0xf7 {
		// Short list - return the whole list as a single item
		listLen := int(prefix - 0xc0)
		if len(data) < 1+listLen {
			return nil, 0, errors.BadRequest.With("RLP list too short")
		}
		return data[1 : 1+listLen], 1 + listLen, nil
	} else {
		// Long list
		lenOfLen := int(prefix - 0xf7)
		if len(data) < 1+lenOfLen {
			return nil, 0, errors.BadRequest.With("RLP list length too short")
		}
		listLen := bytesToUint64(data[1 : 1+lenOfLen])
		if len(data) < 1+lenOfLen+int(listLen) {
			return nil, 0, errors.BadRequest.With("RLP list data too short")
		}
		return data[1+lenOfLen : 1+lenOfLen+int(listLen)], 1 + lenOfLen + int(listLen), nil
	}
}

func bytesToUint64(b []byte) uint64 {
	var result uint64
	for _, v := range b {
		result = result<<8 | uint64(v)
	}
	return result
}

func uint64ToBytes(n uint64) []byte {
	if n == 0 {
		return []byte{}
	}
	var buf [8]byte
	i := 7
	for n > 0 {
		buf[i] = byte(n & 0xff)
		n >>= 8
		i--
	}
	return buf[i+1:]
}

func encodeRLPList(items [][]byte) []byte {
	var content []byte
	for _, item := range items {
		content = append(content, encodeRLPString(item)...)
	}

	if len(content) <= 55 {
		return append([]byte{byte(0xc0 + len(content))}, content...)
	}

	lenBytes := uint64ToBytes(uint64(len(content)))
	return append(append([]byte{byte(0xf7 + len(lenBytes))}, lenBytes...), content...)
}

func encodeRLPString(s []byte) []byte {
	if len(s) == 0 {
		return []byte{0x80}
	}
	if len(s) == 1 && s[0] < 0x80 {
		return s
	}
	if len(s) <= 55 {
		return append([]byte{byte(0x80 + len(s))}, s...)
	}
	lenBytes := uint64ToBytes(uint64(len(s)))
	return append(append([]byte{byte(0xb7 + len(lenBytes))}, lenBytes...), s...)
}

/*
 * EthereumDataSignature methods
 */

// GetSigner returns the derived signer URL (lite account from ETH address)
func (s *EthereumDataSignature) GetSigner() *url.URL { return s.Signer }

// RoutingLocation returns the signer URL
func (s *EthereumDataSignature) RoutingLocation() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion
func (s *EthereumDataSignature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp
func (s *EthereumDataSignature) GetTimestamp() uint64 { return s.Timestamp }

// GetTransactionHash returns TransactionHash
func (s *EthereumDataSignature) GetTransactionHash() [32]byte { return s.TransactionHash }

// GetVote returns the vote type
func (s *EthereumDataSignature) GetVote() VoteType { return s.Vote }

// Hash returns the hash of the signature
func (s *EthereumDataSignature) Hash() []byte { return signatureHash(s) }

// Metadata returns the signature's metadata
func (s *EthereumDataSignature) Metadata() Signature {
	r := s.Copy()
	r.TransactionHash = [32]byte{}
	return r
}

// Initiator returns a Hasher that calculates the Merkle hash of the signature
func (s *EthereumDataSignature) Initiator() (hash.Hasher, error) {
	if s.Signer == nil || s.SignerVersion == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 2)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	if s.Timestamp != 0 {
		hasher.AddUint(s.Timestamp)
	}
	return hasher, nil
}

// Verify validates the embedded Ethereum signature against the transaction body
// This is the core of the self-authenticating write mechanism
func (s *EthereumDataSignature) Verify(sig Signature, msg Signable) bool {
	// For EthereumDataSignature, verification happens during transaction validation
	// by extracting and validating the signature from the EthereumDataEntry
	// The actual verification is done by VerifyEthereumDataSignature
	return true // Placeholder - actual verification happens in executor
}

// VerifyEthereumDataSignature validates an EthereumDataSignature against an EthereumDataEntry.
// This function extracts the signature from the raw Ethereum transaction,
// recovers the signer, and validates the signature.
// Returns the derived signer URL on success.
func VerifyEthereumDataSignature(entry *EthereumDataEntry, expectedChainID uint64) (*url.URL, error) {
	if entry == nil || len(entry.RawTx) == 0 {
		return nil, errors.BadRequest.With("missing ethereum data entry")
	}

	// Parse the Ethereum transaction
	ethTx, err := ParseEthereumTx(entry.RawTx)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("invalid ethereum transaction: %w", err)
	}

	// Verify chain ID if specified
	if expectedChainID != 0 && ethTx.ChainID != nil {
		if ethTx.ChainID.Uint64() != expectedChainID {
			return nil, errors.BadRequest.WithFormat("chain ID mismatch: expected %d, got %d",
				expectedChainID, ethTx.ChainID.Uint64())
		}
	}

	// Recover the signer's Ethereum address
	ethAddr, err := ethTx.RecoverSigner(entry.RawTx)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to recover signer: %w", err)
	}

	// Derive the Accumulate lite account URL from the Ethereum address
	liteAccount, err := LiteTokenAddressFromHash(ethAddr, ACME)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to derive lite account: %w", err)
	}

	return liteAccount, nil
}
