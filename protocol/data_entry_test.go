package protocol

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataEntry(t *testing.T) {
	de := AccumulateDataEntry{}

	de.Data = append(de.Data, []byte("test data entry"))
	for i := 0; i < 10; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	expectedHash := "29f613df53d1e38dcfea87b2582985cae5265699ef8fc5c500b0bee8f32974ed"
	entryHash := fmt.Sprintf("%x", de.Hash())
	if entryHash != expectedHash {
		t.Fatalf("expected hash %v, but received %v", expectedHash, entryHash)
	}

	cost, err := DataEntryCost(&de)
	if err != nil {
		t.Fatal(err)
	}
	if cost != FeeData.AsUInt64() {
		t.Fatalf("expected a cost of 10 credits, but computed %d", cost)
	}

	//now make the data entry larger and compute cost
	for i := 0; i < 100; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	cost, err = DataEntryCost(&de)
	if err != nil {
		t.Fatal(err)
	}

	//the size is now 987 bytes so it should cost 50 credits
	if cost != 5*FeeData.AsUInt64() {
		t.Fatalf("expected a cost of 50 credits, but computed %d", cost)
	}

	//now let's blow up the size of the entry to > 20kB to make sure it fails.
	for i := 0; i < 2000; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	//now the size of the entry is 20480 bytes, so the cost should fail.
	cost, err = DataEntryCost(&de)
	if err == nil {
		t.Fatalf("expected failure on data to large, but it passed and returned a cost of %d", cost)
	}
}

func TestDataEntryEmpty(t *testing.T) {
	de := new(AccumulateDataEntry)
	de.Data = [][]byte{nil, []byte("foo")}

	marshalled, err := de.MarshalBinary()
	require.NoError(t, err)

	de2 := new(AccumulateDataEntry)
	require.NoError(t, de2.UnmarshalBinary(marshalled))
	require.True(t, de.Equal(de2))
}
