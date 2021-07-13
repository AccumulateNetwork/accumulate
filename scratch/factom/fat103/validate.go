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

package fat103

import (
	"crypto/sha512"
	"fmt"
	"strconv"
	"time"

	"github.com/Factom-Asset-Tokens/factom"
	"github.com/Factom-Asset-Tokens/factom/jsonlen"
)

// Validate validates the structure of the ExtIDs of the factom.Entry to make
// sure that it has a valid timestamp salt and a valid set of RCD/signature
// pairs.
func Validate(e factom.Entry, expected map[factom.Bytes32]struct{},
	whitelist ...factom.RCDType) error {
	if len(expected) == 0 || len(e.ExtIDs) != 2*len(expected)+1 {
		return fmt.Errorf("invalid number of ExtIDs")
	}

	// Validate Timestamp Salt
	timestampSalt := string(e.ExtIDs[0])
	sec, err := strconv.ParseInt(timestampSalt, 10, 64)
	if err != nil {
		return fmt.Errorf("ExtIDs[0]: timestamp salt: %w", err)
	}
	ts := time.Unix(sec, 0)
	diff := e.Timestamp.Sub(ts)
	if -12*time.Hour > diff || diff > 12*time.Hour {
		return fmt.Errorf("ExtIDs[0]: timestamp salt: expired")
	}

	// Compose the signed message data using exactly allocated bytes.
	numRcdSigPairs := len(e.ExtIDs) / 2
	maxRcdSigIDSalt := numRcdSigPairs - 1
	maxRcdSigIDSaltStrLen := jsonlen.Uint64(uint64(maxRcdSigIDSalt))
	timeSalt := e.ExtIDs[0]
	maxMsgLen := maxRcdSigIDSaltStrLen +
		len(timeSalt) +
		len(e.ChainID) +
		len(e.Content)
	msg := make([]byte, maxMsgLen)
	i := maxRcdSigIDSaltStrLen
	i += copy(msg[i:], timeSalt)
	i += copy(msg[i:], e.ChainID[:])
	copy(msg[i:], e.Content)

	rcdSigs := e.ExtIDs[1:]
	for i := 0; i < len(rcdSigs); i += 2 {
		// Prepend the RCD Sig ID Salt to the message data.
		rcdSigID := i / 2
		rcdSigIDSalt := strconv.FormatUint(uint64(rcdSigID), 10)
		// Compute the start index of our msg buffer based on the
		// length of the rcdSigIDSalt.
		start := maxRcdSigIDSaltStrLen - len(rcdSigIDSalt)
		copy(msg[start:], rcdSigIDSalt)

		rcd := factom.RCD(rcdSigs[i])
		sig := rcdSigs[i+1]
		msgHash := sha512.Sum512(msg[start:])

		if err := rcd.Validate(sig, msgHash[:], whitelist...); err != nil {
			return fmt.Errorf("ExtIDs[%v]: %w", i+1, err)
		}

		rcdHash := rcd.Hash()
		if _, ok := expected[rcdHash]; !ok {
			return fmt.Errorf(
				"ExtIDs[%v]: unexpected or duplicate RCD Hash", i+1)
		}
		delete(expected, rcdHash)
	}

	return nil
}
