package testing

import (
	"crypto/sha256"
	"fmt"
	"net"
	"runtime"
)

func getIP() net.IP {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		panic("could not determine caller")
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", file, line)))
	ip := net.ParseIP("127.0.0.0")
	ip[13] = hash[0]
	ip[14] = hash[1]
	ip[15] = hash[2]
	if ip[15] == 0 {
		ip[15]++
	}
	return ip
}

func GetIP() net.IP {
	return getIP()
}

func GetIPs(n int) []net.IP {
	ip := getIP()
	ips := make([]net.IP, n)
	for i := range ips {
		ips[i] = make(net.IP, len(ip))
		copy(ips[i], ip)
		ip[15]++
	}
	return ips
}
