// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"log"
	"sync"
)

// ValidatorRepository provides a centralized repository for validator information
type ValidatorRepository struct {
	mu                 sync.RWMutex
	logger             *log.Logger
	knownAddresses     map[string]string
	validatorsByBVN    map[string][]string
	validatorEndpoints map[string]ValidatorEndpoints
	validatorIDs       map[string]string // Maps validator IDs to validator names
}

// ValidatorEndpoints contains the various endpoints for a validator
type ValidatorEndpoints struct {
	P2PAddress     string
	RPCAddress     string
	APIAddress     string
	MetricsAddress string
}

// NewValidatorRepository creates a new validator repository
func NewValidatorRepository(logger *log.Logger) *ValidatorRepository {
	repo := &ValidatorRepository{
		logger:             logger,
		knownAddresses:     make(map[string]string),
		validatorsByBVN:    make(map[string][]string),
		validatorEndpoints: make(map[string]ValidatorEndpoints),
		validatorIDs:       make(map[string]string),
	}

	// Initialize with known validator addresses
	repo.initializeKnownAddresses()
	repo.initializeValidatorsByBVN()
	repo.initializeValidatorEndpoints()

	return repo
}

// GetKnownAddressForValidator returns a known IP address for a validator ID
func (r *ValidatorRepository) GetKnownAddressForValidator(validatorID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if addr, ok := r.knownAddresses[validatorID]; ok {
		if r.logger != nil {
			r.logger.Printf("Found known address for validator %s: %s", validatorID, addr)
		}
		return addr
	}

	if r.logger != nil {
		r.logger.Printf("No known address for validator %s", validatorID)
	}
	return ""
}

// GetValidatorsForBVN returns the list of validators for a specific BVN
func (r *ValidatorRepository) GetValidatorsForBVN(bvnName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if validators, ok := r.validatorsByBVN[bvnName]; ok {
		return validators
	}
	return nil
}

// GetValidatorsForPartition returns the list of validators for a specific partition
func (r *ValidatorRepository) GetValidatorsForPartition(partitionID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// For Directory Network, return all known validators
	if partitionID == "dn" {
		validators := make([]string, 0, len(r.knownAddresses))
		for validator := range r.knownAddresses {
			validators = append(validators, validator)
		}
		return validators
	}
	
	// For BVNs, check if we have a mapping
	// Strip "bvn-" prefix if present
	bvnName := partitionID
	if len(partitionID) > 4 && partitionID[:4] == "bvn-" {
		bvnName = partitionID[4:]
	}
	
	return r.GetValidatorsForBVN(bvnName)
}

// GetKnownAddress returns a known IP address for a validator ID
func (r *ValidatorRepository) GetKnownAddress(validatorID string) string {
	return r.GetKnownAddressForValidator(validatorID)
}

// GetValidatorByID returns the validator name for a given validator ID
func (r *ValidatorRepository) GetValidatorByID(validatorID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if validatorName, ok := r.validatorIDs[validatorID]; ok {
		if r.logger != nil {
			r.logger.Printf("Found validator name for ID %s: %s", validatorID, validatorName)
		}
		return validatorName
	}

	if r.logger != nil {
		r.logger.Printf("No validator name found for ID %s", validatorID)
	}
	return ""
}

// GetValidatorID returns the validator ID for a given validator name
func (r *ValidatorRepository) GetValidatorID(validatorName string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Iterate through the map to find the validator ID
	for id, name := range r.validatorIDs {
		if name == validatorName {
			if r.logger != nil {
				r.logger.Printf("Found validator ID for name %s: %s", validatorName, id)
			}
			return id
		}
	}

	if r.logger != nil {
		r.logger.Printf("No validator ID found for name %s", validatorName)
	}
	return ""
}

// GetValidatorEndpoints returns the endpoints for a validator
func (r *ValidatorRepository) GetValidatorEndpoints(validatorID string) ValidatorEndpoints {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if endpoints, ok := r.validatorEndpoints[validatorID]; ok {
		return endpoints
	}
	return ValidatorEndpoints{}
}

// initializeKnownAddresses initializes the map of known validator addresses
func (r *ValidatorRepository) initializeKnownAddresses() {
	// Consolidated list of known validator addresses from mainnet output
	r.knownAddresses = map[string]string{
		// Validators with operator IDs
		"kompendium.acme":        "116.202.214.38",
		"LunaNova.acme":          "193.35.56.176",
		"TurtleBoat.acme":        "3.28.207.55",
		"MusicCityNode.acme":     "54.188.179.135",
		"ConsensusNetworks.acme": "unknown", // Listed as unknown in output
		"tfa.acme":               "65.109.48.173",
		"CodeForj.acme":          "45.79.217.209",
		"PrestigeIT.acme":        "50.17.246.3",
		"defidevs.acme":          "54.85.31.44", // Multiple entries for defidevs
		"Sphereon.acme":          "85.215.104.235",
		"ACMEMining.acme":        "54.160.138.164",
		"Inveniam.acme":          "34.224.230.218",
		"HighStakes.acme":        "34.170.72.80",
		"FederateThis.acme":      "144.76.105.23",
		"DetroitLedgerTech.acme": "unknown", // Listed as unknown in output
		
		// Additional defidevs entries
		"defidevs.acme.1":        "54.146.244.44",
		"defidevs.acme.2":        "23.22.212.106",
		
		// Non-operator validators (by ID)
		"validator-130d8113":     "3.135.9.97",
		"validator-4838c713":     "3.86.85.133",
		"validator-8bdc72a0":     "44.204.224.126",
		"validator-83dc73b6":     "192.99.35.81",
		"validator-d7b2fa10":     "3.99.166.147",
		"validator-c7ccc9b1":     "35.92.21.90",
		"validator-25a0ec24":     "99.35.223.146",
		"validator-ec62c2e0":     "99.35.223.145",
		"validator-aa55ef47":     "91.237.141.175",
		"validator-8ad64d6a":     "18.133.170.113",
		"validator-ba3a2bc1":     "18.168.202.86",
		"validator-f278a3b6":     "16.171.4.135",
		"validator-0354a6d3":     "54.237.244.42",
	}
	
	// Add validator IDs mapping
	r.validatorIDs = map[string]string{
		"0b2d838c": "kompendium.acme",
		"0db47c9a": "LunaNova.acme",
		"31b66a34": "TurtleBoat.acme",
		"44bb47ce": "MusicCityNode.acme",
		"4f32232a": "ConsensusNetworks.acme",
		"63196e1d": "tfa.acme",
		"6623fffb": "CodeForj.acme",
		"6cdf2fbd": "PrestigeIT.acme",
		"735c3cb5": "defidevs.acme",
		"775ecf99": "Sphereon.acme",
		"78b9c101": "ACMEMining.acme",
		"7b3cba2b": "Inveniam.acme",
		"7cd6f209": "HighStakes.acme",
		"8a93bab8": "defidevs.acme.1",
		"bc79dbf7": "defidevs.acme.2",
		"d2de1608": "FederateThis.acme",
		"fd8e48c6": "DetroitLedgerTech.acme",
		"130d8113": "validator-130d8113",
		"4838c713": "validator-4838c713",
		"8bdc72a0": "validator-8bdc72a0",
		"83dc73b6": "validator-83dc73b6",
		"d7b2fa10": "validator-d7b2fa10",
		"c7ccc9b1": "validator-c7ccc9b1",
		"25a0ec24": "validator-25a0ec24",
		"ec62c2e0": "validator-ec62c2e0",
		"aa55ef47": "validator-aa55ef47",
		"8ad64d6a": "validator-8ad64d6a",
		"ba3a2bc1": "validator-ba3a2bc1",
		"f278a3b6": "validator-f278a3b6",
		"0354a6d3": "validator-0354a6d3",
	}
}

// initializeValidatorsByBVN initializes the map of validators by BVN
func (r *ValidatorRepository) initializeValidatorsByBVN() {
	// Map of BVN to validators based on the mainnet output
	r.validatorsByBVN = map[string][]string{
		"Directory": {
			// All validators participate in Directory network
			"kompendium.acme",
			"LunaNova.acme",
			"TurtleBoat.acme",
			"MusicCityNode.acme",
			"ConsensusNetworks.acme",
			"tfa.acme",
			"CodeForj.acme",
			"PrestigeIT.acme",
			"defidevs.acme",
			"Sphereon.acme",
			"ACMEMining.acme",
			"Inveniam.acme",
			"HighStakes.acme",
			"defidevs.acme.1",
			"defidevs.acme.2",
			"FederateThis.acme",
			"DetroitLedgerTech.acme",
			"validator-130d8113",
			"validator-4838c713",
			"validator-8bdc72a0",
			"validator-83dc73b6",
			"validator-d7b2fa10",
			"validator-c7ccc9b1",
			"validator-aa55ef47",
			"validator-8ad64d6a",
			"validator-ba3a2bc1",
			"validator-f278a3b6",
			"validator-0354a6d3",
		},
		"Apollo": {
			// Based on mainnet output
			"kompendium.acme",
			"LunaNova.acme",
			"ACMEMining.acme",
			"defidevs.acme.2", // bc79dbf7
			"FederateThis.acme",
			"validator-130d8113",
			"validator-4838c713",
			"validator-aa55ef47",
		},
		"Chandrayaan": {
			// Based on mainnet output
			"TurtleBoat.acme",
			"ConsensusNetworks.acme",
			"PrestigeIT.acme",
			"defidevs.acme", // 735c3cb5
			"Sphereon.acme",
			"Inveniam.acme",
			"validator-8bdc72a0",
			"validator-d7b2fa10",
			"validator-c7ccc9b1",
			"validator-8ad64d6a",
			"validator-f278a3b6",
		},
		"Yutu": {
			// Based on mainnet output
			"MusicCityNode.acme",
			"tfa.acme",
			"CodeForj.acme",
			"HighStakes.acme",
			"defidevs.acme.1", // 8a93bab8
			"DetroitLedgerTech.acme",
			"validator-ba3a2bc1",
			"validator-0354a6d3",
		},
	}
}

// initializeValidatorEndpoints initializes the map of validator endpoints
func (r *ValidatorRepository) initializeValidatorEndpoints() {
	// For each known validator, initialize its endpoints
	for validatorID, ipAddress := range r.knownAddresses {
		r.validatorEndpoints[validatorID] = ValidatorEndpoints{
			P2PAddress:     "tcp://" + ipAddress + ":26656",
			RPCAddress:     "http://" + ipAddress + ":26657",
			APIAddress:     "http://" + ipAddress + ":8080",
			MetricsAddress: "http://" + ipAddress + ":26660",
		}
	}
}
