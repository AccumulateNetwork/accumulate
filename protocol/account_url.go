// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"reflect"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

var (
	urlPtrType = reflect.TypeOf((*url.URL)(nil))
	txidType   = reflect.TypeOf(url.TxID{})
)

// CheckAccountUrls refuses a value that names an account by a URL with user
// info, a port, a query or a fragment, wherever the URL sits in it. An
// account's identity (url.AccountID) is its host and path alone, so such a
// URL names the same account as the URL without them, and compares unequal to
// it (url.Equal, url.Compare).
func CheckAccountUrls(v any) error {
	return checkAccountUrls(reflect.ValueOf(v))
}

func checkAccountUrls(v reflect.Value) error {
	if !v.IsValid() {
		return nil
	}
	switch v.Type() {
	case urlPtrType:
		if v.IsNil() {
			return nil
		}
		return checkAccountUrl(v.Interface().(*url.URL))
	case txidType:
		return nil
	}

	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return checkAccountUrls(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !v.Type().Field(i).IsExported() {
				continue
			}
			if err := checkAccountUrls(v.Field(i)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		for i := 0; i < v.Len(); i++ {
			if err := checkAccountUrls(v.Index(i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkAccountUrl(u *url.URL) error {
	switch {
	case u.UserInfo != "":
		return errors.BadRequest.WithFormat("account URL %v has user info", u)
	case strings.Contains(u.Authority, ":"):
		return errors.BadRequest.WithFormat("account URL %v has a port", u)
	case u.Query != "":
		return errors.BadRequest.WithFormat("account URL %v has a query", u)
	case u.Fragment != "":
		return errors.BadRequest.WithFormat("account URL %v has a fragment", u)
	}
	return nil
}
