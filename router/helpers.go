package router

import tmnet "github.com/tendermint/tendermint/libs/net"

func RandPort() int {
	port, err := tmnet.GetFreePort()
	if err != nil {
		panic(err)
	}
	return port
}

