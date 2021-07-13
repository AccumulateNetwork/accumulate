// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package factom

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/AccumulateNetwork/accumulated/scratch/factom/varintf"
	//"varintf"
)

// Transaction is a Factoid Transaction which is stored in the FBlock.
//
// Transactions can be between FAAddresses, or from an FAAddress to an
// ECAddress.
//
// Transaction amounts are all delimited in Factoshis.
type Transaction struct {
	// ID the sha256 hash of the binary Transaction ledger, which includes
	// the header, timestamp salt, and all inputs and outputs.
	ID *Bytes32

	// Timestamp is established by the FBlock. It is only populated if the
	// Transaction was unmarshaled from within an FBlock
	Timestamp time.Time

	// TimestampSalt is accurate to the millisecond
	TimestampSalt time.Time

	// Totals all denoted in factoshis. Populated by UnmarshalBinary.
	TotalIn, TotalFCTOut, TotalECOut, TotalBurn uint64

	FCTInputs  []AddressAmount
	FCTOutputs []AddressAmount
	ECOutputs  []AddressAmount
	Signatures []RCDSignature

	// marshalBinaryCache is the binary data of the DBlock. It is cached by
	// UnmarshalBinary so it can be re-used by MarshalBinary.
	marshalBinaryCache []byte
}

// ClearMarshalBinaryCache discards the cached MarshalBinary data.
//
// Subsequent calls to MarshalBinary will re-construct the data from the fields
// of the DBlock.
func (tx *Transaction) ClearMarshalBinaryCache() {
	tx.marshalBinaryCache = nil
}

// AddressAmount relates a 32 byte Address payload and an Amount.
//
// Bytes are used for the Address for improved unmarshaling efficiency of
// Transactions.
type AddressAmount struct {
	Address Bytes
	Amount  uint64
}

// AddressBytes32 converts the Address to Bytes32.
func (adr AddressAmount) AddressBytes32() Bytes32 {
	var b32 Bytes32
	copy(b32[:], adr.Address)
	return b32
}

// FAAddress converts the Address to an FAAddress.
func (adr AddressAmount) FAAddress() FAAddress {
	return FAAddress(adr.AddressBytes32())
}

// ECAddress converts the Address to an ECAddress.
func (adr AddressAmount) ECAddress() ECAddress {
	return ECAddress(adr.AddressBytes32())
}

// RCDSignature relates an RCD and a corresponding Signature of a signed
// Transaction.
type RCDSignature struct {
	RCD       RCD
	Signature Bytes
}

// ValidateType01 validates the RCD and Signature against the msg, which is
// normally the Transaction Binary Ledger.
func (rs RCDSignature) ValidateType01(msg []byte) error {
	return rs.RCD.ValidateType01(rs.Signature, msg)
}

// UnmarshalBinary unmarshals the variable length RCD and Signature block into
// rs.
//
// Use Len to determine how much data was read.
func (rs *RCDSignature) UnmarshalBinary(data []byte) error {
	if err := rs.RCD.UnmarshalBinary(data); err != nil {
		return err
	}
	data = data[len(rs.RCD):] // Skip past RCD
	if len(data) < rs.RCD.SignatureBlockSize() {
		return fmt.Errorf("%v: invalid signature block size", rs.RCD.Type())
	}
	rs.Signature = data[:rs.RCD.SignatureBlockSize()]
	return nil
}

// MarshalBinary concatenates the RCD and the signature.
func (rs RCDSignature) MarshalBinary() ([]byte, error) {
	return append(rs.RCD, rs.Signature...), nil
}

// Len is the total size of the RCD and Signature.
func (rs RCDSignature) Len() int {
	return len(rs.RCD) + len(rs.Signature)
}

// IsPopulated returns true if s has already been successfully populated by a
// call to Get. IsPopulated returns false if s.SignatureBlock or
// s.ReedeemCondition are nil
func (rs RCDSignature) IsPopulated() bool {
	return rs.RCD != nil && rs.Signature != nil
}

// IsPopulated returns true if tx has already been successfully populated by a
// call to Get. IsPopulated returns false if tx.FCTInputs is empty, or if
// tx.Signatures is not equal in length to the tx.FCTInputs, or if
// tx.TimestampSalt is zero.
func (tx Transaction) IsPopulated() bool {
	return len(tx.FCTInputs) > 0 &&
		len(tx.Signatures) == len(tx.FCTInputs) &&
		!tx.TimestampSalt.IsZero()
}

const (
	// TransactionVersion is the magic number Version byte in all
	// Transactions.
	TransactionVersion byte = 0x02

	// TransactionHeaderSize is the size of a Binary Transaction Header.
	TransactionHeaderSize = 1 + // Version byte
		6 + // Timestamp salt
		1 + // Input count
		1 + // Output count
		1 // EC output count
	TransactionMinTotalSize = TransactionHeaderSize +
		32 + 1 + // 1 Input
		32 + 1 + // 1 Output
		33 + 32 // RCD01 and Signature
)

// UnmarshalBinary unmarshals and validates the first Transaction from data,
// which may include subsequent Transactions. If no error is returned, the
// Transaction is valid, including all RCDs and signatures.
//
// Use MarshalBinaryLen to efficiently determine the number of bytes read from
// data.
func (tx *Transaction) UnmarshalBinary(data []byte) error {
	// Parse header
	if len(data) < TransactionHeaderSize {
		return fmt.Errorf("insufficient length")
	}

	// Only Version 0x02 is supported.
	if data[0] != TransactionVersion {
		return fmt.Errorf("invalid version")
	}

	i := 1

	msTsSalt := getInt48BE(data[i:])
	tx.TimestampSalt = time.Unix(0, msTsSalt*1e6)
	i += 6

	fctInputCount := uint(data[i])
	i++
	fctOutputCount := uint(data[i])
	i++
	ecOutputCount := uint(data[i])
	i++

	adrs := make([]AddressAmount, fctInputCount+fctOutputCount+ecOutputCount)

	var totalIn uint64
	var totalOut uint64
	var totalECOut uint64
	for j := range adrs {
		amount, size := varintf.Decode(data[i:])
		if size == 0 {
			return fmt.Errorf("insufficient length")
		}
		if size < 0 {
			return fmt.Errorf("invalid amount")
		}
		i += size

		if len(data[i:]) < 32 {
			return fmt.Errorf("insufficient length")
		}

		adr := &adrs[j]
		adr.Amount = amount
		adr.Address = data[i : i+32]
		i += 32

		if uint(j) < fctInputCount {
			totalIn += amount
		} else {
			totalOut += amount
			if uint(j) >= fctInputCount+fctOutputCount {
				totalECOut += amount
			}
		}
	}

	isCoinbaseTx := fctInputCount == 0 && ecOutputCount == 0
	if !isCoinbaseTx && totalOut > totalIn {
		return fmt.Errorf("outputs exceed inputs")
	}

	tx.TotalIn = totalIn
	tx.TotalFCTOut = totalOut - totalECOut
	tx.TotalECOut = totalECOut
	tx.TotalBurn = totalIn - totalOut

	ledger := data[:i]

	tx.FCTInputs = adrs[:fctInputCount]
	adrs = adrs[fctInputCount:]
	tx.FCTOutputs = adrs[:fctOutputCount]
	tx.ECOutputs = adrs[fctOutputCount:]

	tx.Signatures = make([]RCDSignature, fctInputCount)

	for j := range tx.Signatures {
		rcdSig := &tx.Signatures[j]
		if err := rcdSig.UnmarshalBinary(data[i:]); err != nil {
			return err
		}
		i += rcdSig.Len()

		// Validate RCD
		rcdHash := rcdSig.RCD.Hash()
		if bytes.Compare(tx.FCTInputs[j].Address, rcdHash[:]) != 0 {
			return fmt.Errorf("invalid RCD hash")
		}
		if err := rcdSig.ValidateType01(ledger); err != nil {
			return err
		}
	}

	txID := Bytes32(sha256.Sum256(ledger))
	if tx.ID == nil {
		tx.ID = &txID
	} else if *tx.ID != txID {
		return fmt.Errorf("invalid TxID")
	}

	tx.marshalBinaryCache = data[:i]

	return nil
}

// MarshalBinaryLedger marshals the header, inputs, outputs, and EC outputs of
// the Transaction. This is so that the data can be conveniently signed or
// hashed.
func (tx Transaction) MarshalBinaryLedger() ([]byte, error) {
	if !tx.IsPopulated() {
		return nil, fmt.Errorf("not populated")
	}

	if len(tx.FCTInputs) > 256 ||
		len(tx.FCTOutputs) > 256 ||
		len(tx.ECOutputs) > 256 {
		return nil, fmt.Errorf("too many inputs or outputs")
	}

	if len(tx.FCTInputs) != len(tx.Signatures) {
		return nil, fmt.Errorf("number of inputs and signatures differ")
	}

	data := make([]byte, tx.MarshalBinaryLen())

	var i int
	data[i] = TransactionVersion
	i++

	putInt48BE(data[i:], tx.TimestampSalt.Unix()*1e3)
	i += 6

	data[i] = byte(len(tx.FCTInputs))
	i++
	data[i] = byte(len(tx.FCTOutputs))
	i++
	data[i] = byte(len(tx.ECOutputs))
	i++

	for _, adrs := range [][]AddressAmount{
		tx.FCTInputs,
		tx.FCTOutputs, tx.ECOutputs,
	} {
		for _, adr := range adrs {
			size := varintf.BufLen(adr.Amount)
			varintf.Put(data[i:i+size], adr.Amount)
			i += size
			i += copy(data[i:], adr.Address)
		}
	}

	return data[:i], nil
}

// MarshalBinary marshals the Transaction into its binary form. If the
// Transaction was orignally Unmarshaled, then the cached data is re-used, so
// this is efficient. See ClearMarshalBinaryCache.
//
// This assumes that the Transaction has all signatures in place already. See
// Transaction.Sign for signing transactions.
func (tx Transaction) MarshalBinary() ([]byte, error) {
	if tx.marshalBinaryCache != nil {
		return tx.marshalBinaryCache, nil
	}

	data, err := tx.MarshalBinaryLedger()
	if err != nil {
		return nil, err
	}

	for _, rcdSig := range tx.Signatures {
		data = append(data, rcdSig.RCD...)
		data = append(data, rcdSig.Signature...)
	}

	return data, nil
}

// Sign populates the Signatures of the tx with the given signingSet, which
// must correspond to the tx.FCTInputs. The complete binary marshaled
// Transaction is returned.
func (tx *Transaction) Sign(signingSet ...RCDSigner) ([]byte, error) {
	if len(signingSet) != len(tx.FCTInputs) {
		return nil, fmt.Errorf("signingSet size not equal to FCTInputs")
	}

	ledger, err := tx.MarshalBinaryLedger()
	if err != nil {
		return nil, err
	}

	tx.Signatures = make([]RCDSignature, len(tx.FCTInputs))
	data := ledger
	for i, rcdSigner := range signingSet {
		rcdSig := &tx.Signatures[i]
		rcdSig.RCD = rcdSigner.RCD()
		rcdSig.Signature = rcdSigner.Sign(ledger)
		hash := rcdSig.RCD.Hash()
		if bytes.Compare(tx.FCTInputs[i].Address, hash[:]) != 0 {
			return nil, fmt.Errorf("invalid RCD for FCTInput")
		}

		data = append(data, rcdSig.RCD...)
		data = append(data, rcdSig.Signature...)
	}

	txID := Bytes32(sha256.Sum256(ledger))
	tx.ID = &txID

	tx.marshalBinaryCache = data

	return data, nil
}

// MarshalBinaryLen efficiently calculates the full Transaction size. The
// cached binary marshal data is used if populated.
func (tx Transaction) MarshalBinaryLen() int {
	if tx.marshalBinaryCache != nil {
		return len(tx.marshalBinaryCache)
	}

	size := TransactionHeaderSize +
		(len(tx.FCTInputs)+len(tx.FCTOutputs)+len(tx.ECOutputs))*32

	for _, adrs := range [][]AddressAmount{
		tx.FCTInputs,
		tx.FCTOutputs, tx.ECOutputs,
	} {
		for _, adr := range adrs {
			size += varintf.BufLen(adr.Amount)
		}
	}

	for _, rcdSig := range tx.Signatures {
		size += rcdSig.Len()
	}

	return size
}
