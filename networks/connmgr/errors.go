package connmgr

import (
	"errors"
	"fmt"
)

var ErrInvalidUrl = errors.New("invalid URL")

func errorCouldNotSelectNode(url string, err error) error {
	return fmt.Errorf("error while slecting node for url %s: %v", url, err)
}

func bvnNotFound(bvnName string) error {
	return fmt.Errorf("bvn %s could not be found", bvnName)
}
