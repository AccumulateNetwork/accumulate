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
	"crypto/sha256"
	"crypto/sha512"

	merkle "github.com/AdamSLevy/go-merkle"
)

// ComputeDBlockHeaderHash returns sha256(data[:DBlockHeaderSize]).
func ComputeDBlockHeaderHash(data []byte) Bytes32 {
	return sha256.Sum256(data[:DBlockHeaderSize])
}

// ComputeEBlockHeaderHash returns sha256(data[:EBlockHeaderSize]).
func ComputeEBlockHeaderHash(data []byte) Bytes32 {
	return sha256.Sum256(data[:EBlockHeaderSize])
}

// ComputeFBlockKeyMR returns the Key Merkle root of the FBlock.
func ComputeFBlockKeyMR(elements [][]byte) (Bytes32, error) {
	tree := merkle.NewTreeWithOpts(merkle.TreeOptions{DoubleOddNodes: true, DisableHashLeaves: true})
	if err := tree.Generate(elements, sha256.New()); err != nil {
		return Bytes32{}, err
	}
	root := tree.Root()
	var keyMR Bytes32
	copy(keyMR[:], root.Hash)
	return keyMR, nil
}

// ComputeFBlockBodyMR returns the merkle root of the tree created with
// elements as leaves, where the leaves are hashed.
func ComputeFBlockBodyMR(elements [][]byte) (Bytes32, error) {
	tree := merkle.NewTreeWithOpts(merkle.TreeOptions{DoubleOddNodes: true})
	if err := tree.Generate(elements, sha256.New()); err != nil {
		return Bytes32{}, err
	}
	root := tree.Root()
	var bodyMR Bytes32
	copy(bodyMR[:], root.Hash)
	return bodyMR, nil
}

// ComputeDBlockBodyMR returns the merkle root of the tree created with
// elements as leaves, where the leaves are hashed.
func ComputeDBlockBodyMR(elements [][]byte) (Bytes32, error) {
	tree := merkle.NewTreeWithOpts(merkle.TreeOptions{DoubleOddNodes: true})
	if err := tree.Generate(elements, sha256.New()); err != nil {
		return Bytes32{}, err
	}
	root := tree.Root()
	var bodyMR Bytes32
	copy(bodyMR[:], root.Hash)
	return bodyMR, nil
}

// ComputeEBlockBodyMR returns the merkle root of the tree created with
// elements as leaves, where the leaves are not hashed.
func ComputeEBlockBodyMR(elements [][]byte) (Bytes32, error) {
	tree := merkle.NewTreeWithOpts(merkle.TreeOptions{
		DoubleOddNodes:    true,
		DisableHashLeaves: true})
	if err := tree.Generate(elements, sha256.New()); err != nil {
		return Bytes32{}, err
	}
	root := tree.Root()
	var bodyMR Bytes32
	copy(bodyMR[:], root.Hash)
	return bodyMR, nil
}

// ComputeFullHash returns sha256(data).
func ComputeFullHash(data []byte) Bytes32 {
	return sha256.Sum256(data)
}

// ComputeKeyMR returns sha256(headerHash|bodyMR).
func ComputeKeyMR(headerHash, bodyMR *Bytes32) Bytes32 {
	data := make([]byte, len(headerHash)+len(bodyMR))
	i := copy(data, headerHash[:])
	copy(data[i:], bodyMR[:])
	return sha256.Sum256(data)
}

// ComputeChainID returns the chain ID for a set of NameIDs.
func ComputeChainID(nameIDs []Bytes) Bytes32 {
	hash := sha256.New()
	for _, id := range nameIDs {
		idSum := sha256.Sum256(id)
		hash.Write(idSum[:])
	}
	c := hash.Sum(nil)
	var chainID Bytes32
	copy(chainID[:], c)
	return chainID
}

// ComputeEntryHash returns the Entry hash of data. Entry's are hashed via:
// sha256(sha512(data) + data).
func ComputeEntryHash(data []byte) Bytes32 {
	sum := sha512.Sum512(data)
	saltedSum := make([]byte, len(sum)+len(data))
	i := copy(saltedSum, sum[:])
	copy(saltedSum[i:], data)
	return sha256.Sum256(saltedSum)
}
