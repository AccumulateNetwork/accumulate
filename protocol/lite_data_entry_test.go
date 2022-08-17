package protocol

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

/*
	Entry with EntryHash 1bd5955a72f8696416ac3ca39f7aa6a054e7209aa2f9a5f95d601640b8d047a5
	+----------+-------------------------------------------------------+
	| ChainID                                                          |
	| b36c1c4073305a41edc6353a094329c24ffa54c0a47fb56227a04477bcb78923 |
		+----------+-------------------------------------------------------+
	| ExtID[0] | Tag #1 of entry                                       |
	| ExtID[1] | Tag #2 of entry                                       |
	| ExtID[2] | Tag #3 of entry                                       |
		+----------+-------------------------------------------------------+
	| Content  | This is useful content of the entry. You can save     |
	|          | text, hash, JSON or raw ASCII data here.              |
	+----------+-------------------------------------------------------+
*/

func TestLiteDataEntry(t *testing.T) {

	firstEntry := AccumulateDataEntry{}

	firstEntry.Data = append(firstEntry.Data, []byte{})
	firstEntry.Data = append(firstEntry.Data, []byte("Factom PRO"))
	firstEntry.Data = append(firstEntry.Data, []byte("Tutorial"))

	//create a chainId
	chainId := ComputeLiteDataAccountId(&firstEntry)

	var factomChainId [32]byte
	_, err := hex.Decode(factomChainId[:], []byte("b36c1c4073305a41edc6353a094329c24ffa54c0a47fb56227a04477bcb78923"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(chainId, factomChainId[:]) {
		t.Fatalf("lite token account id doesn't match the expected id")
	}

	lde := NewFactomDataEntry()
	copy(lde.AccountId[:], chainId)
	lde.Data = []byte("This is useful content of the entry. You can save text, hash, JSON or raw ASCII data here.")
	for i := 0; i < 3; i++ {
		lde.ExtIds = append(lde.ExtIds, []byte(fmt.Sprintf("Tag #%d of entry", i+1)))
	}

	expectedHash := "1bd5955a72f8696416ac3ca39f7aa6a054e7209aa2f9a5f95d601640b8d047a5"
	entryHash := lde.Hash()
	entryHashHex := fmt.Sprintf("%x", entryHash)
	if entryHashHex != expectedHash {
		t.Fatalf("expected hash %v, but received %x", expectedHash, entryHash)
	}

	cost, err := DataEntryCost(lde.Wrap())
	if err != nil {
		t.Fatal(err)
	}
	if cost != FeeData.AsUInt64() {
		t.Fatalf("expected a cost of 10 credits, but computed %d", cost)
	}

	de := new(AccumulateDataEntry)

	//add test for an empty entry to make sure function behaves as expected.
	accountId := *(*[32]byte)(ComputeLiteDataAccountId(de))
	if strings.Compare(fmt.Sprintf("%x", accountId),
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855") != 0 {
		t.Fatalf("invalid account id from empty lite entry")
	}

	de.Data = append(de.Data, []byte("a cost test"))
	//now make the data entry larger and compute cost
	for i := 0; i < 100; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	cost, err = DataEntryCost(de)
	if err != nil {
		t.Fatal(err)
	}

	//the size is now 987 bytes so it should cost 40 credits
	if cost != 4*FeeData.AsUInt64() {
		t.Fatalf("expected a cost of 40 credits, but computed %d", cost)
	}

	//now let's blow up the size of the entry to > 20kB to make sure it fails.
	for i := 0; i < 2000; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	//now the size of the entry is 20480 bytes, so the cost should fail.
	cost, err = DataEntryCost(de)
	if err == nil {
		t.Fatalf("expected failure on data to large, but it passed and returned a cost of %d", cost)
	}
}
