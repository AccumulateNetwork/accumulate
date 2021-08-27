package url

import (
	"net/url"
	"testing"
)

func TestParseURL(t *testing.T) {
	u := "acc://RedWagon:8000/世界/acc"
	v, err := url.Parse(u)
	_ = v
	if err != nil {
		t.Fatal(err)
	}
}
