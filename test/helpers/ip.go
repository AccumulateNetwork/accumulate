package testing

import (
	"crypto/sha256"
	"fmt"
	"net"
	"runtime"
)

func hashCaller(skip int, id string) []byte {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		panic("could not determine caller")
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d-%s", file, line, id)))
	return hash[:]
}

// getIP creates a 127.x.y.z address from the first 3 bytes of the hash.
func getIP(hash []byte) net.IP {
	ip := net.ParseIP("127.0.0.0")
	ip[13] = hash[0]
	ip[14] = hash[1]
	ip[15] = hash[2]
	if ip[15] == 0 {
		ip[15]++
	}
	return ip
}

// // getPort calculates a port number between 1001 and 65535 from bytes 4-5 of the
// // hash.
// func getPort(hash []byte) int {
// 	v := int(hash[3])<<8 | int(hash[4])
// 	return v%(1<<16-1002) + 1002
// }

func GetIP() net.IP {
	return getIP(hashCaller(1, ""))
}

func GetIPs(n int) []net.IP {
	ips := make([]net.IP, n)
	for i := range ips {
		ips[i] = getIP(hashCaller(1, fmt.Sprint(i)))
	}
	return ips
}
