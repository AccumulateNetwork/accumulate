package hash

import (
	"crypto/sha256"
	"encoding/binary"
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
	merkle.InitSha256()

	// Add each hash
	for _, h := range h {
		merkle.AddToMerkleTree(h)
	}

	// Return the DAG root
	return merkle.GetMDRoot().Bytes()
}
