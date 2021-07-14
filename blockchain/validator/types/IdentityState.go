package types

import "fmt"

type IdentityState struct {
	publickey [32]byte
	adi string
}

func (app *IdentityState) MarshalBinary() ([]byte, error) {
	badi := []byte(app.adi)
    data := make([]byte,len(badi) + 32)
    i := copy(data[:], app.publickey[:])
    copy(data[i:],badi)
	return data, nil
}

func (app *IdentityState) UnmarshalBinary(data []byte) error {
	if len(data) < 32 + 1 {
		return fmt.Errorf("insufficent data")
	}
	i := copy(app.publickey[:], data[:32])
	app.adi = string([]byte(data[i:]))

	return nil
}

//
//func (app *IdentityState) MarshalEntry() (*Entry, error) {
//	return nil, nil
//}
//func (app *IdentityState) UnmarshalEntry(entry *Entry) error {
//	return nil
//}
