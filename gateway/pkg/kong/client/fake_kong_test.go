// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"sigs.k8s.io/yaml"

	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeKong is an in-memory Kong Admin API for routes, services, upstreams and targets.
type fakeKong struct {
	mu       sync.Mutex
	server   *httptest.Server
	entities map[string]map[string]map[string]any // collection -> id -> entity
	nextID   int
	calls    []string
}

func newFakeKong() *fakeKong {
	f := &fakeKong{entities: map[string]map[string]map[string]any{
		"routes": {}, "services": {}, "upstreams": {}, "targets": {}, "plugins": {},
	}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	DeferCleanup(f.server.Close)
	return f
}

func (f *fakeKong) api() *kong.ClientWithResponses {
	api, err := kong.NewClientWithResponses(f.server.URL)
	Expect(err).NotTo(HaveOccurred())
	return api
}

// add stores an entity and returns its ID.
func (f *fakeKong) add(collection string, entity map[string]any) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, _ := entity["id"].(string)
	if id == "" {
		f.nextID++
		id = fmt.Sprintf("%s-%d", strings.TrimSuffix(collection, "s"), f.nextID)
		entity["id"] = id
	}
	f.entities[collection][id] = entity
	return id
}

func (f *fakeKong) get(collection, id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.entities[collection][id]
}

func (f *fakeKong) names(collection string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	names := make([]string, 0, len(f.entities[collection]))
	for _, e := range f.entities[collection] {
		names = append(names, fmt.Sprint(e["name"]))
	}
	slices.Sort(names)
	return names
}

// writes returns the PUT, POST and DELETE calls.
func (f *fakeKong) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var writes []string
	for _, call := range f.calls {
		if !strings.HasPrefix(call, http.MethodGet) {
			writes = append(writes, call)
		}
	}
	return writes
}

func (f *fakeKong) resetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

func (f *fakeKong) find(collection, idOrName string) map[string]any {
	if e, ok := f.entities[collection][idOrName]; ok {
		return e
	}
	for _, e := range f.entities[collection] {
		if e["name"] == idOrName {
			return e
		}
	}
	return nil
}

func hasAllTags(entity map[string]any, filter string) bool {
	if filter == "" {
		return true
	}
	tags, _ := entity["tags"].([]any)
	for _, want := range strings.Split(filter, ",") {
		if !slices.Contains(tags, any(want)) {
			return false
		}
	}
	return true
}

func (f *fakeKong) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	collection := parts[0]
	if _, ok := f.entities[collection]; !ok {
		w.WriteHeader(http.StatusTeapot)
		return
	}

	if len(parts) == 3 && collection == "upstreams" && parts[2] == "targets" {
		f.serveTargets(w, r, parts[1])
		return
	}

	if len(parts) == 3 && collection == "services" && parts[2] == "routes" && r.Method == http.MethodGet {
		f.serveServiceRoutes(w, parts[1])
		return
	}

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		var data []any
		for _, e := range f.entities[collection] {
			if hasAllTags(e, r.URL.Query().Get("tags")) {
				data = append(data, e)
			}
		}
		if data == nil {
			data = []any{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	case len(parts) == 2 && r.Method == http.MethodGet:
		e := f.find(collection, parts[1])
		if e == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "Not found"})
			return
		}
		writeJSON(w, http.StatusOK, e)
	case len(parts) == 2 && r.Method == http.MethodPut:
		body := map[string]any{}
		data, err := io.ReadAll(r.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(data, &body)).To(Succeed())
		existing := f.find(collection, parts[1])
		if existing != nil {
			body["id"] = existing["id"]
		} else {
			f.nextID++
			body["id"] = fmt.Sprintf("%s-%d", strings.TrimSuffix(collection, "s"), f.nextID)
			if _, named := body["name"]; !named {
				body["name"] = parts[1]
			}
		}
		f.entities[collection][body["id"].(string)] = body
		writeJSON(w, http.StatusOK, body)
	case len(parts) == 2 && r.Method == http.MethodDelete:
		if e := f.find(collection, parts[1]); e != nil {
			delete(f.entities[collection], e["id"].(string))
		}
		w.WriteHeader(http.StatusNoContent)
	case len(parts) == 4 && collection == "upstreams" && parts[2] == "targets" && r.Method == http.MethodDelete:
		delete(f.entities["targets"], parts[3])
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusTeapot)
	}
}

func (f *fakeKong) serveServiceRoutes(w http.ResponseWriter, service string) {
	owner := f.find("services", service)
	if owner == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Not found"})
		return
	}
	data := []any{}
	for _, route := range f.entities["routes"] {
		if ref, _ := route["service"].(map[string]any); ref["id"] == owner["id"] {
			data = append(data, route)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (f *fakeKong) serveTargets(w http.ResponseWriter, r *http.Request, upstream string) {
	owner := f.find("upstreams", upstream)
	if owner == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Not found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		data := []any{}
		for _, t := range f.entities["targets"] {
			if up, _ := t["upstream"].(map[string]any); up["id"] == owner["id"] {
				data = append(data, t)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	case http.MethodPost:
		body := map[string]any{}
		data, err := io.ReadAll(r.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(data, &body)).To(Succeed())
		f.nextID++
		body["id"] = fmt.Sprintf("target-%d", f.nextID)
		body["upstream"] = map[string]any{"id": owner["id"]}
		f.entities["targets"][body["id"].(string)] = body
		writeJSON(w, http.StatusCreated, body)
	default:
		w.WriteHeader(http.StatusTeapot)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.WriteHeader(status)
	Expect(json.NewEncoder(w).Encode(body)).To(Succeed())
}

// kongSnapshot is a synthetic Kong export of a route written by the Java operator.
type kongSnapshot struct {
	Service map[string]any   `json:"service"`
	Route   map[string]any   `json:"route"`
	Plugins []map[string]any `json:"plugins"`
}

// loadSnapshot reads a snapshot and gives its route and service distinct IDs.
func loadSnapshot(file string) kongSnapshot {
	data, err := os.ReadFile(filepath.Join("..", "legacy", "testdata", "java-routes", file))
	Expect(err).NotTo(HaveOccurred())
	s := kongSnapshot{}
	Expect(yaml.Unmarshal(data, &s)).To(Succeed())
	s.Service["id"] = "legacy-service-id"
	s.Route["id"] = "legacy-route-id"
	s.Route["service"] = map[string]any{"id": "legacy-service-id"}
	return s
}
