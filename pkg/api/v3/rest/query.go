// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package rest

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Querier struct{ api.Querier }

func (s Querier) Register(r *httprouter.Router) error {
	// This service needs escaped paths
	r.UseEscapedPath = true

	/// ***** Default ***** ///

	r.GET("/query/:id", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.DefaultQuery{
			IncludeReceipt: parseBool.allowMissing().fromQuery(r).try(&err, "include_receipt"),
		})
	})

	/// ***** Chain ***** ///

	r.GET("/query/:id/chain", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.ChainQuery{})
	})

	r.GET("/query/:id/chain/:name", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.ChainQuery{
			Name:  p.ByName("name"),
			Range: tryParseRangeFromQuery(&err, r, "", true),
		})
	})

	r.GET("/query/:id/chain/:name/index/:index", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.ChainQuery{
			Name:           p.ByName("name"),
			Index:          ptr(parseUint).fromRoute(p).try(&err, "index"),
			IncludeReceipt: parseBool.allowMissing().fromQuery(r).try(&err, "include_receipt"),
		})
	})

	r.GET("/query/:id/chain/:name/entry/:entry", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.ChainQuery{
			Name:           p.ByName("name"),
			Entry:          parseBytes.fromRoute(p).try(&err, "entry"),
			IncludeReceipt: parseBool.allowMissing().fromQuery(r).try(&err, "include_receipt"),
		})
	})

	/// ***** Data ***** ///

	r.GET("/query/:id/data", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.DataQuery{
			Range: tryParseRangeFromQuery(&err, r, "", true),
		})
	})

	r.GET("/query/:id/data/index/:index", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.DataQuery{
			Index: ptr(parseUint).fromRoute(p).try(&err, "index"),
		})
	})

	r.GET("/query/:id/data/entry/:entry", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.DataQuery{
			Entry: parseBytes.fromRoute(p).try(&err, "entry"),
		})
	})

	/// ***** Directory ***** ///

	r.GET("/query/:id/directory", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.DirectoryQuery{
			Range: tryParseRangeFromQuery(&err, r, "", false),
		})
	})

	/// ***** Pending ***** ///

	r.GET("/query/:id/pending", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.PendingQuery{
			Range: tryParseRangeFromQuery(&err, r, "", false),
		})
	})

	/// ***** Block ***** ///

	r.GET("/block/minor", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		s.tryCall(&err, w, r, protocol.DnUrl(), &api.BlockQuery{
			MinorRange: tryParseRangeFromQuery(&err, r, "", false),
			OmitEmpty:  parseBool.allowMissing().fromQuery(r).try(&err, "omit_empty"),
		})
	})

	r.GET("/block/minor/:index", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		s.tryCall(&err, w, r, protocol.DnUrl(), &api.BlockQuery{
			Minor:      ptr(parseUint).fromRoute(p).try(&err, "index"),
			EntryRange: tryParseRangeFromQuery(&err, r, "entry_", true),
		})
	})

	r.GET("/block/major", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		s.tryCall(&err, w, r, protocol.DnUrl(), &api.BlockQuery{
			MajorRange: tryParseRangeFromQuery(&err, r, "", false),
			OmitEmpty:  parseBool.allowMissing().fromQuery(r).try(&err, "omit_empty"),
		})
	})

	r.GET("/block/major/:index", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		s.tryCall(&err, w, r, protocol.DnUrl(), &api.BlockQuery{
			Major:      ptr(parseUint).fromRoute(p).try(&err, "index"),
			MinorRange: tryParseRangeFromQuery(&err, r, "minor_", true),
			OmitEmpty:  parseBool.allowMissing().fromQuery(r).try(&err, "omit_empty"),
		})
	})

	/// ***** Search ***** ///

	r.GET("/search/:id/anchor/:value", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.AnchorSearchQuery{
			Anchor:         parseBytes.fromRoute(p).try(&err, "value"),
			IncludeReceipt: parseBool.allowMissing().fromQuery(r).try(&err, "include_receipt"),
		})
	})

	r.GET("/search/:id/publicKey/:value", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		key := parser[address.Address](address.Parse).fromRoute(p).try(&err, "value")
		typ := parseSignatureType.allowMissing().fromQuery(r).try(&err, "type")
		if err != nil {
			responder{w, r}.write(nil, err)
			return
		}

		// If the value is raw hex...
		if unknown, ok := key.(*address.Unknown); ok {
			if typ == protocol.SignatureTypeUnknown {
				// and there's no type, interpret it as a hash
				responder{w, r}.write(s.Query(r.Context(), id, &api.PublicKeyHashSearchQuery{
					PublicKeyHash: unknown.Value,
				}))
			} else {
				// and there is a type, interpret it as a key
				responder{w, r}.write(s.Query(r.Context(), id, &api.PublicKeySearchQuery{
					PublicKey: unknown.Value,
					Type:      typ,
				}))
			}
			return
		}

		// If the value has a public key, search for it
		if pub, ok := key.GetPublicKey(); ok {
			responder{w, r}.write(s.Query(r.Context(), id, &api.PublicKeySearchQuery{
				PublicKey: pub,
				Type:      key.GetType(),
			}))
			return
		}

		// If the value has a public key hash, search for it
		if hash, ok := key.GetPublicKeyHash(); ok {
			responder{w, r}.write(s.Query(r.Context(), id, &api.PublicKeyHashSearchQuery{
				PublicKeyHash: hash,
			}))
			return
		}

		responder{w, r}.write(nil, errors.BadRequest.WithFormat("unsupported address %v", key))
	})

	r.GET("/search/:id/delegate/:value", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.DelegateSearchQuery{
			Delegate: parseUrl.fromRoute(p).try(&err, "value"),
		})
	})

	r.GET("/search/:id/messageHash/:value", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		id := parseUrl.fromRoute(p).try(&err, "id")
		s.tryCall(&err, w, r, id, &api.MessageHashSearchQuery{
			Hash: parseBytes32.fromRoute(p).try(&err, "value"),
		})
	})
	return nil
}

func (s Querier) tryCall(lastErr *error, w http.ResponseWriter, r *http.Request, scope *url.URL, query api.Query) {
	tryCall2(lastErr, w, r, s.Query, scope, query)
}

func tryParseRangeFromQuery(err *error, r *http.Request, prefix string, allowMissing bool) *api.RangeOptions {
	if allowMissing &&
		!r.URL.Query().Has(prefix+"start") &&
		!r.URL.Query().Has(prefix+"count") &&
		!r.URL.Query().Has(prefix+"expand") &&
		!r.URL.Query().Has(prefix+"from_end") {
		return nil
	}

	return &api.RangeOptions{
		Start:   parseUint.allowMissing().fromQuery(r).try(err, prefix+"start"),
		Count:   ptr(parseUint).allowMissing().fromQuery(r).try(err, prefix+"count"),
		Expand:  ptr(parseBool).allowMissing().fromQuery(r).try(err, prefix+"expand"),
		FromEnd: parseBool.allowMissing().fromQuery(r).try(err, prefix+"from_end"),
	}
}
