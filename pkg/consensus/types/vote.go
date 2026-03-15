// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

// Vote represents a validator's vote on a header.
// Votes are collected to form certificates.
type Vote struct {
	// HeaderDigest is the digest of the header being voted on.
	HeaderDigest HeaderDigest
	// Round is the round number of the header.
	Round Round
	// Epoch is the epoch number of the header.
	Epoch uint64
	// Author is the public key of the voting validator.
	Author ed25519.PublicKey
	// Signature is the ed25519 signature over the vote content.
	Signature []byte
}

// NewVote creates a new unsigned vote.
func NewVote(headerDigest HeaderDigest, round Round, epoch uint64, author ed25519.PublicKey) *Vote {
	return &Vote{
		HeaderDigest: headerDigest,
		Round:        round,
		Epoch:        epoch,
		Author:       author,
	}
}

// voteContent returns the bytes that are signed/verified.
// Format: HeaderDigest (32 bytes) + Round (8 bytes) + Epoch (8 bytes)
func (v *Vote) voteContent() []byte {
	data := make([]byte, 32+8+8)
	copy(data[0:32], v.HeaderDigest[:])
	binary.BigEndian.PutUint64(data[32:40], uint64(v.Round))
	binary.BigEndian.PutUint64(data[40:48], v.Epoch)
	return data
}

// Sign signs the vote with the given private key.
func (v *Vote) Sign(key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return errors.New("invalid private key size")
	}

	// Verify the public key matches
	pub := key.Public().(ed25519.PublicKey)
	if !bytes.Equal(pub, v.Author) {
		return errors.New("private key does not match vote author")
	}

	content := v.voteContent()
	v.Signature = ed25519.Sign(key, content)
	return nil
}

// Verify verifies the vote signature.
func (v *Vote) Verify() error {
	if len(v.Author) != ed25519.PublicKeySize {
		return errors.New("invalid author public key size")
	}

	if len(v.Signature) != ed25519.SignatureSize {
		return errors.New("invalid signature size")
	}

	content := v.voteContent()
	if !ed25519.Verify(v.Author, content, v.Signature) {
		return errors.New("invalid vote signature")
	}

	return nil
}

// VerifyForHeader verifies that the vote is valid for the given header.
func (v *Vote) VerifyForHeader(header *Header) error {
	// Check that vote parameters match header
	headerDigest := header.Digest()
	if v.HeaderDigest != headerDigest {
		return errors.New("vote header digest does not match header")
	}

	if v.Round != header.Round {
		return errors.New("vote round does not match header round")
	}

	if v.Epoch != header.Epoch {
		return errors.New("vote epoch does not match header epoch")
	}

	// Verify the signature
	return v.Verify()
}

// Marshal serializes the vote to bytes.
func (v *Vote) Marshal() ([]byte, error) {
	// Size: HeaderDigest (32) + Round (8) + Epoch (8) + Author (32) + SigLen (4) + Signature
	size := 32 + 8 + 8 + 32 + 4 + len(v.Signature)
	data := make([]byte, size)
	offset := 0

	// HeaderDigest
	copy(data[offset:], v.HeaderDigest[:])
	offset += 32

	// Round
	binary.BigEndian.PutUint64(data[offset:], uint64(v.Round))
	offset += 8

	// Epoch
	binary.BigEndian.PutUint64(data[offset:], v.Epoch)
	offset += 8

	// Author
	copy(data[offset:], v.Author)
	offset += 32

	// Signature
	binary.BigEndian.PutUint32(data[offset:], uint32(len(v.Signature)))
	offset += 4
	copy(data[offset:], v.Signature)

	return data, nil
}

// UnmarshalVote deserializes a vote from bytes.
func UnmarshalVote(data []byte) (*Vote, error) {
	if len(data) < 32+8+8+32+4 {
		return nil, errors.New("vote data too short")
	}

	v := &Vote{}
	offset := 0

	// HeaderDigest
	copy(v.HeaderDigest[:], data[offset:offset+32])
	offset += 32

	// Round
	v.Round = Round(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	// Epoch
	v.Epoch = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Author
	v.Author = make([]byte, 32)
	copy(v.Author, data[offset:offset+32])
	offset += 32

	// Signature
	sigLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if sigLen > 1024 {
		return nil, errors.New("vote signature too large")
	}

	if offset+int(sigLen) > len(data) {
		return nil, errors.New("vote data truncated: missing signature")
	}

	v.Signature = make([]byte, sigLen)
	copy(v.Signature, data[offset:offset+int(sigLen)])
	offset += int(sigLen)

	if offset != len(data) {
		return nil, errors.New("vote data has trailing bytes")
	}

	return v, nil
}

// Clone creates a deep copy of the vote.
func (v *Vote) Clone() *Vote {
	author := make([]byte, len(v.Author))
	copy(author, v.Author)

	signature := make([]byte, len(v.Signature))
	copy(signature, v.Signature)

	return &Vote{
		HeaderDigest: v.HeaderDigest,
		Round:        v.Round,
		Epoch:        v.Epoch,
		Author:       author,
		Signature:    signature,
	}
}

// VoteDigest returns a digest uniquely identifying this vote.
// Used for deduplication.
func (v *Vote) VoteDigest() [32]byte {
	data := make([]byte, 32+32) // HeaderDigest + Author
	copy(data[0:32], v.HeaderDigest[:])
	copy(data[32:64], v.Author)
	return sha256.Sum256(data)
}
