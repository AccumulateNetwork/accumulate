package main

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type urlFlag struct {
	v **url.URL
}

func (f urlFlag) Type() string   { return "acc-url" }
func (f urlFlag) String() string { return fmt.Sprint(*f.v) }
func (f urlFlag) Set(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	*f.v = u
	return nil
}
