package types

const (
	ChainTypeDC               = uint64(iota + 1) // Directory Chain
	ChainTypeBVC                                 // Block Validator Chain
	ChainTypeAdi                                 // Accumulate Digital/Distributed Identity/Identifier/Domain
	ChainTypeToken                               // Token Issue
	ChainTypeTokenAccount                        // Token Account
	ChainTypeAnonTokenAccount                    // Anonymous Token Account
	ChainTypeTransaction                         // Pending Chain
	ChainTypeSignatureGroup                      //
)

func ChainTypeString(cType uint64) string {
	switch cType {
	case ChainTypeDC:
		return "ChainTypeDC"
	case ChainTypeBVC:
		return "ChainTypeBVC"
	case ChainTypeAdi:
		return "ChainTypeAdi"
	case ChainTypeTokenAccount:
		return "ChainTypeTokenAccount"
	case ChainTypeToken:
		return "ChainTypeToken"
	case ChainTypeAnonTokenAccount:
		return "ChainTypeAnonTokenAccount"
	case ChainTypeTransaction:
		return "ChainTypeTransaction"
	case ChainTypeSignatureGroup:
		return "ChainTypeSignatureGroup"
	default:
		return "unknown"
	}
}
