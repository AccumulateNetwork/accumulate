package protocol

import (
	"crypto/sha256"
)

// ComputeLiteDataAccountId will compute the chain id from the first entry in the chain which defines
// the names as part of the external id's
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#chainid
func ComputeLiteDataAccountId(firstEntry *DataEntry) []byte {
	hash := sha256.New()
	for _, id := range firstEntry.ExtIds {
		idSum := sha256.Sum256(id)
		hash.Write(idSum[:])
	}

	c := hash.Sum(nil)
	var chainId [32]byte
	copy(chainId[:], c)
	return chainId[:]
}
