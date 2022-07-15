package testdata

import _ "embed"

//go:embed test_factom_addresses
var FactomAddresses string

//go:embed merkle.yaml
var Merkle string
