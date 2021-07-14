package types

import (
	"crypto/sha256"
	"github.com/AccumulateNetwork/SMT/managed"
	"github.com/AccumulateNetwork/SMT/storage"
	"strings"
)

func GetAddressFromIdentityChain(identitychain []byte) uint64 {
	addr,_ := storage.BytesUint64(identitychain)
	return addr
}


func GetAddressFromIdentityName(name string) uint64 {
	namelower := strings.ToLower(name)
	h := sha256.Sum256([]byte(namelower))

	addr := GetAddressFromIdentityChain(h[:])
	return addr
}


func ComputeEntryHashV2(header []byte, data []byte) (*managed.Hash, *managed.Hash, *managed.Hash) {
	hh := managed.Hash(sha256.Sum256(header))
	dh := managed.Hash(sha256.Sum256(data))
	mr := managed.Hash(sha256.Sum256(append(hh.Bytes(),dh.Bytes()...)))
	return &hh, &dh,&mr
}
