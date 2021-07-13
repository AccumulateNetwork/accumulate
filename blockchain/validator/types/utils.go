package types

import (
	"crypto/sha256"
	"github.com/AccumulateNetwork/SMT/smt"
	"strings"
)

func GetAddressFromIdentityChain(identitychain []byte) uint64 {
	addr,_ := smt.BytesUint64(identitychain)
	return addr
}


func GetAddressFromIdentityName(name string) uint64 {
	namelower := strings.ToLower(name)
	h := sha256.Sum256([]byte(namelower))

	addr := GetAddressFromIdentityChain(h[:])
	return addr
}

