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
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
)

// RCDType is the magic number that represents the RCD type. Currently this
// only takes up one byte but techincally it is a varintf. Down the road the
// underlying type may change to allow for longer IDs for RCD Types.
type RCDType byte

// String returns "RCDTypeXX".
func (r RCDType) String() string {
	return fmt.Sprintf("RCDType%02x", byte(r))
}

const (
	// RCDType01 is the magic number identifying the RCD type that requires
	// an ed25519 signature from the given public key.
	RCDType01 RCDType = 0x01
	// RCDType01Size is the total size of the RCD.
	RCDType01Size = 1 + ed25519.PublicKeySize
	// RCDType01SigSize is the size of the expected ed25519 signature.
	RCDType01SigSize = ed25519.SignatureSize
)

// RCDSigner is the interface implemented by types that can generate Redeem
// Condition Datastructures and the corresponding signatures to validate them.
type RCDSigner interface {
	// RCD constructs the RCD.
	RCD() RCD

	// Sign the msg.
	Sign(msg []byte) []byte
}

// RCD is a Redeem Condition Datastructure. It is just a byte slice with
// special meaning.
type RCD []byte

// String returns a hex encoded string of the RCD.
func (rcd RCD) String() string {
	return Bytes(rcd).String()
}

// Type returns the first byte of rcd as an RCDType.
//
// This will panic of len(rcd) < 1.
func (rcd RCD) Type() RCDType {
	return RCDType(rcd[0])
}

// SignatureBlockSize returns the expected size of the signature block this RCD
// requires.
func (rcd RCD) SignatureBlockSize() int {
	switch rcd.Type() {
	case RCDType01:
		return RCDType01SigSize
	default:
		return -1
	}
}

// Hash returns the sha256d(rcd) which is used as the Factoid Address.
func (rcd RCD) Hash() Bytes32 {
	return sha256d(rcd)
}

// FAAddress returns the sha256d(rcd) which is used as the Factoid Address.
func (rcd RCD) FAAddress() FAAddress {
	return FAAddress(rcd.Hash())
}

// sha256(sha256(data))
func sha256d(data []byte) [sha256.Size]byte {
	hash := sha256.Sum256(data)
	return sha256.Sum256(hash[:])
}

// Validate verifies the RCD against the given sig and msg. Only RCDTypes in
// the whitelist are permitted. If no whitelist is provided all supported
// RCDTypes are allowed.
func (rcd RCD) Validate(sig, msg []byte, whitelist ...RCDType) error {
	if len(rcd) < 1 {
		return fmt.Errorf("invalid RCD size")
	}

	if len(whitelist) > 0 {
		whitemap := make(map[RCDType]struct{}, len(whitelist))
		for _, rcdType := range whitelist {
			whitemap[rcdType] = struct{}{}
		}
		if _, ok := whitemap[rcd.Type()]; !ok {
			return fmt.Errorf("%v not accepted", rcd.Type())
		}
	}

	switch rcd.Type() {
	case RCDType01:
		return rcd.ValidateType01(sig, msg)
	default:
		return fmt.Errorf("unsupported RCD")
	}
}

// ValidateType01 validates the RCD against sig and msg and ensures that the
// RCD is RCDType01.
func (rcd RCD) ValidateType01(sig, msg []byte) error {
	if len(rcd) != RCDType01Size {
		return fmt.Errorf("invalid RCD size")
	}
	if rcd.Type() != RCDType01 {
		return fmt.Errorf("invalid RCD type")
	}
	if len(sig) != RCDType01SigSize {
		return fmt.Errorf("invalid signature size")
	}

	pubKey := []byte(rcd[1:RCDType01Size])
	if !ed25519.Verify(pubKey, msg, sig) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// UnmarshalBinary parses the first RCD out of data. Use len(rcd) to determine
// how many bytes of data were read.
func (rcd *RCD) UnmarshalBinary(data []byte) error {
	if len(data) < 1 {
		return fmt.Errorf("invalid RCD size")
	}

	rcdType := RCD(data).Type()
	switch rcdType {
	case RCDType01:
		if len(data) < RCDType01Size {
			return fmt.Errorf("invalid %v size", rcdType)
		}
		*rcd = RCD(data[:RCDType01Size])
	default:
		return fmt.Errorf("unknown %v", rcdType)
	}

	return nil
}
