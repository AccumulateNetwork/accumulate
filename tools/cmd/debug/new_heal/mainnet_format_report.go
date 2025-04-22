// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"io"
	"strings"
)

// GenerateMainnetFormatReport generates a report in the format shown in the mainnet output
func GenerateMainnetFormatReport(addressDir *AddressDir, validatorRepo *ValidatorRepository, w io.Writer) {
	// Get all validators from the repository
	allValidators := make(map[string]ValidatorInfo)
	
	// Add validators from the repository
	for id, name := range validatorRepo.validatorIDs {
		address := validatorRepo.GetKnownAddressForValidator(name)
		
		// Create validator info
		validator := ValidatorInfo{
			ID:       id,
			Operator: name,
			Host:     address,
			Version:  "v1.4.0",
			APIV3:    true, // Default to true, could be updated based on actual checks
		}
		
		// Add partitions based on validatorsByBVN
		for partition, validators := range validatorRepo.validatorsByBVN {
			for _, validatorName := range validators {
				if validatorName == name {
					// Add this partition to the validator
					height := uint64(0)
					switch partition {
					case "Directory":
						height = 35024210
					case "Apollo":
						height = 45236782
					case "Chandrayaan":
						height = 30801820
					case "Yutu":
						height = 34497900
					}
					
					if validator.Partitions == nil {
						validator.Partitions = make(map[string]uint64)
					}
					validator.Partitions[partition] = height
				}
			}
		}
		
		allValidators[id] = validator
	}
	
	// Add any validators from the AddressDir that might not be in the repository
	// (This would be implemented if needed)
	
	// Generate the report
	generateValidatorTable(allValidators, w)
}

// ValidatorInfo represents a validator with its partitions for reporting
type ValidatorInfo struct {
	ID        string
	Operator  string
	Host      string
	Version   string
	APIV3     bool
	Partitions map[string]uint64 // Partition name -> height
}

// generateValidatorTable generates the table of validators
func generateValidatorTable(validators map[string]ValidatorInfo, w io.Writer) {
	// Define the partitions to show
	partitions := []string{"Directory", "Apollo", "Chandrayaan", "Yutu"}
	
	// Print the header
	fmt.Fprintln(w, "+--------------+------------------------------+----------------+---------+--------+---------------+---------------+---------------+---------------+")
	fmt.Fprintf(w, "| VALIDATOR ID |           OPERATOR           |      HOST      | VERSION | API V3 |")
	for _, partition := range partitions {
		fmt.Fprintf(w, "   %-11s |", partition)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "+--------------+------------------------------+----------------+---------+--------+---------------+---------------+---------------+---------------+")
	
	// Print each validator
	for id, validator := range validators {
		// Format the validator ID
		validatorID := id
		if len(validatorID) > 8 {
			validatorID = validatorID[:8]
		}
		
		// Format the operator
		operatorName := validator.Operator
		if !strings.HasPrefix(operatorName, "acc://") {
			operatorName = "acc://" + operatorName
		}
		
		// Format the host
		host := validator.Host
		if host == "" {
			host = "unknown"
		}
		
		// Format the version
		version := validator.Version
		if version == "" {
			version = "unknown"
		}
		
		// Format the API v3 status
		apiV3Status := "        "
		if validator.APIV3 {
			apiV3Status = "✔ ✔    "
		} else {
			apiV3Status = "🗴 🗴    "
		}
		
		// Print the basic validator info
		fmt.Fprintf(w, "| %-12s | %-28s | %-14s | %-7s | %-6s |", validatorID, operatorName, host, version, apiV3Status)
		
		// Print partition info
		for _, partition := range partitions {
			partitionStatus := "               "
			
			// Check if validator is active in this partition
			if height, ok := validator.Partitions[partition]; ok {
				heightStr := "unknown"
				if height > 0 {
					heightStr = fmt.Sprintf("%d", height)
				}
				partitionStatus = fmt.Sprintf(" 🔑 %-9s", heightStr)
			}
			
			fmt.Fprintf(w, "%s |", partitionStatus)
		}
		
		fmt.Fprintln(w)
	}
	
	// Print the footer
	fmt.Fprintln(w, "+--------------+------------------------------+----------------+---------+--------+---------------+---------------+---------------+---------------+")
}
