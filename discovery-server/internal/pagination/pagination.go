// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package pagination

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/common-server/pkg/store"

	"github.com/telekom/controlplane/discovery-server/internal/api"
)

const (
	// defaultLimit is the page size when the caller does not specify one.
	defaultLimit = 20
	// fetchBatchSize is the cursor-page size used when draining the store.
	fetchBatchSize = 500
)

// PaginatedResult holds the offset/limit paginated slice together with the
// paging metadata and HATEOAS links expected by the OpenAPI contract.
type PaginatedResult[T any] struct {
	Items  []T
	Paging api.Paging
	Links  api.Links
}

// FetchAll retrieves every item from an ObjectStore by following cursor-based
// pagination to completion. The caller provides the initial ListOpts (with
// Prefix / Filters already set); this function overrides Limit and Cursor.
func FetchAll[T store.Object](ctx context.Context, s store.ObjectStore[T], opts store.ListOpts) ([]T, error) {
	opts.Limit = fetchBatchSize
	opts.Cursor = ""

	var all []T
	for {
		resp, err := s.List(ctx, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Items...)
		if resp.Links.Next == "" {
			break
		}
		// Links.Next is a raw badger key (e.g. "dev--eni--hyperion/my-exposure/"),
		// not a URL. Use it directly as the cursor for the next page.
		opts.Cursor = resp.Links.Next
	}
	return all, nil
}

// Paginate applies offset/limit semantics to a fully-loaded slice and returns
// the page together with Paging metadata and HATEOAS Links.
//
// basePath is the request path used to build link URLs (e.g.
// "/applications/myapp/apiexposures").
// A limit of 0 is treated as "use default".
func Paginate[T any](items []T, offset, limit int32, basePath string) PaginatedResult[T] {
	total := len(items)
	off := int(offset)
	lim := int(limit)

	if lim <= 0 {
		lim = defaultLimit
	}
	if off < 0 {
		off = 0
	}
	if off > total {
		off = total
	}
	end := off + lim
	if end > total {
		end = total
	}

	// Pages are 1-based per the OpenAPI spec (minimum: 1).
	page := off/lim + 1
	lastPage := 1
	if total > 0 {
		lastPage = (total-1)/lim + 1
	}

	return PaginatedResult[T]{
		Items: items[off:end],
		Paging: api.Paging{
			Total:    total,
			Page:     page,
			LastPage: lastPage,
		},
		Links: buildLinks(basePath, off, lim, total),
	}
}

func buildLinks(basePath string, offset, limit, total int) api.Links {
	makeURL := func(off int) string {
		return fmt.Sprintf("%s?offset=%d&limit=%d", basePath, off, limit)
	}

	lastOffset := 0
	if total > 0 {
		lastOffset = ((total - 1) / limit) * limit
	}

	links := api.Links{
		Self:  makeURL(offset),
		First: makeURL(0),
		Last:  makeURL(lastOffset),
	}

	if offset+limit < total {
		links.Next = makeURL(offset + limit)
	}
	if offset-limit >= 0 {
		links.Prev = makeURL(offset - limit)
	}

	return links
}
