package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: debug-consensus <snapshot-file>")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	rd, err := snapshot.Open(f)
	if err != nil {
		fmt.Printf("Error opening snapshot: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Snapshot file: %s\n", os.Args[1])
	fmt.Printf("Version: %d\n", rd.Header.Version)
	if rd.Header.SystemLedger != nil {
		fmt.Printf("Partition: %s\n", rd.Header.SystemLedger.Url)
		fmt.Printf("Block Index: %d\n", rd.Header.SystemLedger.Index)
	}

	// Find and open consensus section
	consensusReader, err := rd.Open(snapshot.SectionTypeConsensus)
	if err != nil {
		fmt.Printf("Error opening consensus section: %v\n", err)
		os.Exit(1)
	}

	// Read raw bytes
	rawBytes, err := io.ReadAll(consensusReader)
	if err != nil {
		fmt.Printf("Error reading consensus section: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nConsensus section size: %d bytes\n", len(rawBytes))

	// Full hex dump
	fmt.Println("\nFull hex dump:")
	fmt.Println(hex.Dump(rawBytes))

	// Try to unmarshal
	consensusDoc := new(cometbft.GenesisDoc)
	err = consensusDoc.UnmarshalBinary(rawBytes)
	if err != nil {
		fmt.Printf("\nError unmarshaling: %v\n", err)

		// Manual parsing to identify the issue
		fmt.Println("\n--- Manual field parsing ---")
		parseManually(rawBytes)
	} else {
		fmt.Println("\nUnmarshal SUCCESS!")
		fmt.Printf("ChainID: %s\n", consensusDoc.ChainID)
		fmt.Printf("Validators: %d\n", len(consensusDoc.Validators))
		for i, v := range consensusDoc.Validators {
			fmt.Printf("  Validator %d: addr=%x, name=%s, power=%d\n", i, v.Address, v.Name, v.Power)
		}
		if consensusDoc.Block != nil {
			fmt.Printf("Block Height: %d\n", consensusDoc.Block.Height)
		}
	}
}

func parseManually(data []byte) {
	pos := 0
	depth := 0

	for pos < len(data) {
		if pos >= len(data) {
			break
		}

		fieldID := data[pos]
		pos++
		indent := ""
		for i := 0; i < depth; i++ {
			indent += "  "
		}

		if fieldID == 0 {
			depth--
			fmt.Printf("%sEnd marker (offset %d)\n", indent, pos-1)
			continue
		}

		fmt.Printf("%sField ID: %d (offset %d)\n", indent, fieldID, pos-1)

		// Read varint length
		length, bytesRead := readVarint(data[pos:])
		if bytesRead == 0 {
			fmt.Printf("%s  ERROR: invalid varint at offset %d\n", indent, pos)
			return
		}
		pos += bytesRead
		fmt.Printf("%s  Length: %d (varint %d bytes)\n", indent, length, bytesRead)

		if pos+length > len(data) {
			fmt.Printf("%s  ERROR: length %d exceeds remaining data %d at offset %d\n", indent, length, len(data)-pos, pos)
			return
		}

		// Show the data
		endPos := pos + length
		if length > 100 {
			fmt.Printf("%s  Data: %s... (showing first 50 bytes)\n", indent, hex.EncodeToString(data[pos:pos+50]))
		} else {
			fmt.Printf("%s  Data: %s\n", indent, hex.EncodeToString(data[pos:endPos]))
		}

		// Try to interpret
		switch fieldID {
		case 1: // string (chainID)
			str := string(data[pos:endPos])
			fmt.Printf("%s  -> String: %q\n", indent, str)
		case 2: // params (ConsensusParams)
			params := new(cometbft.ConsensusParams)
			if err := params.UnmarshalBinary(data[pos:endPos]); err != nil {
				fmt.Printf("%s  -> Params parse error: %v\n", indent, err)
			} else {
				fmt.Printf("%s  -> Params: MaxBytes=%d, MaxGas=%d\n", indent, params.Block.MaxBytes, params.Block.MaxGas)
			}
		case 3: // validators array - recurse
			fmt.Printf("%s  -> Validators array\n", indent)
			depth++
			// Don't advance pos, let the loop continue to parse nested fields
			continue
		case 4: // block
			block := new(cometbft.Block)
			if err := block.UnmarshalBinary(data[pos:endPos]); err != nil {
				fmt.Printf("%s  -> Block parse error: %v\n", indent, err)
			} else {
				fmt.Printf("%s  -> Block: Height=%d, ChainID=%s\n", indent, block.Height, block.ChainID)
			}
		}

		pos = endPos
	}
}

func readVarint(data []byte) (int, int) {
	var result int
	var shift uint
	for i := 0; i < len(data) && i < 10; i++ {
		b := data[i]
		result |= int(b&0x7f) << shift
		if b&0x80 == 0 {
			return result, i + 1
		}
		shift += 7
	}
	return 0, 0
}
