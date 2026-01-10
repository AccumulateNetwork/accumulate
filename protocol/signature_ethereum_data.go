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

// Maximum size for individual RLP elements to prevent DoS attacks
const maxRLPElementSize = 1 << 20 // 1MB

// EthereumTxData represents the parsed data from a raw Ethereum transaction
type EthereumTxData struct {
	Nonce                uint64
	GasPrice             *big.Int // Also used as maxFeePerGas for EIP-1559
	MaxPriorityFeePerGas *big.Int // EIP-1559 only
	GasLimit             uint64
	To                   []byte // 20 bytes, nil for contract creation
	Value                *big.Int
	Data                 []byte
	AccessList           []byte // Raw RLP-encoded access list for EIP-2930/1559
	V                    *big.Int
	R                    *big.Int
	S                    *big.Int
	ChainID              *big.Int
	TxType               uint8 // 0 = legacy, 1 = EIP-2930, 2 = EIP-1559
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
	decoded, rawItems, err := decodeRLPListWithRaw(rawTx)
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
	tx.AccessList = rawItems[7] // Store raw RLP-encoded access list
	tx.V = new(big.Int).SetBytes(decoded[8])
	tx.R = new(big.Int).SetBytes(decoded[9])
	tx.S = new(big.Int).SetBytes(decoded[10])

	return tx, nil
}

// parseEIP1559Tx parses an EIP-1559 transaction (without the type byte)
func parseEIP1559Tx(rawTx []byte) (*EthereumTxData, error) {
	decoded, rawItems, err := decodeRLPListWithRaw(rawTx)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to decode EIP-1559 tx: %w", err)
	}

	if len(decoded) < 12 {
		return nil, errors.BadRequest.With("invalid EIP-1559 transaction: not enough fields")
	}

	tx := &EthereumTxData{TxType: 2}

	tx.ChainID = new(big.Int).SetBytes(decoded[0])
	tx.Nonce = bytesToUint64(decoded[1])
	tx.MaxPriorityFeePerGas = new(big.Int).SetBytes(decoded[2])
	tx.GasPrice = new(big.Int).SetBytes(decoded[3]) // maxFeePerGas
	tx.GasLimit = bytesToUint64(decoded[4])
	tx.To = decoded[5]
	tx.Value = new(big.Int).SetBytes(decoded[6])
	tx.Data = decoded[7]
	tx.AccessList = rawItems[8] // Store raw RLP-encoded access list
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
	addr, err := pubkeyToAddress(pubkey)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to derive address: %w", err)
	}
	return addr, nil
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
				bigIntToBytes(tx.GasPrice),
				uint64ToBytes(tx.GasLimit),
				tx.To,
				bigIntToBytes(tx.Value),
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
				bigIntToBytes(tx.GasPrice),
				uint64ToBytes(tx.GasLimit),
				tx.To,
				bigIntToBytes(tx.Value),
				tx.Data,
			})
			h.Write(unsignedTx)
		}

	case 1:
		// EIP-2930: hash 0x01 || RLP([chainId, nonce, gasPrice, gasLimit, to, value, data, accessList])
		unsignedPayload := encodeRLPListRaw([][]byte{
			tx.ChainID.Bytes(),
			uint64ToBytes(tx.Nonce),
			bigIntToBytes(tx.GasPrice),
			uint64ToBytes(tx.GasLimit),
			tx.To,
			bigIntToBytes(tx.Value),
			tx.Data,
		}, [][]byte{
			tx.AccessList, // Include raw access list without re-encoding
		})
		h.Write([]byte{0x01})
		h.Write(unsignedPayload)

	case 2:
		// EIP-1559: hash 0x02 || RLP([chainId, nonce, maxPriorityFeePerGas, maxFeePerGas, gasLimit, to, value, data, accessList])
		unsignedPayload := encodeRLPListRaw([][]byte{
			tx.ChainID.Bytes(),
			uint64ToBytes(tx.Nonce),
			bigIntToBytes(tx.MaxPriorityFeePerGas),
			bigIntToBytes(tx.GasPrice), // maxFeePerGas
			uint64ToBytes(tx.GasLimit),
			tx.To,
			bigIntToBytes(tx.Value),
			tx.Data,
		}, [][]byte{
			tx.AccessList, // Include raw access list without re-encoding
		})
		h.Write([]byte{0x02})
		h.Write(unsignedPayload)
	}

	return h.Sum(nil)
}

// bigIntToBytes converts a big.Int to bytes, returning empty slice for nil or zero
func bigIntToBytes(n *big.Int) []byte {
	if n == nil || n.Sign() == 0 {
		return []byte{}
	}
	return n.Bytes()
}

// ecrecover recovers the public key from a signature
// Input sig format: [R (32 bytes) || S (32 bytes) || V (1 byte)]
// where V is the recovery ID (0 or 1)
func ecrecover(hash, sig []byte) ([]byte, error) {
	if len(sig) != 65 {
		return nil, errors.BadRequest.With("invalid signature length")
	}
	if len(hash) != 32 {
		return nil, errors.BadRequest.With("invalid hash length")
	}

	// Extract recovery ID from v (last byte)
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

	// Build compact signature format for decred: [recovery_code (1 byte) || R (32 bytes) || S (32 bytes)]
	// The recovery code is: 27 + recID for uncompressed, 31 + recID for compressed
	// We want uncompressed for Ethereum address derivation
	compactSig := make([]byte, 65)
	compactSig[0] = 27 + recID          // Recovery code at first byte
	copy(compactSig[1:33], sig[:32])    // R
	copy(compactSig[33:65], sig[32:64]) // S

	// Use decred's RecoverCompact to recover the public key
	pubKey, _, err := ecdsa.RecoverCompact(compactSig, hash)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to recover public key: %w", err)
	}

	// Return the uncompressed public key (65 bytes: 0x04 + X + Y)
	return pubKey.SerializeUncompressed(), nil
}

// recoverPubkey attempts to recover the public key with a specific recovery ID
// Decred compact format: [recovery_code (1 byte) || R (32 bytes) || S (32 bytes)]
func recoverPubkey(hash []byte, r, s *big.Int, recID byte) ([]byte, error) {
	// Build 65-byte compact signature for decred
	sig := make([]byte, 65)
	sig[0] = 27 + recID // Recovery code at first byte (27 = uncompressed)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[1+32-len(rBytes):33], rBytes) // R starts at position 1
	copy(sig[33+32-len(sBytes):65], sBytes) // S starts at position 33

	pubKey, _, err := ecdsa.RecoverCompact(sig, hash)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to recover public key: %w", err)
	}

	return pubKey.SerializeUncompressed(), nil
}

// pubkeyToAddress converts a public key to an Ethereum address
func pubkeyToAddress(pubkey []byte) ([]byte, error) {
	if len(pubkey) == 0 {
		return nil, errors.BadRequest.With("empty public key")
	}

	// If compressed (33 bytes), decompress first
	var uncompressed []byte
	if len(pubkey) == 33 {
		pub, err := altcrypto.DecompressPubkey(pubkey)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("failed to decompress public key: %w", err)
		}
		uncompressed = altcrypto.FromECDSAPub(pub)
	} else if len(pubkey) == 65 {
		uncompressed = pubkey
	} else {
		return nil, errors.BadRequest.WithFormat("invalid public key length: %d (expected 33 or 65)", len(pubkey))
	}

	// Skip the 0x04 prefix if present
	if len(uncompressed) == 65 && uncompressed[0] == 0x04 {
		uncompressed = uncompressed[1:]
	}

	// Keccak256 hash and take last 20 bytes
	h := sha3.NewLegacyKeccak256()
	h.Write(uncompressed)
	hash := h.Sum(nil)
	return hash[12:], nil // Last 20 bytes
}

// RLP decoding helpers

func decodeRLPList(data []byte) ([][]byte, error) {
	items, _, err := decodeRLPListWithRaw(data)
	return items, err
}

// decodeRLPListWithRaw decodes an RLP list and returns both decoded items and raw RLP-encoded items.
// This is needed for EIP-2930/1559 where we need the raw access list encoding for the signing hash.
func decodeRLPListWithRaw(data []byte) ([][]byte, [][]byte, error) {
	if len(data) == 0 {
		return nil, nil, errors.BadRequest.With("empty RLP data")
	}

	prefix := data[0]
	if prefix < 0xc0 {
		return nil, nil, errors.BadRequest.With("not an RLP list")
	}

	var listData []byte

	if prefix <= 0xf7 {
		// Short list (0-55 bytes)
		listLen := int(prefix - 0xc0)
		if len(data) < 1+listLen {
			return nil, nil, errors.BadRequest.With("RLP list too short")
		}
		listData = data[1 : 1+listLen]
	} else {
		// Long list
		lenOfLen := int(prefix - 0xf7)
		if len(data) < 1+lenOfLen {
			return nil, nil, errors.BadRequest.With("RLP list length too short")
		}
		listLen, err := bytesToUint64Safe(data[1 : 1+lenOfLen])
		if err != nil {
			return nil, nil, err
		}
		if listLen > maxRLPElementSize {
			return nil, nil, errors.BadRequest.WithFormat("RLP list too large: %d > %d", listLen, maxRLPElementSize)
		}
		if len(data) < 1+lenOfLen+int(listLen) {
			return nil, nil, errors.BadRequest.With("RLP list data too short")
		}
		listData = data[1+lenOfLen : 1+lenOfLen+int(listLen)]
	}

	// Decode items, keeping track of raw encoding
	var items [][]byte
	var rawItems [][]byte
	for len(listData) > 0 {
		item, rawItem, consumed, err := decodeRLPItemWithRaw(listData)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, item)
		rawItems = append(rawItems, rawItem)
		listData = listData[consumed:]
	}

	return items, rawItems, nil
}

func decodeRLPItem(data []byte) ([]byte, int, error) {
	item, _, consumed, err := decodeRLPItemWithRaw(data)
	return item, consumed, err
}

// decodeRLPItemWithRaw decodes an RLP item and returns the decoded content,
// the raw RLP-encoded bytes (including prefix), and bytes consumed.
func decodeRLPItemWithRaw(data []byte) ([]byte, []byte, int, error) {
	if len(data) == 0 {
		return nil, nil, 0, errors.BadRequest.With("empty RLP item")
	}

	prefix := data[0]

	if prefix < 0x80 {
		// Single byte
		return data[:1], data[:1], 1, nil
	} else if prefix <= 0xb7 {
		// Short string (0-55 bytes)
		strLen := int(prefix - 0x80)
		if len(data) < 1+strLen {
			return nil, nil, 0, errors.BadRequest.With("RLP string too short")
		}
		totalLen := 1 + strLen
		return data[1 : 1+strLen], data[:totalLen], totalLen, nil
	} else if prefix <= 0xbf {
		// Long string
		lenOfLen := int(prefix - 0xb7)
		if len(data) < 1+lenOfLen {
			return nil, nil, 0, errors.BadRequest.With("RLP string length too short")
		}
		strLen, err := bytesToUint64Safe(data[1 : 1+lenOfLen])
		if err != nil {
			return nil, nil, 0, err
		}
		if strLen > maxRLPElementSize {
			return nil, nil, 0, errors.BadRequest.WithFormat("RLP string too large: %d > %d", strLen, maxRLPElementSize)
		}
		totalLen := 1 + lenOfLen + int(strLen)
		if len(data) < totalLen {
			return nil, nil, 0, errors.BadRequest.With("RLP string data too short")
		}
		return data[1+lenOfLen : totalLen], data[:totalLen], totalLen, nil
	} else if prefix <= 0xf7 {
		// Short list - return the whole list as a single item (content only for item, full for raw)
		listLen := int(prefix - 0xc0)
		totalLen := 1 + listLen
		if len(data) < totalLen {
			return nil, nil, 0, errors.BadRequest.With("RLP list too short")
		}
		return data[1:totalLen], data[:totalLen], totalLen, nil
	} else {
		// Long list
		lenOfLen := int(prefix - 0xf7)
		if len(data) < 1+lenOfLen {
			return nil, nil, 0, errors.BadRequest.With("RLP list length too short")
		}
		listLen, err := bytesToUint64Safe(data[1 : 1+lenOfLen])
		if err != nil {
			return nil, nil, 0, err
		}
		if listLen > maxRLPElementSize {
			return nil, nil, 0, errors.BadRequest.WithFormat("RLP list too large: %d > %d", listLen, maxRLPElementSize)
		}
		totalLen := 1 + lenOfLen + int(listLen)
		if len(data) < totalLen {
			return nil, nil, 0, errors.BadRequest.With("RLP list data too short")
		}
		return data[1+lenOfLen : totalLen], data[:totalLen], totalLen, nil
	}
}

func bytesToUint64(b []byte) uint64 {
	var result uint64
	for _, v := range b {
		result = result<<8 | uint64(v)
	}
	return result
}

// bytesToUint64Safe converts bytes to uint64 with overflow checking
func bytesToUint64Safe(b []byte) (uint64, error) {
	if len(b) > 8 {
		return 0, errors.BadRequest.WithFormat("RLP length field too large: %d bytes", len(b))
	}
	var result uint64
	for _, v := range b {
		result = result<<8 | uint64(v)
	}
	return result, nil
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

// encodeRLPListRaw encodes a list where some items need RLP string encoding
// and others are already RLP-encoded (like access lists).
func encodeRLPListRaw(items [][]byte, rawItems [][]byte) []byte {
	var content []byte
	for _, item := range items {
		content = append(content, encodeRLPString(item)...)
	}
	// Append raw items without re-encoding
	for _, raw := range rawItems {
		content = append(content, raw...)
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

// Verify validates the embedded Ethereum signature against the transaction body.
// For EthereumDataSignature, this method returns false because verification
// requires access to the transaction body (EthereumDataEntry) which is not
// available through the Signable interface. Use VerifyEthereumDataSignature
// instead, which is called by the executor during transaction validation.
func (s *EthereumDataSignature) Verify(sig Signature, msg Signable) bool {
	// EthereumDataSignature cannot be verified through this interface because
	// the signature is embedded in the EthereumDataEntry, not in the signature itself.
	// Verification must happen via VerifyEthereumDataSignature which takes the entry.
	return false
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
	if expectedChainID != 0 {
		if ethTx.ChainID == nil || ethTx.ChainID.Sign() == 0 {
			// Pre-EIP-155 transactions don't have chain ID - reject if chain ID is required
			return nil, errors.BadRequest.WithFormat("chain ID required but transaction has no chain ID (pre-EIP-155)")
		}
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
