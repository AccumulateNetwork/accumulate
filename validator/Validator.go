package validator

//should define return codes for validation...
type ValidationCode uint32

const (
	Success ValidationCode = 0
	BufferUnderflow = 1
	BufferOverflow = 2
	InvalidSignature = 3
	Fail = 4
)

type ValidatorInterface interface {
	Validate(tx []byte) uint32
}


type ValidationContext struct {
	//tbd
}



