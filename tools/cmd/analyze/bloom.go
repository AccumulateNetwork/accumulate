package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

const bSize = 1024 * 1024 * 256
const bMask = bSize - 1
const bCnt = 6
const shiftBy = 4

type Bloom struct {
	table [bSize]byte
	Stats BloomStats
}

type BloomStats struct {
	EntriesAdded int
	PartitionID  string
	BuildTime    time.Duration
}

func NewBloom(partitionID string) *Bloom {
	b := new(Bloom)
	b.Stats.PartitionID = partitionID
	return b
}

func (b *Bloom) set(bit uint64) {
	// Calculate the byte index and bit position
	byteIndex := (bit & (bMask)) >> 3 // Divide by 8 to get byte index
	bitPos := bit & 7                 // Get the bit position within the byte (0-7)

	// Set the bit
	b.table[byteIndex] |= (1 << bitPos)
}

func (b *Bloom) test(bit uint64) bool {
	// Calculate the byte index and bit position
	byteIndex := (bit & (bMask)) >> 3 // Divide by 8 to get byte index
	bitPos := bit & 7                 // Get the bit position within the byte (0-7)

	// Test if the bit is set
	return (b.table[byteIndex] & (1 << bitPos)) != 0
}

func (b *Bloom) Add(hash []byte) {
	if len(hash) != 32 {
		panic("only 32 byte hashes can be added to a bloom filter")
	}

	// Directly set bits for each hash segment
	for i := 0; i < bCnt; i++ {
		// Read 8 bytes as uint64 in big-endian order and set the bit
		bit := binary.BigEndian.Uint64(hash[i*shiftBy : i*shiftBy+8])

		// Calculate the byte index and bit position
		byteIndex := (bit >> 3) & bMask // Divide by 8 to get byte index

		// Set the bit
		b.table[byteIndex] |= (1 << (bit & 7))
	}

	// Update statistics
	b.Stats.EntriesAdded++
}

func (b *Bloom) Test(hash []byte) bool {
	if len(hash) != 32 {
		panic("only 32 byte hashes can be added to a bloom filter")
	}

	// Directly test bits for each hash segment
	for i := 0; i < bCnt; i++ {
		// Read 8 bytes as uint64 in big-endian order
		bit := binary.BigEndian.Uint64(hash[i*shiftBy : i*shiftBy+8])

		// Calculate the byte index and bit position
		byteIndex := (bit >> 3) & bMask // Divide by 8 to get byte index

		// Test if the bit is set
		if (b.table[byteIndex] & (1 << (bit & 7))) == 0 {
			return false // If any bit is not set, return false
		}
	}

	// All bits were set
	return true
}

// EstimateFalsePositiveRate estimates the false positive rate of the bloom filter
// based on the number of entries added and the size of the filter
func (b *Bloom) EstimateFalsePositiveRate() float64 {
	// Calculate the probability of a false positive
	// p = (1 - e^(-kn/m))^k
	// where:
	// k = number of hash functions (bcnt in our case)
	// n = number of entries added
	// m = number of bits in the filter (bsize*8)

	m := float64(bSize * 8)            // Total bits in the filter
	n := float64(b.Stats.EntriesAdded) // Number of entries added
	k := float64(bCnt)                 // Number of hash functions

	// Calculate (1 - e^(-kn/m))^k
	power := -k * n / m
	probability := math.Pow(1.0-math.Exp(power), k)

	return probability
}

// GetStats returns a string with statistics about the bloom filter
func (b *Bloom) GetStats() string {
	falsePositiveRate := b.EstimateFalsePositiveRate() * 100.0 // Convert to percentage
	sizeMB := float64(bSize) / (1024 * 1024)
	totalBits := bSize * 8

	return fmt.Sprintf("Bloom filter for partition %s:\n"+
		"  - Size: %.0f MB (%d bits)\n"+
		"  - Hash functions: %d\n"+
		"  - Entries added: %d\n"+
		"  - Build time: %v\n"+
		"  - Estimated false positive rate: %.8f%%",
		b.Stats.PartitionID,
		sizeMB,
		totalBits,
		bCnt,
		b.Stats.EntriesAdded,
		b.Stats.BuildTime,
		falsePositiveRate)
}
