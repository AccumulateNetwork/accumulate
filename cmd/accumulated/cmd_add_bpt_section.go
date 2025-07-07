package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

var cmdAddBptSection = &cobra.Command{
	Use:   "add-bpt-section <input-snapshot> <output-snapshot>",
	Short: "Add a BPT section to a snapshot file by computing it from the account data",
	Args:  cobra.ExactArgs(2),
	Run:   addBptSection,
}

func addBptSection(_ *cobra.Command, args []string) {
	inputFile := args[0]
	outputFile := args[1]

	fmt.Printf("Adding BPT section to snapshot: %s -> %s\n", inputFile, outputFile)

	// Step 1: Open the input snapshot file
	file, err := os.Open(inputFile)
	checkf(err, "open input snapshot file")
	defer file.Close()

	// Step 2: Create temporary in-memory database for BPT computation
	store := memory.New(nil)
	db := database.New(store, nil)
	db.SetObserver(execute.NewDatabaseObserver())
	defer db.Close()

	// Step 3: Restore the snapshot to compute BPT
	fmt.Println("Restoring snapshot to compute BPT...")
	opts := &database.RestoreOptions{
		SkipHashCheck: true, // Skip hash check since we're computing it
	}
	
	err = database.Restore(db, file, opts)
	checkf(err, "restore snapshot")

	// Step 4: Get the computed BPT root hash
	batch := db.Begin(false)
	defer batch.Discard()
	
	rootHash, err := batch.GetBptRootHash()
	checkf(err, "get BPT root hash")
	fmt.Printf("Computed BPT Root Hash: %x\n", rootHash)

	// Step 5: Read the original snapshot to copy sections
	file.Seek(0, 0) // Reset file position
	reader, err := snapshot.Open(file)
	checkf(err, "open snapshot reader")

	// Step 6: Create output snapshot with BPT section
	fmt.Println("Creating output snapshot with BPT section...")
	outFile, err := os.Create(outputFile)
	checkf(err, "create output file")
	defer outFile.Close()

	writer, err := snapshot.Create(outFile)
	checkf(err, "create snapshot writer")

	// Step 7: Copy header with updated root hash
	header := *reader.Header
	header.RootHash = rootHash
	err = writer.WriteHeader(&header)
	checkf(err, "write header")

	// Step 8: Copy existing sections (accounts, etc.)
	for i, section := range reader.Sections {
		fmt.Printf("Copying section %d: %s\n", i, section.Type())
		
		// Open section reader
		sectionReader, err := reader.OpenSection(i)
		checkf(err, "open section %d", i)

		// Create section writer
		sectionWriter, err := writer.OpenSection(section.Type())
		checkf(err, "create section writer")

		// Copy section data
		err = copySection(sectionReader, sectionWriter)
		checkf(err, "copy section %d", i)

		err = sectionWriter.Close()
		checkf(err, "close section writer")
	}

	// Step 9: Add BPT section
	fmt.Println("Adding BPT section...")
	err = addBptSectionToWriter(writer, batch)
	checkf(err, "add BPT section")

	// Step 10: Close writer
	err = writer.Close()
	checkf(err, "close writer")

	fmt.Printf("✅ Successfully created snapshot with BPT section: %s\n", outputFile)
	fmt.Printf("Root Hash: %x\n", rootHash)
}

func copySection(reader snapshot.SectionReader, writer snapshot.SectionWriter) error {
	// Copy section data in chunks
	buf := make([]byte, 64*1024) // 64KB buffer
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			_, writeErr := writer.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return err
		}
	}
	return nil
}

func addBptSectionToWriter(writer *snapshot.Writer, batch *database.Batch) error {
	// Create BPT section writer
	bptWriter, err := writer.OpenRawBPT()
	if err != nil {
		return fmt.Errorf("open BPT section: %w", err)
	}
	defer bptWriter.Close()

	// Iterate over BPT and write entries
	bpt := batch.BPT()
	err = bpt.ForEach(func(key []byte, value []byte) error {
		// Write key-value pair (32 bytes key hash + 32 bytes value hash)
		if len(key) != 32 {
			return fmt.Errorf("invalid key length: %d", len(key))
		}
		if len(value) != 32 {
			return fmt.Errorf("invalid value length: %d", len(value))
		}

		// Write key hash
		_, err := bptWriter.Write(key)
		if err != nil {
			return fmt.Errorf("write key: %w", err)
		}

		// Write value hash
		_, err = bptWriter.Write(value)
		if err != nil {
			return fmt.Errorf("write value: %w", err)
		}

		return nil
	})

	return err
}
