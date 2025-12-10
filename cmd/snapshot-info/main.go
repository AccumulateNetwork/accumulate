package main

import (
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: snapshot-info <snapshot-file>")
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
	fmt.Printf("RootHash: %x\n", rd.Header.RootHash)
	if rd.Header.SystemLedger != nil {
		fmt.Printf("Partition: %s\n", rd.Header.SystemLedger.Url)
		fmt.Printf("Block Index: %d\n", rd.Header.SystemLedger.Index)
		fmt.Printf("Timestamp: %s\n", rd.Header.SystemLedger.Timestamp)
	}

	fmt.Println("\nSections:")
	hasConsensus := false
	for _, s := range rd.Sections {
		typeName := s.Type().String()
		if s.Type() == snapshot.SectionTypeConsensus {
			hasConsensus = true
		}
		fmt.Printf("  - %s (offset: %d, size: %d)\n", typeName, s.Offset(), s.Size())
	}

	fmt.Println()
	if hasConsensus {
		fmt.Println("[OK] Snapshot has Consensus section - suitable for restore")
	} else {
		fmt.Println("[FAIL] Snapshot MISSING Consensus section - NOT suitable for restore")
	}
}
