package networks

import (
	"fmt"
	"net/url"
	"strconv"
)

func GetRpcAddr(netOrIp string) (string, error) {
	net := all[netOrIp]
	ip, err := url.Parse(netOrIp)
	if net != nil {
		ip = &url.URL{Scheme: "tcp", Host: fmt.Sprintf("%s:%d", net.Nodes[0].IP, net.Port+TmRpcPortOffset)}
	} else if err != nil {
		return "", fmt.Errorf("%q is not a URL or a named network", netOrIp)
	} else if ip.Port() == "" {
		return "", fmt.Errorf("missing port number: %q", netOrIp)
	} else if _, err := strconv.ParseInt(ip.Port(), 10, 17); err != nil {
		return "", fmt.Errorf("invalid port number: %q", netOrIp)
	}

	return ip.String(), nil
}

func Resolve(name string) (*Subnet, error) {
	sub := all[name]
	if sub != nil {
		return sub, nil
	}
	if nameCount[name] > 1 {
		return nil, fmt.Errorf("%q is ambiguous and must be qualified with the network name", name)
	}
	return nil, fmt.Errorf("%q is not the name of a subnet", name)
}

func SubnetsForNetwork(networkName string) []*Subnet {
	network := networks[networkName]
	var ret []*Subnet
	for _, subnet := range network {
		ret = append(ret, subnet)
	}
	return ret
}
