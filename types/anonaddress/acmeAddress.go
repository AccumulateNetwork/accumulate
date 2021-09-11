package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// GenerateAcmeAddress
// Given a public key, create a Ascii representation that can be used
// for the Anonymous token chain.
func GenerateAcmeAddress(pubKey []byte) string {
	keyHash := sha256.Sum256(pubKey[:])                               // Get the hash of the public key
	encAddress := hex.EncodeToString(keyHash[:20])                    // Compute base36 to give fewer characters
	encAddress = strings.ToLower(encAddress)                          // Lowercase for usability
	address := string(append([]byte("acme-"), []byte(encAddress)...)) // Add the "acme" prefix
	cs := sha256.Sum256([]byte(address))                              // compute checksum of address (including prefix)
	checksum := strings.ToLower(hex.EncodeToString(cs[28:]))          // compute lowercase hex checksum (last 4 bytes)
	bAcmeAddress := append([]byte(address)[:], []byte(checksum)...)   // append checksum to end
	AcmeAddress := string(bAcmeAddress)                               // Get the string version, for the debugger
	return AcmeAddress                                                // Return it.
}

func IsAcmeAddress(address string) error {
	if len(address) != 53 { //										     All addresses are 53 chars long
		return fmt.Errorf("address length %d != 53", len(address)) //    error if this one isn't
	} //
	cs := sha256.Sum256([]byte(address[:45]))       //                   get hash of the address including prefix
	checksum, err := hex.DecodeString(address[45:]) //                   get the checksum from end of address
	if err != nil {                                 //                   error means non hex in checksum
		return fmt.Errorf("checksum invalid") //
	} //
	if !bytes.Equal(cs[28:], checksum) { //                              check if the checksum checks out
		return fmt.Errorf("checksum does not match") //
	} //
	_, err = hex.DecodeString(address[5:45]) //                          decode the nex of the address
	if err != nil {                          //                          all we can check is that the hex is hex
		return fmt.Errorf("invalid hex found in address") //             if not, report error
	} //

	return nil //                                                        Looks good as it can be checked
}
