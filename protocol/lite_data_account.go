package protocol

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
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

func (c *LiteDataAccount) AccountId() ([]byte, error) {
	u, err := url.Parse(c.Header().Url)
	if err != nil {
		return nil, err
	}

	head, err := ParseLiteDataAddress(u)
	if err != nil {
		return nil, err
	}

	//reconstruct full lite chain id
	return append(head, c.Tail...), nil
}
