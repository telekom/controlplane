// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package rollbacktest provides an in-memory fake of the Kong admin API endpoints used by rollback.
package rollbacktest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
)

// Request is a recorded write request.
type Request struct {
	Method string
	Path   string
	Body   map[string]any
}

func (r Request) String() string {
	return r.Method + " " + r.Path
}

// FakeKong is an in-memory Kong admin API serving routes, services, plugins, upstreams and targets.
// List endpoints return one item per page to exercise paging.
type FakeKong struct {
	mu        sync.Mutex
	Server    *httptest.Server
	Services  map[string]map[string]any
	Routes    map[string]map[string]any
	Plugins   map[string]map[string]any
	Upstreams map[string]map[string]any
	// Targets is keyed by upstream ID, then target ID.
	Targets map[string]map[string]map[string]any
	// Writes records all PUT and DELETE requests in order.
	Writes []Request
	// Fail maps "METHOD /path" to a status code returned instead of handling the request.
	Fail map[string]int
	// OnWrite may modify an entity before it is stored.
	OnWrite func(path string, entity map[string]any)
}

func New() *FakeKong {
	f := &FakeKong{
		Services:  map[string]map[string]any{},
		Routes:    map[string]map[string]any{},
		Plugins:   map[string]map[string]any{},
		Upstreams: map[string]map[string]any{},
		Targets:   map[string]map[string]map[string]any{},
		Fail:      map[string]int{},
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *FakeKong) Close() {
	f.Server.Close()
}

// WritePaths returns "METHOD /path" for every recorded write.
func (f *FakeKong) WritePaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.Writes))
	for i, w := range f.Writes {
		out[i] = w.String()
	}
	return out
}

func (f *FakeKong) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	var body map[string]any
	if r.Method == http.MethodPut {
		data, _ := io.ReadAll(r.Body)
		body = map[string]any{}
		if err := json.Unmarshal(data, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
			return
		}
	}
	if r.Method == http.MethodPut || r.Method == http.MethodDelete {
		var recorded map[string]any
		if body != nil {
			data, _ := json.Marshal(body)
			_ = json.Unmarshal(data, &recorded)
		}
		f.Writes = append(f.Writes, Request{Method: r.Method, Path: r.URL.Path, Body: recorded})
	}
	if status, ok := f.Fail[r.Method+" "+r.URL.Path]; ok {
		writeJSON(w, status, map[string]any{"message": "injected failure"})
		return
	}

	switch {
	case len(parts) == 2 && parts[0] == "services":
		f.entity(w, r, f.Services, parts[1], body, 1700000000)
	case len(parts) == 2 && parts[0] == "routes":
		f.entity(w, r, f.Routes, parts[1], body, 1700000000)
	case len(parts) == 2 && parts[0] == "plugins":
		f.entity(w, r, f.Plugins, parts[1], body, 1700000000)
	case len(parts) == 2 && parts[0] == "upstreams":
		key := parts[1]
		for id, u := range f.Upstreams {
			if u["name"] == key {
				key = id
			}
		}
		if r.Method == http.MethodDelete {
			delete(f.Targets, key)
		}
		f.entity(w, r, f.Upstreams, key, body, 1700000000)
	case len(parts) == 3 && parts[0] == "routes" && parts[2] == "plugins" && r.Method == http.MethodGet:
		var items []map[string]any
		for _, p := range f.Plugins {
			if route, ok := p["route"].(map[string]any); ok && route["id"] == parts[1] {
				items = append(items, p)
			}
		}
		f.page(w, r, items)
	case len(parts) == 3 && parts[0] == "upstreams" && parts[2] == "targets" && r.Method == http.MethodGet:
		var items []map[string]any
		for _, t := range f.Targets[parts[1]] {
			items = append(items, t)
		}
		f.page(w, r, items)
	case len(parts) == 4 && parts[0] == "upstreams" && parts[2] == "targets":
		if f.Targets[parts[1]] == nil {
			f.Targets[parts[1]] = map[string]map[string]any{}
		}
		f.entity(w, r, f.Targets[parts[1]], parts[3], body, 1700000000.123)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found"})
	}
}

func (f *FakeKong) entity(w http.ResponseWriter, r *http.Request, coll map[string]map[string]any, id string, body map[string]any, ts any) {
	switch r.Method {
	case http.MethodGet:
		e, ok := coll[id]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, e)
	case http.MethodPut:
		body["id"] = id
		body["created_at"] = ts
		if f.OnWrite != nil {
			f.OnWrite(r.URL.Path, body)
		}
		coll[id] = body
		writeJSON(w, http.StatusOK, body)
	case http.MethodDelete:
		if _, ok := coll[id]; !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found"})
			return
		}
		delete(coll, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// page returns one item per page, sorted by ID, using the item index as offset.
func (f *FakeKong) page(w http.ResponseWriter, r *http.Request, items []map[string]any) {
	slices.SortFunc(items, func(a, b map[string]any) int {
		return strings.Compare(a["id"].(string), b["id"].(string))
	})
	idx := 0
	if off := r.URL.Query().Get("offset"); off != "" {
		for i, it := range items {
			if it["id"] == off {
				idx = i
			}
		}
	}
	res := map[string]any{"data": []map[string]any{}}
	if idx < len(items) {
		res["data"] = items[idx : idx+1]
		if idx+1 < len(items) {
			res["offset"] = items[idx+1]["id"]
		}
	}
	writeJSON(w, http.StatusOK, res)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
