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
	"math/rand"
	"strconv"
	"time"

	"github.com/AccumulateNetwork/accumulated/factom"
	"github.com/AccumulateNetwork/accumulated/factom/jsonlen"
)

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// Sign the RCD/Sig ID Salt + Timestamp Salt + Chain ID Salt + Content of the
// factom.Entry and add the RCD + signature pairs for the given addresses to
// the ExtIDs. This clears any existing ExtIDs.
func Sign(e factom.Entry, signingSet ...factom.RCDSigner) factom.Entry {
	// Set the Entry's timestamp so that the signatures will verify against
	// this time salt.
	timeSalt := newTimestampSalt()
	e.Timestamp = time.Now()

	// Compose the signed message data using exactly allocated bytes.
	maxRcdSigIDSaltStrLen := jsonlen.Uint64(uint64(len(signingSet)))
	maxMsgLen := maxRcdSigIDSaltStrLen +
		len(timeSalt) +
		len(e.ChainID) +
		len(e.Content)
	msg := make(factom.Bytes, maxMsgLen)
	i := maxRcdSigIDSaltStrLen
	i += copy(msg[i:], timeSalt[:])
	i += copy(msg[i:], e.ChainID[:])
	copy(msg[i:], e.Content)

	// Generate the ExtIDs for each address in the signing set.
	e.ExtIDs = make([]factom.Bytes, 1, len(signingSet)*2+1)
	e.ExtIDs[0] = timeSalt
	for rcdSigID, a := range signingSet {
		// Compose the RcdSigID salt and prepend it to the message.
		rcdSigIDSalt := strconv.FormatUint(uint64(rcdSigID), 10)
		start := maxRcdSigIDSaltStrLen - len(rcdSigIDSalt)
		copy(msg[start:], rcdSigIDSalt)

		msgHash := sha512.Sum512(msg[start:])

		e.ExtIDs = append(e.ExtIDs, []byte(a.RCD()), a.Sign(msgHash[:]))
	}
	return e
}
func newTimestampSalt() []byte {
	timestamp := time.Now().Add(time.Duration(-rng.Int63n(int64(1 * time.Hour))))
	return []byte(strconv.FormatInt(timestamp.Unix(), 10))
}
