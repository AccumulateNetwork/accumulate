package state

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/AccumulateNetwork/SMT/managed"
	"strings"
)

func GetIdentityChainFromAdi(adi string) *managed.Hash {
	namelower := strings.ToLower(adi)
	h := managed.Hash(sha256.Sum256([]byte(namelower)))

	return &h
}

func GetAddressFromIdentityChain(identitychain []byte) uint64 {
	addr := binary.LittleEndian.Uint64(identitychain)
	return addr
}

func GetAddressFromIdentityName(name string) uint64 {
	addr := GetAddressFromIdentityChain(GetIdentityChainFromAdi(name).Bytes())
	return addr
}

func ComputeEntryHashV2(header []byte, data []byte) (*managed.Hash, *managed.Hash, *managed.Hash) {
	hh := managed.Hash(sha256.Sum256(header))
	dh := managed.Hash(sha256.Sum256(data))
	mr := managed.Hash(sha256.Sum256(append(hh.Bytes(), dh.Bytes()...)))
	return &hh, &dh, &mr
}
