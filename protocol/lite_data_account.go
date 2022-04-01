package protocol

import (
	"crypto/sha256"
)

// ComputeLiteDataAccountId will compute the chain id from the first entry in the chain which defines
// the names as part of the external id's
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#chainid
func ComputeLiteDataAccountId(firstEntry *DataEntry) []byte {
	var chainId [32]byte
	if firstEntry == nil {
		return chainId[:]
	}
	hash := sha256.New()

	n := len(firstEntry.Data)
	for i := 1; i < n; i++ {
		idSum := sha256.Sum256(firstEntry.Data[i])
		hash.Write(idSum[:])
	}

	c := hash.Sum(nil)
	copy(chainId[:], c)
	return chainId[:]
}

func (c *LiteDataAccount) AccountId() ([]byte, error) {
	head, err := ParseLiteDataAddress(c.Header().Url)
	if err != nil {
		return nil, err
	}

	//reconstruct full lite chain id
	return append(head, c.Tail...), nil
}
