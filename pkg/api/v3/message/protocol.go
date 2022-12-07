package message

import (
	"bytes"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

// P_ACC is the multicodec code for the acc protocol.
const P_ACC = 0x300000

func init() {
	// Register the acc protocol
	err := multiaddr.AddProtocol(multiaddr.Protocol{
		Name:  "acc",
		Code:  P_ACC,
		VCode: multiaddr.CodeToVarint(P_ACC),
		Size:  -1,
		Transcoder: multiaddr.NewTranscoderFromFunctions(
			func(s string) ([]byte, error) { return []byte(strings.ToLower(s)), nil },
			func(b []byte) (string, error) { return string(bytes.ToLower(b)), nil },
			func(_ []byte) error { return nil },
		),
	})
	if err != nil {
		panic(err)
	}
}
