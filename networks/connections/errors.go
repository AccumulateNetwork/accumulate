package connections

import (
	"errors"
	"fmt"
)

var ErrInvalidUrl = errors.New("invalid URL")
var NoHealthyNodes = errors.New("no health nodes available")
var LocaNodeNotFound = errors.New("no local node was found")

func errorCouldNotSelectNode(url string, err error) error {
	return fmt.Errorf("error while slecting node for url %s: %v", url, err)
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
