package networks

import (
	"fmt"
	"net/url"
	"strconv"
)

func resolve(name string) (*Subnet, error) {
	sub := all[name]
	if sub != nil {
		return sub, nil
	}
	if nameCount[name] > 1 {
		return nil, fmt.Errorf("%q is ambiguous and must be qualified with the network name", name)
	}
	return nil, nil
}

func Resolve(name string) (*Subnet, error) {
	sub, err := resolve(name)
	if err != nil {
		return nil, err
	}
	if sub != nil {
		return sub, nil
	}
	return nil, fmt.Errorf("%q is not the name of a subnet", name)
}

func ResolveAddr(netOrIp string, allowIp bool) (string, int, error) {
	sub, err := resolve(netOrIp)
	if err != nil {
		return "", 0, err
	}
	if sub != nil {
		return sub.Nodes[0].IP, sub.Port, nil
	}
	if !allowIp {
		return "", 0, fmt.Errorf("%q is not the name of a subnet", netOrIp)
	}

	ip, err := url.Parse(netOrIp)
	if err != nil {
		return "", 0, fmt.Errorf("%q is not a URL or a named network", netOrIp)
	}

	if ip.Path != "" && ip.Path != "/" {
		return "", 0, fmt.Errorf("address cannot have a path")
	}

	if ip.Port() == "" {
		return "", 0, fmt.Errorf("%q does not specify a port number", netOrIp)
	}
	port, err := strconv.ParseUint(ip.Port(), 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("%q is not a valid port number", ip.Port())
	}

	return ip.Hostname(), int(port), nil
}
