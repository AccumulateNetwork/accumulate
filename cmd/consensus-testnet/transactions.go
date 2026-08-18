// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"
)

// TransactionType identifies the type of transaction.
type TransactionType uint8

const (
	// TxTypeData is a simple data transaction for testing throughput.
	TxTypeData TransactionType = iota + 1

	// TxTypeSetBlockTime changes the block production interval.
	TxTypeSetBlockTime

	// TxTypeSetTxRate changes the transaction generation rate.
	TxTypeSetTxRate
)

// Transaction is the common interface for all testnet transactions.
type Transaction interface {
	Type() TransactionType
	Hash() [32]byte
	Marshal() []byte
	Sender() ed25519.PublicKey
	Timestamp() time.Time
}

// TxHeader contains common fields for all transactions.
type TxHeader struct {
	TxType    TransactionType
	From      ed25519.PublicKey
	Time      time.Time
	Nonce     uint64
	Signature []byte
}

// DataTx is a simple data transaction for testing throughput.
type DataTx struct {
	TxHeader
	Data []byte
}

func NewDataTx(from ed25519.PublicKey, data []byte, nonce uint64) *DataTx {
	return &DataTx{
		TxHeader: TxHeader{
			TxType: TxTypeData,
			From:   from,
			Time:   time.Now(),
			Nonce:  nonce,
		},
		Data: data,
	}
}

func (tx *DataTx) Type() TransactionType     { return TxTypeData }
func (tx *DataTx) Sender() ed25519.PublicKey { return tx.From }
func (tx *DataTx) Timestamp() time.Time      { return tx.Time }

func (tx *DataTx) Hash() [32]byte {
	return sha256.Sum256(tx.marshalWithoutSig())
}

func (tx *DataTx) marshalWithoutSig() []byte {
	// Format: type(1) + pubkey(32) + time(8) + nonce(8) + datalen(4) + data
	size := 1 + 32 + 8 + 8 + 4 + len(tx.Data)
	buf := make([]byte, size)
	offset := 0

	buf[offset] = byte(tx.TxType)
	offset++

	copy(buf[offset:], tx.From)
	offset += 32

	binary.BigEndian.PutUint64(buf[offset:], uint64(tx.Time.UnixNano()))
	offset += 8

	binary.BigEndian.PutUint64(buf[offset:], tx.Nonce)
	offset += 8

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(tx.Data)))
	offset += 4

	copy(buf[offset:], tx.Data)
	return buf
}

func (tx *DataTx) Marshal() []byte {
	base := tx.marshalWithoutSig()
	// Append signature length + signature
	buf := make([]byte, len(base)+4+len(tx.Signature))
	copy(buf, base)
	offset := len(base)
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(tx.Signature)))
	offset += 4
	copy(buf[offset:], tx.Signature)
	return buf
}

func (tx *DataTx) Sign(key ed25519.PrivateKey) {
	hash := tx.Hash()
	tx.Signature = ed25519.Sign(key, hash[:])
}

func (tx *DataTx) Verify() bool {
	if len(tx.Signature) != ed25519.SignatureSize {
		return false
	}
	hash := tx.Hash()
	return ed25519.Verify(tx.From, hash[:], tx.Signature)
}

// SetBlockTimeTx changes the block production interval.
type SetBlockTimeTx struct {
	TxHeader
	BlockInterval time.Duration // New block interval
}

func NewSetBlockTimeTx(from ed25519.PublicKey, interval time.Duration, nonce uint64) *SetBlockTimeTx {
	return &SetBlockTimeTx{
		TxHeader: TxHeader{
			TxType: TxTypeSetBlockTime,
			From:   from,
			Time:   time.Now(),
			Nonce:  nonce,
		},
		BlockInterval: interval,
	}
}

func (tx *SetBlockTimeTx) Type() TransactionType     { return TxTypeSetBlockTime }
func (tx *SetBlockTimeTx) Sender() ed25519.PublicKey { return tx.From }
func (tx *SetBlockTimeTx) Timestamp() time.Time      { return tx.Time }

func (tx *SetBlockTimeTx) Hash() [32]byte {
	return sha256.Sum256(tx.marshalWithoutSig())
}

func (tx *SetBlockTimeTx) marshalWithoutSig() []byte {
	// Format: type(1) + pubkey(32) + time(8) + nonce(8) + interval(8)
	buf := make([]byte, 1+32+8+8+8)
	offset := 0

	buf[offset] = byte(tx.TxType)
	offset++

	copy(buf[offset:], tx.From)
	offset += 32

	binary.BigEndian.PutUint64(buf[offset:], uint64(tx.Time.UnixNano()))
	offset += 8

	binary.BigEndian.PutUint64(buf[offset:], tx.Nonce)
	offset += 8

	binary.BigEndian.PutUint64(buf[offset:], uint64(tx.BlockInterval.Nanoseconds()))
	return buf
}

func (tx *SetBlockTimeTx) Marshal() []byte {
	base := tx.marshalWithoutSig()
	buf := make([]byte, len(base)+4+len(tx.Signature))
	copy(buf, base)
	offset := len(base)
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(tx.Signature)))
	offset += 4
	copy(buf[offset:], tx.Signature)
	return buf
}

func (tx *SetBlockTimeTx) Sign(key ed25519.PrivateKey) {
	hash := tx.Hash()
	tx.Signature = ed25519.Sign(key, hash[:])
}

func (tx *SetBlockTimeTx) Verify() bool {
	if len(tx.Signature) != ed25519.SignatureSize {
		return false
	}
	hash := tx.Hash()
	return ed25519.Verify(tx.From, hash[:], tx.Signature)
}

// SetTxRateTx changes the transaction generation rate.
type SetTxRateTx struct {
	TxHeader
	TxPerSecond uint32 // New transaction rate
}

func NewSetTxRateTx(from ed25519.PublicKey, rate uint32, nonce uint64) *SetTxRateTx {
	return &SetTxRateTx{
		TxHeader: TxHeader{
			TxType: TxTypeSetTxRate,
			From:   from,
			Time:   time.Now(),
			Nonce:  nonce,
		},
		TxPerSecond: rate,
	}
}

func (tx *SetTxRateTx) Type() TransactionType     { return TxTypeSetTxRate }
func (tx *SetTxRateTx) Sender() ed25519.PublicKey { return tx.From }
func (tx *SetTxRateTx) Timestamp() time.Time      { return tx.Time }

func (tx *SetTxRateTx) Hash() [32]byte {
	return sha256.Sum256(tx.marshalWithoutSig())
}

func (tx *SetTxRateTx) marshalWithoutSig() []byte {
	// Format: type(1) + pubkey(32) + time(8) + nonce(8) + rate(4)
	buf := make([]byte, 1+32+8+8+4)
	offset := 0

	buf[offset] = byte(tx.TxType)
	offset++

	copy(buf[offset:], tx.From)
	offset += 32

	binary.BigEndian.PutUint64(buf[offset:], uint64(tx.Time.UnixNano()))
	offset += 8

	binary.BigEndian.PutUint64(buf[offset:], tx.Nonce)
	offset += 8

	binary.BigEndian.PutUint32(buf[offset:], tx.TxPerSecond)
	return buf
}

func (tx *SetTxRateTx) Marshal() []byte {
	base := tx.marshalWithoutSig()
	buf := make([]byte, len(base)+4+len(tx.Signature))
	copy(buf, base)
	offset := len(base)
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(tx.Signature)))
	offset += 4
	copy(buf[offset:], tx.Signature)
	return buf
}

func (tx *SetTxRateTx) Sign(key ed25519.PrivateKey) {
	hash := tx.Hash()
	tx.Signature = ed25519.Sign(key, hash[:])
}

func (tx *SetTxRateTx) Verify() bool {
	if len(tx.Signature) != ed25519.SignatureSize {
		return false
	}
	hash := tx.Hash()
	return ed25519.Verify(tx.From, hash[:], tx.Signature)
}

// UnmarshalTransaction deserializes a transaction from bytes.
func UnmarshalTransaction(data []byte) (Transaction, error) {
	if len(data) < 1 {
		return nil, errors.New("transaction data too short")
	}

	txType := TransactionType(data[0])
	switch txType {
	case TxTypeData:
		return unmarshalDataTx(data)
	case TxTypeSetBlockTime:
		return unmarshalSetBlockTimeTx(data)
	case TxTypeSetTxRate:
		return unmarshalSetTxRateTx(data)
	default:
		return nil, errors.New("unknown transaction type")
	}
}

func unmarshalDataTx(data []byte) (*DataTx, error) {
	// Minimum: type(1) + pubkey(32) + time(8) + nonce(8) + datalen(4) + siglen(4) = 57
	if len(data) < 57 {
		return nil, errors.New("data transaction too short")
	}

	tx := &DataTx{}
	offset := 0

	tx.TxType = TransactionType(data[offset])
	offset++

	tx.From = make([]byte, 32)
	copy(tx.From, data[offset:offset+32])
	offset += 32

	tx.Time = time.Unix(0, int64(binary.BigEndian.Uint64(data[offset:])))
	offset += 8

	tx.Nonce = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	dataLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(dataLen)+4 > len(data) {
		return nil, errors.New("data transaction truncated")
	}

	tx.Data = make([]byte, dataLen)
	copy(tx.Data, data[offset:offset+int(dataLen)])
	offset += int(dataLen)

	sigLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(sigLen) > len(data) {
		return nil, errors.New("data transaction signature truncated")
	}

	tx.Signature = make([]byte, sigLen)
	copy(tx.Signature, data[offset:offset+int(sigLen)])

	return tx, nil
}

func unmarshalSetBlockTimeTx(data []byte) (*SetBlockTimeTx, error) {
	// type(1) + pubkey(32) + time(8) + nonce(8) + interval(8) + siglen(4) = 61
	if len(data) < 61 {
		return nil, errors.New("set block time transaction too short")
	}

	tx := &SetBlockTimeTx{}
	offset := 0

	tx.TxType = TransactionType(data[offset])
	offset++

	tx.From = make([]byte, 32)
	copy(tx.From, data[offset:offset+32])
	offset += 32

	tx.Time = time.Unix(0, int64(binary.BigEndian.Uint64(data[offset:])))
	offset += 8

	tx.Nonce = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	tx.BlockInterval = time.Duration(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	sigLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(sigLen) > len(data) {
		return nil, errors.New("set block time transaction signature truncated")
	}

	tx.Signature = make([]byte, sigLen)
	copy(tx.Signature, data[offset:offset+int(sigLen)])

	return tx, nil
}

func unmarshalSetTxRateTx(data []byte) (*SetTxRateTx, error) {
	// type(1) + pubkey(32) + time(8) + nonce(8) + rate(4) + siglen(4) = 57
	if len(data) < 57 {
		return nil, errors.New("set tx rate transaction too short")
	}

	tx := &SetTxRateTx{}
	offset := 0

	tx.TxType = TransactionType(data[offset])
	offset++

	tx.From = make([]byte, 32)
	copy(tx.From, data[offset:offset+32])
	offset += 32

	tx.Time = time.Unix(0, int64(binary.BigEndian.Uint64(data[offset:])))
	offset += 8

	tx.Nonce = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	tx.TxPerSecond = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	sigLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(sigLen) > len(data) {
		return nil, errors.New("set tx rate transaction signature truncated")
	}

	tx.Signature = make([]byte, sigLen)
	copy(tx.Signature, data[offset:offset+int(sigLen)])

	return tx, nil
}
