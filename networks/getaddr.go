package networks

import (
	"fmt"
	"net/url"
	"strconv"
)

func GetRpcAddr(netOrIp string, portOffset int) (string, error) {
	net := All[netOrIp]
	ip, err := url.Parse(netOrIp)
	if net != nil {
		ip = &url.URL{Scheme: "tcp", Host: fmt.Sprintf("%s:%d", net.Nodes[0].IP, net.Port+portOffset)}
	} else if err != nil {
		return "", fmt.Errorf("%q is not a URL or a named network", netOrIp)
	} else if ip.Port() == "" {
		return "", fmt.Errorf("missing port number: %q", netOrIp)
	} else if _, err := strconv.ParseInt(ip.Port(), 10, 17); err != nil {
		return "", fmt.Errorf("invalid port number: %q", netOrIp)
	}

	return ip.String(), nil
}
