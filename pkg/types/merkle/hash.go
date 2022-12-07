package merkle

import (
	"crypto/sha256"
	"fmt"
	"math/bits"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

func copyHash(v []byte) []byte {
	u := make([]byte, len(v))
	copy(u, v)
	return u
}

func combineHashes(a, b []byte) []byte {
	h := sha256.New()
	h.Write(a)
	h.Write(b)
	return h.Sum(nil)
}

// This Stateful Merkle Tree implementation handles 256 bit hashes
type hashHelper []byte

func (h hashHelper) BinarySize() int {
	return len(encoding.MarshalBytes(h))
}

func (h hashHelper) MarshalBinary() ([]byte, error) {
	return encoding.MarshalBytes(h), nil
}

func (h *hashHelper) UnmarhsalBinary(b []byte) error {
	v, err := encoding.UnmarshalBytes(b)
	*h = v
	return err
}

type sparseHashList [][]byte

func (l sparseHashList) Copy() sparseHashList {
	m := make(sparseHashList, len(l))
	for i, v := range l {
		m[i] = copyHash(v)
	}
	return m
}

func (l sparseHashList) BinarySize(height int64) int {
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

func (l sparseHashList) MarshalBinary(height int64) ([]byte, error) {
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

func (l *sparseHashList) UnmarshalBinary(height int64, data []byte) error {
	// Count the number of bits required to store the height
	n := bits.Len64(uint64(height))

	// Clear the list and ensure it has sufficient capacity
	*l = append((*l)[:0], make(sparseHashList, n)...)

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

type hashList [][]byte

func (l hashList) BinarySize() int {
	s := len(encoding.MarshalUint(uint64(len(l))))
	for _, h := range l {
		s += hashHelper(h).BinarySize()
	}
	return s
}

func (l hashList) MarshalBinary() ([]byte, error) {
	b := encoding.MarshalUint(uint64(len(l)))
	for _, h := range l {
		c, _ := hashHelper(h).MarshalBinary()
		b = append(b, c...)
	}
	return b, nil
}

func (l *hashList) UnmarhsalBinary(b []byte) error {
	n, err := encoding.UnmarshalUint(b)
	if err != nil {
		return err
	}
	b = b[len(encoding.MarshalUint(n)):]

	*l = make(hashList, n)
	for i := range *l {
		err = (*hashHelper)(&(*l)[i]).UnmarhsalBinary(b)
		if err != nil {
			return err
		}
		b = b[(*hashHelper)(&(*l)[i]).BinarySize():]
	}

	return nil
}
