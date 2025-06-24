package main

import (
	"fmt"
)

// sc_reconstruct is the implementation of the snapshot reconstruction process
// that will be assigned to the sc_ReconstructSnapshot function variable in sc.go
func sc_reconstruct(scState *sc_State) error {
	// Step 1: Start reconstruction and validate temporary files
	// This ensures all section files are ready for reconstruction and opens the output file
	err := sc_StartReconstruction(scState)
	if err != nil {

		return fmt.Errorf("failed to start reconstruction: %w", err)
	}

	// Ensure cleanup happens when we're done
	defer sc_Cleanup(scState)

	// Step 2: Write all sections to the output file
	// The output file is now stored in scState.OutFile
	err = sc_WriteSections(scState)
	if err != nil {
		return fmt.Errorf("failed to write sections: %w", err)
	}

	// Step 3: Update section offsets in the header and section headers
	// Note: Section offsets are now calculated during section writing
	// so we don't need to update them separately

	// Ensure all writes are flushed to disk
	err = scState.OutFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync output file: %w", err)
	}

	// Print summary
	fmt.Printf("\nReconstruction completed successfully\n")
	fmt.Printf("Total sections: %d\n", scState.SectionFiles.Count())

	return nil
}
