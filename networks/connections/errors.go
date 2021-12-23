package connections

import (
	"errors"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/url"
)

var ErrInvalidUrl = errors.New("invalid URL")
var NoHealthyNodes = errors.New("no healthy nodes available")
var LocaNodeNotFound = errors.New("no local node was found")
var LocaNodeNotHealthy = errors.New("the local node was found to be unhealthy")

func errorCouldNotSelectNode(url *url.URL, err error) error {
	return fmt.Errorf("error while slecting node for url %s: %v", url.String(), err)
}

func bvnNotFound(bvnName string) error {
	return fmt.Errorf("bvn %s could not be found", bvnName)
}

func dnNotFound() error {
	return fmt.Errorf("no directory node could not be found")
}

func errorNodeNotHealthy(subnet string, address string, err error) error {
	return fmt.Errorf("the only node available %s / %s is not healthy due to error %v", subnet, address, err)
}

func dnNotHealthy(address string, err error) error {
	return fmt.Errorf("the directory node %s is not healthy due to error %v", address, err)
}

func bvnNotHealthy(address string, err error) error {
	return fmt.Errorf("the block validator node %s is not healthy due to error %v", address, err)
}
