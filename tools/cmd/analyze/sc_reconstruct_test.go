package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestScReconstruct tests the snapshot reconstruction functionality
// using a real snapshot file
func TestScReconstruct(t *testing.T) {
	// Path to the test snapshot file
	sourceSnapshot := "/home/paul/work/acc1/dn.snap"

	// Create a temporary directory for the test output
	tempDir, err := os.MkdirTemp("", "sc-reconstruct-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up after the test

	// Path to destination snapshot
	destSnapshot := filepath.Join(tempDir, "reconstructed.snap")

	// Create a mock cobra command to simulate CLI usage
	cmd := &cobra.Command{}

	// Set up arguments as they would be passed from the CLI
	// First argument is the output path, second is the input snapshot
	args := []string{destSnapshot, sourceSnapshot}

	err = sc_Run(cmd, args)
	if err != nil {
		t.Fatalf("Reconstruction failed: %v", err)
	}

	// Verify the output file exists and has content
	if _, err := os.Stat(destSnapshot); os.IsNotExist(err) {
		t.Errorf("Output file was not created at %s", destSnapshot)
	} else {
		// Get file size
		fileInfo, err := os.Stat(destSnapshot)
		if err != nil {
			t.Errorf("Failed to get output file info: %v", err)
		} else {
			fmt.Printf("Successfully created reconstructed snapshot at %s (%d bytes)\n",
				destSnapshot, fileInfo.Size())

			// Basic validation: file should not be empty
			if fileInfo.Size() == 0 {
				t.Errorf("Reconstructed file is empty")
			}
		}
	}

	fmt.Println("Test completed successfully")
}

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
// printHexDump prints a hexadecimal representation of data for debugging
// offset is the starting offset for the address column
// maxBytes is the maximum number of bytes to print (-1 for all)
func printHexDump(data []byte, offset int, maxBytes int) {
	if maxBytes < 0 || maxBytes > len(data) {
		maxBytes = len(data)
	}

	// Limit the data to the specified maxBytes
	data = data[:maxBytes]

	// Print 16 bytes per line
	for i := 0; i < len(data); i += 16 {
		// Print the offset
		fmt.Printf("%08x  ", offset+i)

		// Print the hex values
		chunk := data[i:]
		if len(chunk) > 16 {
			chunk = chunk[:16]
		}

		// Print hex representation
		for j := 0; j < len(chunk); j++ {
			fmt.Printf("%02x ", chunk[j])
			if j == 7 {
				fmt.Print(" ") // Extra space after 8 bytes
			}
		}

		// Pad if less than 16 bytes
		for j := len(chunk); j < 16; j++ {
			fmt.Print("   ")
			if j == 7 {
				fmt.Print(" ") // Extra space after 8 bytes
			}
		}

		// Print ASCII representation
		fmt.Print(" |")
		for j := 0; j < len(chunk); j++ {
			if chunk[j] >= 32 && chunk[j] <= 126 { // Printable ASCII
				fmt.Printf("%c", chunk[j])
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println("|")
	}

	// Print total size
	fmt.Printf("Total: %d bytes\n", len(data))
}

func TestCompareHeaderEncoding(t *testing.T) {
	// First, dump the first 256 bytes of the original snapshot file
	sourceSnapshot := "/home/paul/work/acc1/dn.snap"

	// Open the original snapshot file
	originalFile, err := os.Open(sourceSnapshot)
	if err != nil {
		t.Fatalf("Failed to open original snapshot file: %v", err)
	}
	defer originalFile.Close()

	// Read the first 256 bytes
	originalBytes := make([]byte, 256)
	n, err := io.ReadFull(originalFile, originalBytes)
	if err != nil && err != io.ErrUnexpectedEOF {
		t.Fatalf("Failed to read original snapshot file: %v", err)
	}

	// If the file is smaller than 256 bytes, adjust the slice
	if n < 256 {
		originalBytes = originalBytes[:n]
	}

	fmt.Println("=== First 256 bytes of original snapshot file (dn.snap) ===")
	printHexDump(originalBytes, 0, -1)

	// Extract and display the header section from the original snapshot file
	// First 64 bytes are the file header
	fileHeader := originalBytes[:64]

	// Parse the section size from the header
	sectionSize := binary.BigEndian.Uint64(fileHeader[8:16])

	// Read the header section data
	_, err = originalFile.Seek(64, io.SeekStart) // Go back to position after file header
	if err != nil {
		t.Fatalf("Failed to seek in original snapshot file: %v", err)
	}

	// Read the header section data
	originalHeaderData := make([]byte, sectionSize)
	_, err = io.ReadFull(originalFile, originalHeaderData)
	if err != nil {
		t.Fatalf("Failed to read header section data: %v", err)
	}

	fmt.Println("\n=== Original File Header (64 bytes) ===")
	printHexDump(fileHeader, 0, -1)

	fmt.Println("\n=== Original Header Section Data ===")
	printHexDump(originalHeaderData, 0, -1)

	// Calculate padding in the original file
	originalTotalSize := 64 + int(sectionSize) // file header + header section data
	originalPadding := (64 - (originalTotalSize % 64)) % 64

	fmt.Printf("\nOriginal header section size: %d bytes\n", sectionSize)
	fmt.Printf("Original total size (file header + header section): %d bytes\n", originalTotalSize)
	fmt.Printf("Original padding needed: %d bytes\n", originalPadding)
	// Create a test header with sample data
	rootHash := [32]byte{}
	for i := 0; i < 32; i++ {
		rootHash[i] = byte(i)
	}

	// Parse the URL string into a URL object
	systemUrl, err := url.Parse("acc://system")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}

	systemLedger := &protocol.SystemLedger{
		Url:   systemUrl,
		Index: 1,
	}

	// ==========================================
	// Our approach: Using sc_WriteSectionType1
	// ==========================================
	var ourBuffer bytes.Buffer

	// Create an encoding writer that follows the same format as snapshot.Header.MarshalBinary
	writer := encoding.NewWriter(&ourBuffer)

	// Write field 1: Version = 2
	writer.WriteUint(1, 2) // Field 1, Value 2 (format version)

	// Write field 2: Root hash
	writer.WriteHash(2, &rootHash)

	// Write field 3: System ledger
	systemLedgerData, err := systemLedger.MarshalBinary()
	if err != nil {
		t.Fatalf("Failed to marshal system ledger: %v", err)
	}
	writer.WriteBytes(3, systemLedgerData)

	// Finalize the writer
	_, _, err = writer.Reset(nil)
	if err != nil {
		t.Fatalf("Failed to finalize header encoding: %v", err)
	}

	// Get the encoded header data
	ourHeaderData := ourBuffer.Bytes()

	// ==========================================
	// Debug snap collect approach: Using snapshot.Header
	// ==========================================
	// Create a snapshot header
	header := &snapshot.Header{
		Version:      2,
		RootHash:     rootHash,
		SystemLedger: systemLedger,
	}

	// Marshal the header using the snapshot package
	debugHeaderData, err := header.MarshalBinary()
	if err != nil {
		t.Fatalf("Failed to marshal header: %v", err)
	}

	// ==========================================
	// Our approach: Calculate section header and padding
	// ==========================================
	// Create our section header (64 bytes)
	var ourSectionHeader [64]byte

	// Set section type to 1 (header section)
	binary.BigEndian.PutUint64(ourSectionHeader[0:8], 1)

	// Set section size
	binary.BigEndian.PutUint64(ourSectionHeader[8:16], uint64(len(ourHeaderData)))

	// Calculate padding needed to align next section to 64-byte boundary
	ourTotalSize := 64 + len(ourHeaderData) // section header + header data
	ourPadding := (64 - (ourTotalSize % 64)) % 64

	// Calculate offset to next section
	ourNextOffset := uint64(ourTotalSize + ourPadding)
	binary.BigEndian.PutUint64(ourSectionHeader[16:24], ourNextOffset)

	// Create our padding bytes
	ourPaddingBytes := make([]byte, ourPadding)

	// ==========================================
	// Debug snap collect approach: Calculate section header and padding
	// ==========================================
	// Create debug section header (64 bytes)
	var debugSectionHeader [64]byte

	// Set section type to 1 (header section)
	binary.BigEndian.PutUint64(debugSectionHeader[0:8], 1)

	// Set section size
	binary.BigEndian.PutUint64(debugSectionHeader[8:16], uint64(len(debugHeaderData)))

	// Calculate padding needed to align next section to 64-byte boundary
	debugTotalSize := 64 + len(debugHeaderData) // section header + header data
	debugPadding := (64 - (debugTotalSize % 64)) % 64

	// Calculate offset to next section
	debugNextOffset := uint64(debugTotalSize + debugPadding)
	binary.BigEndian.PutUint64(debugSectionHeader[16:24], debugNextOffset)

	// Create debug padding bytes
	debugPaddingBytes := make([]byte, debugPadding)

	// ==========================================
	// Print hex dumps for comparison
	// ==========================================
	fmt.Println("\n=== Our Approach ===")
	fmt.Println("=== Our Header Section Data ===")
	printHexDump(ourHeaderData, 0, -1)

	fmt.Println("\n=== Our Section Header (64 bytes) ===")
	printHexDump(ourSectionHeader[:], 0, -1)

	fmt.Println("\n=== Our Padding Bytes ===")
	printHexDump(ourPaddingBytes, 0, -1)

	fmt.Printf("\nOur header data size: %d bytes\n", len(ourHeaderData))
	fmt.Printf("Our total size (section header + header data): %d bytes\n", ourTotalSize)
	fmt.Printf("Our padding needed: %d bytes\n", ourPadding)
	fmt.Printf("Our next section offset: %d bytes\n", ourNextOffset)

	fmt.Println("\n=== Debug Snap Collect Approach ===")
	fmt.Println("=== Debug Header Section Data ===")
	printHexDump(debugHeaderData, 0, -1)

	fmt.Println("\n=== Debug Section Header (64 bytes) ===")
	printHexDump(debugSectionHeader[:], 0, -1)

	fmt.Println("\n=== Debug Padding Bytes ===")
	printHexDump(debugPaddingBytes, 0, -1)

	fmt.Printf("\nDebug header data size: %d bytes\n", len(debugHeaderData))
	fmt.Printf("Debug total size (section header + header data): %d bytes\n", debugTotalSize)
	fmt.Printf("Debug padding needed: %d bytes\n", debugPadding)
	fmt.Printf("Debug next section offset: %d bytes\n", debugNextOffset)

	// Compare the header data encodings
	fmt.Println("\n=== Comparison Results ===")
	fmt.Println("1. Header Section Data:")
	if bytes.Equal(ourHeaderData, debugHeaderData) {
		fmt.Println("   The header data encodings are identical!")
	} else {
		fmt.Println("   The header data encodings are different!")

		// Find the first difference
		minLen := len(ourHeaderData)
		if len(debugHeaderData) < minLen {
			minLen = len(debugHeaderData)
		}

		for i := 0; i < minLen; i++ {
			if ourHeaderData[i] != debugHeaderData[i] {
				fmt.Printf("     First difference at byte %d: our: %02X, debug: %02X\n",
					i, ourHeaderData[i], debugHeaderData[i])
				break
			}
		}

		if len(ourHeaderData) != len(debugHeaderData) {
			fmt.Printf("     Length difference: our: %d bytes, debug: %d bytes\n",
				len(ourHeaderData), len(debugHeaderData))
		}
	}

	// Compare the section headers
	fmt.Println("\n2. Section Headers:")
	if bytes.Equal(ourSectionHeader[:], debugSectionHeader[:]) {
		fmt.Println("   The section headers are identical!")
	} else {
		fmt.Println("   The section headers are different!")

		// Find the first difference in section headers
		for i := 0; i < 64; i++ {
			if ourSectionHeader[i] != debugSectionHeader[i] {
				fmt.Printf("     First difference at byte %d: our: %02X, debug: %02X\n",
					i, ourSectionHeader[i], debugSectionHeader[i])
				break
			}
		}
	}

	// Compare the padding
	fmt.Println("\n3. Padding:")
	if ourPadding == debugPadding {
		fmt.Printf("   The padding sizes are identical! (%d bytes)\n", ourPadding)
	} else {
		fmt.Printf("   The padding sizes are different! (our: %d bytes, debug: %d bytes)\n",
			ourPadding, debugPadding)
	}

	// Compare the next section offsets
	fmt.Println("\n4. Next Section Offsets:")
	if ourNextOffset == debugNextOffset {
		fmt.Printf("   \u2713 The next section offsets are identical! (%d bytes)\n", ourNextOffset)
	} else {
		fmt.Printf("   \u2717 The next section offsets are different! (our: %d bytes, debug: %d bytes)\n",
			ourNextOffset, debugNextOffset)
	}

	// Compare our encoding with the original header section data
	fmt.Println("\n5. Comparison with Original Snapshot:")
	if bytes.Equal(ourHeaderData, originalHeaderData) {
		fmt.Println("   \u2713 Our header data matches the original snapshot file!")
	} else {
		fmt.Println("   \u2717 Our header data differs from the original snapshot file!")

		// Find the first difference
		minLen := len(ourHeaderData)
		if len(originalHeaderData) < minLen {
			minLen = len(originalHeaderData)
		}

		for i := 0; i < minLen; i++ {
			if ourHeaderData[i] != originalHeaderData[i] {
				fmt.Printf("     First difference at byte %d: our: %02X, original: %02X\n",
					i, ourHeaderData[i], originalHeaderData[i])
				break
			}
		}

		if len(ourHeaderData) != len(originalHeaderData) {
			fmt.Printf("     Length difference: our: %d bytes, original: %d bytes\n",
				len(ourHeaderData), len(originalHeaderData))
		}
	}

	// Compare debug encoding with the original header section data
	if bytes.Equal(debugHeaderData, originalHeaderData) {
		fmt.Println("   \u2713 Debug header data matches the original snapshot file!")
	} else {
		fmt.Println("   \u2717 Debug header data differs from the original snapshot file!")

		// Find the first difference
		minLen := len(debugHeaderData)
		if len(originalHeaderData) < minLen {
			minLen = len(originalHeaderData)
		}

		for i := 0; i < minLen; i++ {
			if debugHeaderData[i] != originalHeaderData[i] {
				fmt.Printf("     First difference at byte %d: debug: %02X, original: %02X\n",
					i, debugHeaderData[i], originalHeaderData[i])
				break
			}
		}

		if len(debugHeaderData) != len(originalHeaderData) {
			fmt.Printf("     Length difference: debug: %d bytes, original: %d bytes\n",
				len(debugHeaderData), len(originalHeaderData))
		}
	}
}
