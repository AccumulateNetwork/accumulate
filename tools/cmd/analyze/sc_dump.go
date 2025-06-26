// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
)

// dumpFileHeader reads and prints the first 64 bytes of a file in hex format
func dumpFileHeader(filePath string) error {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read the first 64 bytes
	header := make([]byte, 64)
	n, err := file.Read(header)
	if err != nil {
		return fmt.Errorf("failed to read file header: %w", err)
	}
	if n < 64 {
		header = header[:n]
	}

	// Print the header in hex format
	fmt.Printf("File: %s\n", filePath)
	fmt.Printf("Header (first 64 bytes):\n")
	for i := 0; i < len(header); i += 16 {
		// Print offset
		fmt.Printf("%08x  ", i)

		// Print hex values
		for j := 0; j < 16; j++ {
			if i+j < len(header) {
				fmt.Printf("%02x ", header[i+j])
			} else {
				fmt.Printf("   ")
			}
			if j == 7 {
				fmt.Printf(" ")
			}
		}

		// Print ASCII representation
		fmt.Printf(" |")
		for j := 0; j < 16; j++ {
			if i+j < len(header) {
				if header[i+j] >= 32 && header[i+j] <= 126 {
					fmt.Printf("%c", header[i+j])
				} else {
					fmt.Printf(".")
				}
			} else {
				fmt.Printf(" ")
			}
		}
		fmt.Printf("|\n")
	}
	return nil
}
