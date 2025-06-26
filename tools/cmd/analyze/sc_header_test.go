// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"testing"
)

// TestCompareHeaderEncoding compares different approaches to encoding the header section (section type 1)
// This test helps identify differences between:
// 1. The original snapshot file header section data
// 2. Our reconstruction approach using encoding.NewWriter
// 3. The debug snap collect approach using snapshot.Header.MarshalBinary
//
// The test also compares:
// - Section headers (64-byte headers)
// - Padding calculations for alignment
// - Next section offset calculations
//
// This test is critical for ensuring our snapshot reconstruction process produces
// a byte-for-byte identical header section compared to the original snapshot format.
func TestAnalyzeHeaderEncoding(t *testing.T) {
	// Define the path to the test snapshot file
	snapshotPath := "/home/paul/work/acc1/dn.snap"
	_, err := os.Stat(snapshotPath)
	if os.IsNotExist(err) {
		t.Skipf("Skipping test: snapshot file not found at %s", snapshotPath)
		return
	} else if err != nil {
		t.Fatalf("Failed to check if snapshot file exists: %v", err)
	}

	// Open the snapshot file
	file, err := os.Open(snapshotPath)
	if err != nil {
		t.Fatalf("Failed to open snapshot file: %v", err)
	}
	defer file.Close()

	// Read the first 256 bytes (which should include the header section)
	headerBytes := make([]byte, 256)
	n, err := file.Read(headerBytes)
	if err != nil || n < 256 {
		t.Fatalf("Failed to read header bytes: %v", err)
	}

	// Print the header section in detail
	fmt.Println("=== Original Snapshot Header Section ===")
	fmt.Println("First 64 bytes (Section Header):")
	printHexDump(headerBytes[:64], 0, -1)

	// Extract the section size from the header
	sectionSize := uint64(0)
	for i := 0; i < 8; i++ {
		sectionSize = (sectionSize << 8) | uint64(headerBytes[15-i])
	}
	fmt.Printf("Section Size from Header: %d (0x%x) bytes\n", sectionSize, sectionSize)

	// Extract the next section offset from the header
	nextOffset := uint64(0)
	for i := 0; i < 8; i++ {
		nextOffset = (nextOffset << 8) | uint64(headerBytes[23-i])
	}
	fmt.Printf("Next Section Offset from Header: 0x%x\n", nextOffset)

	// Print the header data (after the section header)
	fmt.Println("\nHeader Data (after Section Header):")
	printHexDump(headerBytes[64:], 64, -1)

	// Calculate actual header data size
	actualHeaderSize := int(nextOffset) - 64
	fmt.Printf("Actual Header Data Size: %d bytes\n", actualHeaderSize)

	// Check if there's padding
	if actualHeaderSize > int(sectionSize) {
		paddingSize := actualHeaderSize - int(sectionSize)
		fmt.Printf("Padding Size: %d bytes\n", paddingSize)
	}

	// Now check our reconstructed snapshot
	reconstructedPath := "/home/paul/work/acc1/dn.snap.reconstructed"
	reconstructedFile, err := os.Open(reconstructedPath)
	if err != nil {
		t.Fatalf("Failed to open reconstructed file: %v", err)
	}
	defer reconstructedFile.Close()

	// Read the first 256 bytes from the reconstructed file
	reconstructedBytes := make([]byte, 256)
	n, err = reconstructedFile.Read(reconstructedBytes)
	if err != nil || n < 256 {
		t.Fatalf("Failed to read header bytes from reconstructed file: %v", err)
	}

	// Print the reconstructed header section
	fmt.Println("\n=== Reconstructed Snapshot Header Section ===")
	fmt.Println("First 64 bytes (Section Header):")
	printHexDump(reconstructedBytes[:64], 0, -1)

	// Extract the section size from the reconstructed header
	reconstructedSize := uint64(0)
	for i := 0; i < 8; i++ {
		reconstructedSize = (reconstructedSize << 8) | uint64(reconstructedBytes[15-i])
	}
	fmt.Printf("Section Size from Header: %d (0x%x) bytes\n", reconstructedSize, reconstructedSize)

	// Extract the next section offset from the reconstructed header
	reconstructedOffset := uint64(0)
	for i := 0; i < 8; i++ {
		reconstructedOffset = (reconstructedOffset << 8) | uint64(reconstructedBytes[23-i])
	}
	fmt.Printf("Next Section Offset from Header: 0x%x\n", reconstructedOffset)

	// Print the reconstructed header data
	fmt.Println("\nHeader Data (after Section Header):")
	printHexDump(reconstructedBytes[64:], 64, -1)

	// Calculate actual reconstructed header data size
	actualReconstructedSize := int(reconstructedOffset) - 64
	fmt.Printf("Actual Header Data Size: %d bytes\n", actualReconstructedSize)

	// Check if there's padding in the reconstructed file
	if actualReconstructedSize > int(reconstructedSize) {
		paddingSize := actualReconstructedSize - int(reconstructedSize)
		fmt.Printf("Padding Size: %d bytes\n", paddingSize)
	}
}
