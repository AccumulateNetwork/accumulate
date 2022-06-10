package hash

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

type Hasher [][]byte

func (h *Hasher) append(v []byte) {
	vh := sha256.Sum256(v)
	*h = append(*h, vh[:])
}

func (h *Hasher) AddHash(v *[32]byte) {
	// Copy to avoid weird memory issues
	vv := *v
	*h = append(*h, vv[:])
}

func (h *Hasher) AddInt(v int64) {
	var b [10]byte
	n := binary.PutVarint(b[:], v)
	h.append(b[:n])
}

func (h *Hasher) AddUint(v uint64) {
	var b [10]byte
	n := binary.PutUvarint(b[:], v)
	h.append(b[:n])
}

func (h *Hasher) AddBool(v bool) {
	var u uint64
	if v {
		u = 1
	}
	h.AddUint(u)
}

func (h *Hasher) AddTime(v time.Time) {
	h.AddInt(v.UTC().Unix())
}

func (h *Hasher) AddBytes(v []byte) {
	h.append(v)
}

func (h *Hasher) AddString(v string) {
	h.append([]byte(v))
}

func (h *Hasher) AddDuration(v time.Duration) {
	sec, ns := encoding.SplitDuration(v)
	var b [10]byte
	n := binary.PutUvarint(b[:], sec)
	var c [10]byte
	m := binary.PutUvarint(c[:], ns)
	h.append(append(b[:n], c[:m]...))
}

func (h *Hasher) AddBigInt(v *big.Int) {
	h.append(v.Bytes())
}

func (h *Hasher) AddUrl(v *url.URL) {
	h.AddString(v.String())
}

func (h *Hasher) AddValue(v interface{ MerkleHash() []byte }) {
	*h = append(*h, v.MerkleHash())
}

func (h *Hasher) AddEnum(v interface{ ID() uint64 }) {
	h.AddUint(v.ID())
}

func (h Hasher) MerkleHash() []byte {
	if len(h) == 0 {
		return make([]byte, 32)
	}

	// Initialize a merkle state
	merkle := managed.MerkleState{}

	// Add each hash
	for _, h := range h {
		merkle.AddToMerkleTree(h)
	}

	// Return the DAG root
	return merkle.GetMDRoot().Bytes()
}

// Receipt returns a receipt for the numbered element. Receipt returns nil if
// either index is out of bounds.
func (h Hasher) Receipt(start, anchor int) *managed.Receipt {
	if start < 0 || start >= len(h) || anchor < 0 || anchor >= len(h) {
		return nil
	}

	// Trivial case
	if len(h) == 1 {
		return &managed.Receipt{
			Start:  h[0],
			End:    h[0],
			Anchor: h[0],
		}
	}

	// Build a merkle state
	anchorState := new(managed.MerkleState)
	for _, h := range h[:anchor+1] {
		anchorState.AddToMerkleTree(h)
	}
	anchorState.PadPending()

	// Initialize the receipt
	r := new(managed.Receipt)
	r.StartIndex = int64(start)
	r.EndIndex = int64(anchor)
	r.Start = h[start]
	r.Anchor = h[start]

	// Build the receipt
	err := r.BuildReceiptWith(h.getIntermediate, managed.Sha256, anchorState)
	if err != nil {
		// The data is static and in memory so there should never be an error
		panic(err)
	}

	return r
}

func Combine(l, r []byte) []byte {
	digest := sha256.New()
	_, _ = digest.Write(l)
	_, _ = digest.Write(r)
	return digest.Sum(nil)
}

// MerkleCascade calculates a Merkle cascade for a hash list. MerkleCascade can
// add hashes to an existing cascade or calculate a new cascade. If maxHeight is
// positive, MerkleCascade will stop at that height.
func MerkleCascade(cascade, hashList [][]byte, maxHeight int64) [][]byte {
	for _, h := range hashList {
		for i := int64(0); maxHeight < 0 || i < maxHeight; i++ {
			// Append at height
			if i == int64(len(cascade)) {
				cascade = append(cascade, h)
				break
			}

			// Fill at height
			v := &(cascade)[i]
			if *v == nil {
				*v = h
				break
			}

			// Combine hashes, carry to next height
			h = Combine(*v, h)
			*v = nil
		}
	}

	return cascade
}

// getIntermediate returns the last two hashes that would be combined to create
// the local Merkle root at the given index and height. The element must be odd.
func (h Hasher) getIntermediate(element, height int64) (managed.Hash, managed.Hash, error) {
	if element%2 != 1 {
		return nil, nil, errors.New("element is not odd")
	}

	// Build a Merkle cascade with the hashes up to element
	cascade := MerkleCascade(nil, h[:element], height)

	// If height is greater than the cascade length, there is no intermediate
	// value
	if int(height) > len(cascade) {
		return nil, nil, fmt.Errorf("no values found at height %d", height)
	}

	if cascade[height-1] == nil {
		// TODO Paul why is this an error?
		return nil, nil, fmt.Errorf("nil at height %d", height)
	}

	// Cascade up to the appropriate height
	right := make([]byte, 32)
	copy(right, h[element])
	for _, v := range cascade[:height-1] {
		if v == nil {
			return nil, nil, fmt.Errorf("should not encounter a nil at height %d", height)
		}
		right = Combine(v, right)
	}

	// Copy the left side
	left := make([]byte, 32)
	copy(left, cascade[height-1])
	return left, right, nil
}
