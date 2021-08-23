package security

type Signature interface {
	Validate(sigSpec SigSpec, trans []byte) // Validates a given transaction.  Note that the sigSpec may be required
	//                                         in order to know the public keys required and the number of signatures
	//                                         that are required or involved.
}
