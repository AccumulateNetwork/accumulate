package db

import (
	"encoding/binary"
	"fmt"
)

type Version uint64

func NewVersion(commit int, major int, minor int, revision int) Version {
	return Version(commit*0x100000000 + major*0x1000000 + minor*0x10000 + revision)
}

func (v Version) Commit() uint32 {
	return uint32(v >> 0x100000000)
}

func (v Version) Major() uint8 {
	return uint8(v >> 0x1000000)
}

func (v Version) Minor() uint8 {
	return uint8(v >> 0x10000)
}

func (v Version) Revision() uint16 {
	return uint16(v)
}

func (v Version) Bytes() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b[:], uint64(v))
	return b[:]
}

func (v *Version) FromBytes(data []byte) {
	*v = Version(binary.BigEndian.Uint64(data))
}

func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d.%d ", v.Major(), v.Minor(), v.Revision(), v.Commit())
}
