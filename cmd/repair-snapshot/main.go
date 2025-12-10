// repair-snapshot repairs Dec 1 2025 snapshots that have malformed consensus sections.
// The issue: validators in the consensus section are missing the 0x00 end markers
// that the widget binary encoding requires.
package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: repair-snapshot <input-snapshot> <output-snapshot>")
		fmt.Println("\nRepairs Dec 1 2025 snapshots with malformed consensus sections.")
		fmt.Println("The issue is missing 0x00 end markers in the validators array.")
		os.Exit(1)
	}

	inputPath := os.Args[1]
	outputPath := os.Args[2]

	if err := repairSnapshot(inputPath, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nDone! Repaired snapshot written to:", outputPath)
}

func repairSnapshot(inputPath, outputPath string) error {
	// Open input snapshot
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer f.Close()

	rd, err := snapshot.Open(f)
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}

	fmt.Printf("Input: %s\n", inputPath)
	fmt.Printf("Version: %d\n", rd.Header.Version)
	if rd.Header.SystemLedger != nil {
		fmt.Printf("Partition: %s\n", rd.Header.SystemLedger.Url)
		fmt.Printf("Block Index: %d\n", rd.Header.SystemLedger.Index)
	}

	// Find and read the consensus section
	consensusReader, err := rd.Open(snapshot.SectionTypeConsensus)
	if err != nil {
		return fmt.Errorf("open consensus section: %w", err)
	}

	rawBytes, err := io.ReadAll(consensusReader)
	if err != nil {
		return fmt.Errorf("read consensus section: %w", err)
	}

	fmt.Printf("\nOriginal consensus section: %d bytes\n", len(rawBytes))

	// Try to unmarshal - it should fail
	consensusDoc := new(cometbft.GenesisDoc)
	if err := consensusDoc.UnmarshalBinary(rawBytes); err == nil {
		fmt.Println("Consensus section is already valid, no repair needed")
		return nil
	} else {
		fmt.Printf("Parse error (expected): %v\n", err)
	}

	// Repair the consensus section by parsing and re-encoding
	repairedBytes, err := repairConsensusSection(rawBytes)
	if err != nil {
		return fmt.Errorf("repair consensus: %w", err)
	}

	fmt.Printf("Repaired consensus section: %d bytes\n", len(repairedBytes))

	// Verify the repaired bytes can be parsed - use a FRESH struct!
	fmt.Printf("\nVerifying repairedBytes directly:\n")
	verifyDoc := new(cometbft.GenesisDoc) // Fresh struct
	if err := verifyDoc.UnmarshalBinary(repairedBytes); err != nil {
		return fmt.Errorf("repaired section still invalid: %w", err)
	}
	fmt.Printf("  Validators in repaired bytes: %d\n", len(verifyDoc.Validators))

	fmt.Printf("\nRepaired consensus section verified!")
	fmt.Printf("\n  ChainID: %s\n", verifyDoc.ChainID)
	fmt.Printf("  Validators: %d\n", len(verifyDoc.Validators))
	for i, v := range verifyDoc.Validators {
		fmt.Printf("    %d: %s (power=%d)\n", i, v.Name, v.Power)
	}
	if verifyDoc.Block != nil {
		fmt.Printf("  Block Height: %d\n", verifyDoc.Block.Height)
	}

	// Create the output file
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	// Create a new snapshot writer
	wr, err := snapshot.Create(out)
	if err != nil {
		return fmt.Errorf("create snapshot writer: %w", err)
	}

	// Write header (same as original)
	if err := wr.WriteHeader(rd.Header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	fmt.Println("\nWritten: header")

	// Copy all sections, replacing the consensus section
	for _, section := range rd.Sections {
		if section.Type() == snapshot.SectionTypeHeader {
			continue // Already written
		}

		sw, err := wr.OpenRaw(section.Type())
		if err != nil {
			return fmt.Errorf("open section %v: %w", section.Type(), err)
		}

		if section.Type() == snapshot.SectionTypeConsensus {
			// Write repaired consensus
			if _, err := sw.Write(repairedBytes); err != nil {
				return fmt.Errorf("write repaired consensus: %w", err)
			}
			fmt.Printf("Written: consensus (repaired, %d bytes)\n", len(repairedBytes))
		} else {
			// Copy original section
			sr, err := section.Open()
			if err != nil {
				return fmt.Errorf("open section %v for reading: %w", section.Type(), err)
			}

			n, err := io.Copy(sw, sr)
			if err != nil {
				return fmt.Errorf("copy section %v: %w", section.Type(), err)
			}
			fmt.Printf("Written: %v (%d bytes)\n", section.Type(), n)
		}

		if err := sw.Close(); err != nil {
			return fmt.Errorf("close section %v: %w", section.Type(), err)
		}
	}

	return nil
}

// repairConsensusSection fixes the missing end markers in the validators array
func repairConsensusSection(data []byte) ([]byte, error) {
	// Parse the fields manually and rebuild with proper encoding
	doc, err := parseCorruptedConsensus(data)
	if err != nil {
		return nil, fmt.Errorf("parse corrupted: %w", err)
	}

	// Debug: Print what we parsed
	fmt.Printf("\n  Parsed GenesisDoc before marshal:\n")
	fmt.Printf("    ChainID: %s\n", doc.ChainID)
	fmt.Printf("    Params: %v\n", doc.Params != nil)
	fmt.Printf("    Validators: %d\n", len(doc.Validators))
	for i, v := range doc.Validators {
		fmt.Printf("      %d: name=%s, power=%d, addr=%x, type=%v\n", i, v.Name, v.Power, v.Address, v.Type)
	}
	fmt.Printf("    Block: %v\n", doc.Block != nil)

	// Don't include block - it causes issues and isn't needed for genesis
	doc.Block = nil

	// Re-encode with proper format
	result, err := doc.MarshalBinary()
	if err != nil {
		return nil, err
	}

	// Verify
	testDoc := new(cometbft.GenesisDoc)
	if err := testDoc.UnmarshalBinary(result); err != nil {
		return nil, fmt.Errorf("verify failed: %w", err)
	}
	fmt.Printf("\n  Verified after marshal:\n")
	fmt.Printf("    Validators: %d\n", len(testDoc.Validators))
	for i, v := range testDoc.Validators {
		fmt.Printf("      %d: name=%s, power=%d\n", i, v.Name, v.Power)
	}

	// Hex dump of marshalled bytes
	fmt.Printf("\n  Marshalled bytes (%d bytes):\n", len(result))
	fmt.Printf("    %x\n", result)

	return result, nil
}

// parseCorruptedConsensus parses the corrupted consensus section
func parseCorruptedConsensus(data []byte) (*cometbft.GenesisDoc, error) {
	doc := new(cometbft.GenesisDoc)
	pos := 0

	for pos < len(data) {
		if pos >= len(data) {
			break
		}

		fieldID := data[pos]
		pos++

		if fieldID == 0 {
			break // End of object
		}

		switch fieldID {
		case 1: // chainID (string)
			length, bytesRead := readVarint(data[pos:])
			if bytesRead == 0 {
				return nil, fmt.Errorf("invalid chainID length at offset %d", pos)
			}
			pos += bytesRead
			if pos+length > len(data) {
				return nil, fmt.Errorf("chainID truncated at offset %d", pos)
			}
			doc.ChainID = string(data[pos : pos+length])
			pos += length
			fmt.Printf("  Parsed chainID: %s\n", doc.ChainID)

		case 2: // params (ConsensusParams - protobuf)
			length, bytesRead := readVarint(data[pos:])
			if bytesRead == 0 {
				return nil, fmt.Errorf("invalid params length at offset %d", pos)
			}
			pos += bytesRead
			if pos+length > len(data) {
				return nil, fmt.Errorf("params truncated at offset %d", pos)
			}
			doc.Params = new(cometbft.ConsensusParams)
			if err := doc.Params.UnmarshalBinary(data[pos : pos+length]); err != nil {
				return nil, fmt.Errorf("parse params: %w", err)
			}
			pos += length
			fmt.Printf("  Parsed params: MaxBytes=%d\n", doc.Params.Block.MaxBytes)

		case 3: // validators (array) - THIS IS THE BROKEN PART
			arrayLen, bytesRead := readVarint(data[pos:])
			if bytesRead == 0 {
				return nil, fmt.Errorf("invalid validators length at offset %d", pos)
			}
			pos += bytesRead
			arrayEnd := pos + arrayLen
			if arrayEnd > len(data) {
				return nil, fmt.Errorf("validators truncated at offset %d", pos)
			}

			fmt.Printf("  Parsing validators array: %d bytes (offset %d to %d)\n", arrayLen, pos, arrayEnd)

			// Parse validators - they're missing end markers
			validators, err := parseCorruptedValidators(data[pos:arrayEnd])
			if err != nil {
				return nil, fmt.Errorf("parse validators: %w", err)
			}
			doc.Validators = validators
			pos = arrayEnd
			fmt.Printf("  Parsed %d validators, continuing at offset %d\n", len(validators), pos)
			fmt.Printf("  Next bytes: %x\n", data[pos:min(pos+10, len(data))])

		case 4: // block (protobuf)
			length, bytesRead := readVarint(data[pos:])
			if bytesRead == 0 {
				return nil, fmt.Errorf("invalid block length at offset %d", pos)
			}
			pos += bytesRead
			if pos+length > len(data) {
				return nil, fmt.Errorf("block truncated at offset %d", pos)
			}
			fmt.Printf("  Block data (%d bytes): %x\n", length, data[pos:pos+length])
			doc.Block = new(cometbft.Block)
			if err := doc.Block.UnmarshalBinary(data[pos : pos+length]); err != nil {
				// Block parsing may fail for genesis blocks, that's OK
				fmt.Printf("  Block parse warning: %v\n", err)
				doc.Block = nil
			} else {
				fmt.Printf("  Parsed block: Height=%d, ChainID=%s, LastCommit=%v\n",
					doc.Block.Height, doc.Block.ChainID, doc.Block.LastCommit != nil)
			}
			pos += length
			// After block, we're done with the GenesisDoc
			// Any remaining data is commit signatures which we don't need for the repair
			if pos < len(data) {
				fmt.Printf("  Ignoring %d bytes of trailing data (likely commit signatures)\n", len(data)-pos)
			}
			return doc, nil

		case 5, 6, 7: // Trailing data fields (commit signatures) - ignore
			// These are extra fields that were appended to the consensus section
			// but aren't part of the widget-encoded GenesisDoc
			fmt.Printf("  Ignoring field %d at offset %d (extra data)\n", fieldID, pos-1)
			// Skip to end
			return doc, nil

		default:
			return nil, fmt.Errorf("unknown field %d at offset %d", fieldID, pos-1)
		}
	}

	return doc, nil
}

// parseCorruptedValidators parses validators that are missing end markers
func parseCorruptedValidators(data []byte) ([]*cometbft.Validator, error) {
	var validators []*cometbft.Validator
	pos := 0

	for pos < len(data) {
		// Each validator starts with field 1 (address)
		if data[pos] != 1 {
			// If we hit a 0 or reach the end, we're done
			if data[pos] == 0 {
				pos++
				continue
			}
			break
		}

		v, bytesRead, err := parseOneValidator(data[pos:])
		if err != nil {
			return nil, fmt.Errorf("parse validator %d: %w", len(validators), err)
		}
		validators = append(validators, v)
		pos += bytesRead

		fmt.Printf("    Validator %d: %s (power=%d, addr=%s)\n",
			len(validators)-1, v.Name, v.Power, hex.EncodeToString(v.Address[:8]))
	}

	return validators, nil
}

// parseOneValidator parses a single validator from the corrupted format
func parseOneValidator(data []byte) (*cometbft.Validator, int, error) {
	v := new(cometbft.Validator)
	pos := 0

	fmt.Printf("      parseOneValidator: data len=%d, first bytes=%x\n", len(data), data[:min(20, len(data))])

	for pos < len(data) {
		fieldID := data[pos]

		fmt.Printf("      pos=%d, fieldID=%d\n", pos, fieldID)

		// Validators end at field 0 or when we hit field 1 again (next validator)
		if fieldID == 0 {
			pos++
			fmt.Printf("      -> End marker at pos %d\n", pos-1)
			break
		}
		if fieldID == 1 && pos > 0 {
			// This is the start of the next validator
			fmt.Printf("      -> Next validator at pos %d\n", pos)
			break
		}

		pos++

		switch fieldID {
		case 1: // address (bytes)
			length, bytesRead := readVarint(data[pos:])
			if bytesRead == 0 {
				return nil, 0, fmt.Errorf("invalid address length")
			}
			pos += bytesRead
			if pos+length > len(data) {
				return nil, 0, fmt.Errorf("address truncated")
			}
			v.Address = make([]byte, length)
			copy(v.Address, data[pos:pos+length])
			pos += length
			fmt.Printf("      -> address: %x\n", v.Address)

		case 2: // type (enum - varint)
			val, bytesRead := readVarint(data[pos:])
			if bytesRead == 0 {
				return nil, 0, fmt.Errorf("invalid type value")
			}
			v.Type = protocol.SignatureType(val)
			pos += bytesRead
			fmt.Printf("      -> type: %d\n", v.Type)

		case 3: // pubKey (bytes)
			length, bytesRead := readVarint(data[pos:])
			if bytesRead == 0 {
				return nil, 0, fmt.Errorf("invalid pubKey length")
			}
			pos += bytesRead
			if pos+length > len(data) {
				return nil, 0, fmt.Errorf("pubKey truncated")
			}
			v.PubKey = make([]byte, length)
			copy(v.PubKey, data[pos:pos+length])
			pos += length
			fmt.Printf("      -> pubKey: %x\n", v.PubKey)

		case 4: // power (int - zigzag varint)
			val, bytesRead := readVarint(data[pos:])
			if bytesRead == 0 {
				return nil, 0, fmt.Errorf("invalid power value")
			}
			// The original corrupted data uses raw varint, but widget encoding uses zigzag
			// Raw varint 2 -> zigzag decoded: 1, or raw varint 4 -> zigzag decoded: 2
			// The raw value IS the power (original encoding was wrong)
			v.Power = int64(val)
			pos += bytesRead
			fmt.Printf("      -> power: %d (raw varint, will be re-encoded as zigzag)\n", v.Power)

		case 5: // name (string)
			length, bytesRead := readVarint(data[pos:])
			if bytesRead == 0 {
				return nil, 0, fmt.Errorf("invalid name length")
			}
			pos += bytesRead
			if pos+length > len(data) {
				return nil, 0, fmt.Errorf("name truncated")
			}
			v.Name = string(data[pos : pos+length])
			pos += length
			fmt.Printf("      -> name: %s\n", v.Name)

		default:
			// Unknown field means we've reached the end of this validator
			// (actually the start of a new field like field 4 = block)
			fmt.Printf("      -> Unknown field %d, stopping\n", fieldID)
			pos-- // Put back the field ID
			return v, pos, nil
		}
	}

	fmt.Printf("      parseOneValidator done: pos=%d, name=%s, power=%d\n", pos, v.Name, v.Power)
	return v, pos, nil
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Ensure we satisfy the ioutil.SectionReader interface for copying
var _ ioutil.SectionReader = (*os.File)(nil)
