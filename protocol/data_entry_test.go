package protocol

import (
	"fmt"
	"testing"
)

func TestDataEntry(t *testing.T) {
	de := DataEntry{}

	de.Data = append(de.Data, []byte("test data entry"))
	for i := 0; i < 10; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	expectedHash := "b617fafc3f7144c34a2e892a89841368bb32c92fc810226f95fdcd15e2dc0d37"
	entryHash := fmt.Sprintf("%x", de.Hash())
	if entryHash != expectedHash {
		t.Fatalf("expected hash %v, but received %x", expectedHash, entryHash)
	}

	cost, err := de.Cost()
	if err != nil {
		t.Fatal(err)
	}
	if cost != FeeWriteData.AsInt() {
		t.Fatalf("expected a cost of 10 credits, but computed %d", cost)
	}

	de.Data = nil
	de.Data = append(de.Data, []byte("test data entry"))
	//now make the data entry larger and compute cost
	for i := 0; i < 100; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	cost, err = de.Cost()
	if err != nil {
		t.Fatal(err)
	}

	//the size is now 987 bytes so it should cost 50 credits
	if cost != 5*FeeWriteData.AsInt() {
		t.Fatalf("expected a cost of 50 credits, but computed %d", cost)
	}

	de.Data = nil
	de.Data = append(de.Data, []byte("test data entry"))
	//now let's blow up the size of the entry to > 10kB to make sure it fails.
	for i := 0; i < 1000; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	//now the size of the entry is 10878 bytes, so the cost should fail.
	cost, err = de.Cost()
	if err == nil {
		t.Fatalf("expected failure on data to large, but it passed and returned a cost of %d", cost)
	}
}
