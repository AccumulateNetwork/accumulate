package protocol

import (
	"crypto/sha256"
)

// ComputeLiteDataAccountId will compute the chain id from the first entry in the chain which defines
// the names as part of the external id's
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#chainid
func ComputeLiteDataAccountId(firstEntry DataEntry) []byte {
	var chainId [64]byte
	if firstEntry == nil {
		return chainId[:]
	}
	hash := sha256.New()

	n := len(firstEntry.GetData())
	for i := 1; i < n; i++ {
		idSum := sha256.Sum256(firstEntry.GetData()[i])
		hash.Write(idSum[:])
	}

	c := hash.Sum(nil)
	copy(chainId[:], c)
	return chainId[:32]
}

func (c *LiteDataAccount) AccountId() ([]byte, error) {
	head, err := ParseLiteDataAddress(c.Url)
	if err != nil {
		return nil, err
	}

	//reconstruct full lite chain id
	return head, nil
}
