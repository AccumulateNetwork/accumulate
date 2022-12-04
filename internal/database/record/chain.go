package record

import (
	"crypto/sha256"
	"fmt"
	"io"
	"math/bits"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package record types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package record enums.yml

type Chain interface {
	Record
	Name() string
	Type() ChainType

	Head() Value[*ChainHead]
	States(uint64) Value[*ChainHead]
	ElementIndex([]byte) Value[uint64]
	Element(uint64) Value[[]byte]

	AddHash(hash []byte, unique bool) error
	GetElementIndex(hash []byte) (int64, error)
	GetState(element int64) *ChainHead
	GetAnyState(element int64) (ms *ChainHead, err error)
	Get(element int64) ([]byte, error)
	GetIntermediate(element, height int64) (Left, Right []byte, err error)
	GetRange(begin, end int64) ([][]byte, error)
}

// ChainType is the type of a chain belonging to an account.
type ChainType uint64

func (m *ChainHead) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return m.UnmarshalBinary(data)
}

// Pad makes sure the Pending list ends in a nil.  This avoids some corner cases
// and simplifies adding elements to the merkle tree.  If Pending doesn't have a
// last entry with a nil value, then one is added.
func (m *ChainHead) Pad() {
	PendingLen := len(m.Pending)
	if PendingLen == 0 || m.Pending[PendingLen-1] != nil {
		m.Pending = append(m.Pending, nil)
	}
}

// MarshalBinary
// Encodes the Merkle State so it can be embedded into the Merkle Tree
func (m *ChainHead) MarshalBinary() (MSBytes []byte, err error) {
	MSBytes = append(MSBytes, common.Int64Bytes(m.Count)...) // Count

	// Write out the pending list. Each bit set in Count indicates a Sub Merkle
	// Tree root. For each bit, if the bit is set, record the hash.
	b, err := SparseHashList(m.Pending).MarshalBinary(m.Count)
	if err != nil {
		return nil, err
	}
	MSBytes = append(MSBytes, b...)

	// Write out the hash list (never returns an error)
	b, _ = HashList(m.HashList).MarshalBinary()
	MSBytes = append(MSBytes, b...)
	return MSBytes, nil
}

// UnmarshalBinary
// Take the state of an MSMarshal instance defined by MSBytes, and set all the values
// in this instance of MSMarshal to the state defined by MSBytes.  It is assumed that the
// hash function has been set by the caller.
func (m *ChainHead) UnmarshalBinary(MSBytes []byte) (err error) {
	// Unmarshal the Count
	m.Count, err = encoding.UnmarshalInt(MSBytes)
	if err != nil {
		return err
	}
	MSBytes = MSBytes[len(encoding.MarshalInt(m.Count)):]

	// Unmarshal the pending list
	err = (*SparseHashList)(&m.Pending).UnmarshalBinary(m.Count, MSBytes)
	if err != nil {
		return err
	}
	MSBytes = MSBytes[SparseHashList(m.Pending).BinarySize(m.Count):]

	// Unmarshal the hash list
	err = (*HashList)(&m.HashList).UnmarshalBinary(MSBytes)
	if err != nil {
		return err
	}

	// Make a copy to avoid weird memory bugs
	for i, h := range m.Pending {
		m.Pending[i] = copyHash(h)
	}
	for i, h := range m.HashList {
		m.HashList[i] = copyHash(h)
	}

	return nil
}

// Add a hash to the merkle tree and incrementally build the ChainHead
func (m *ChainHead) Add(hash []byte) {
	hash = copyHash(hash)

	m.HashList = append(m.HashList, hash) // Add the new Hash to the Hash List
	m.Count++                             // Increment our total Hash Count
	m.Pad()                               // Pad Pending with a nil to remove corner cases
	for i, v := range m.Pending {         // Adding the hash is like incrementing a variable
		if v == nil { //                     Look for an empty slot, and
			m.Pending[i] = hash //               And put the Hash there if one is found
			return              //          Mission complete, so return
		}
		hash = combineHashes(v, hash) // If this slot isn't empty, combine the hash with the slot
		m.Pending[i] = nil            //   and carry the result to the next (clearing this one)
	}
}

// Anchor
// Compute the Merkle Directed Acyclic Graph (Merkle DAG or ChainHead) for
// the ChainHead at this point We take any trailing hashes in ChainHead,
// hash them up and combine to create the Merkle Dag Root. Getting the closing
// ListMDRoot is non-destructive, which is useful for some use cases.
//
// Returns a nil if the MerkleSate is empty.
func (m *ChainHead) Anchor() []byte {
	// We go through m.ChainHead and combine any left over hashes in m.ChainHead with each other and the MR.
	// If this is a power of two, that's okay because we will pick up the MR (a balanced ChainHead) and
	// return that, the correct behavior
	var MDRoot []byte
	if m.Count == 0 { // If the count is zero, we have no root.  Return a nil
		return MDRoot
	}
	for _, v := range m.Pending {
		if MDRoot == nil { // Pick up the first hash in m.ChainHead no matter what.
			MDRoot = copyHash(v) // If a nil is assigned over a nil, no harm no foul.  Fewer cases to test this way.
		} else if v != nil { // If MDRoot isn't nil and v isn't nil, combine them.
			MDRoot = combineHashes(v, MDRoot) // v is on the left, MDRoot candidate is on the right, for a new MDRoot
		}
	}
	// Drop out with a MDRoot unless m.ChainHead is zero length, in which case return a nil (correct)
	// If m.ChainHead has the entries for a power of two, then only one hash (the last) is in m.ChainHead, which
	//       is returned (correct)
	// If m.ChainHead has a railing nil, return the trailing entries combined with the last entry in m.ChainHead (correct)
	return MDRoot
}

func copyHash(v []byte) []byte {
	u := make([]byte, len(v))
	copy(u, v)
	return u
}

func combineHashes(v, u []byte) []byte {
	h := sha256.New()
	h.Write(v)
	h.Write(u)
	return h.Sum(nil)
}

type SparseHashList [][]byte

func (l SparseHashList) Copy() SparseHashList {
	m := make(SparseHashList, len(l))
	for i, v := range l {
		m[i] = copyHash(v)
	}
	return m
}

func (l SparseHashList) BinarySize(height int64) int {
	var n int
	for i, h := range l {
		// If the bit is not set, skip that entry
		if height&(1<<i) == 0 {
			continue
		}

		n += len(encoding.MarshalBytes(h))
	}
	return n
}

func (l SparseHashList) MarshalBinary(height int64) ([]byte, error) {
	var data []byte

	// For each bit in height
	for i := 0; height > 0; i++ {
		if i >= len(l) {
			return nil, fmt.Errorf("missing hash at [%d]", i)
		}

		// If the bit is set, record the hash, otherwise ignore it (it is nil)
		if height&1 > 0 {
			data = append(data, encoding.MarshalBytes(l[i])...)
		}

		// Shift height so we can check the next bit
		height = height >> 1
	}

	return data, nil
}

func (l *SparseHashList) UnmarshalBinary(height int64, data []byte) error {
	// Count the number of bits required to store the height
	n := bits.Len64(uint64(height))

	// Clear the list and ensure it has sufficient capacity
	*l = append((*l)[:0], make(SparseHashList, n)...)

	for i := range *l {
		// If the bit is not set, skip that entry
		if height&(1<<i) == 0 {
			continue
		}

		// If the bit is set, then extract the next hash
		var err error
		(*l)[i], err = encoding.UnmarshalBytes(data)
		if err != nil {
			return err
		}

		// Advance data by the hash size
		data = data[len(encoding.MarshalBytes((*l)[i])):]
	}

	return nil
}

type HashList [][]byte

func (l HashList) BinarySize() int {
	s := len(encoding.MarshalUint(uint64(len(l))))
	for _, h := range l {
		s += len(encoding.MarshalBytes(h))
	}
	return s
}

func (l HashList) MarshalBinary() ([]byte, error) {
	b := encoding.MarshalUint(uint64(len(l)))
	for _, h := range l {
		b = append(b, encoding.MarshalBytes(h)...)
	}
	return b, nil
}

func (l *HashList) UnmarshalBinary(b []byte) error {
	n, err := encoding.UnmarshalUint(b)
	if err != nil {
		return err
	}
	b = b[len(encoding.MarshalUint(n)):]

	*l = make(HashList, n)
	for i := range *l {
		h, err := encoding.UnmarshalBytes(b)
		if err != nil {
			return err
		}
		(*l)[i] = h
		b = b[len(encoding.MarshalBytes(h)):]
	}

	return nil
}
