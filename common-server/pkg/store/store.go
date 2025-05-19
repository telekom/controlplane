// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const DefaultPageSize = 100

// According to https://datatracker.ietf.org/doc/html/rfc6902
type PatchOp string

const (
	OpAdd     PatchOp = "add"
	OpRemove  PatchOp = "remove"
	OpReplace PatchOp = "replace"
	OpMove    PatchOp = "move"
	OpCopy    PatchOp = "copy"
	OpTest    PatchOp = "test"
)

func (op PatchOp) String() string {
	return string(op)
}

func (op PatchOp) IsValid() bool {
	switch op {
	case OpAdd, OpRemove, OpReplace, OpMove, OpCopy, OpTest:
		return true
	}
	return false
}

type Patch struct {
	Path  string  `json:"path"`
	Op    PatchOp `json:"op"`
	Value any     `json:"value"`
}

type FilterOp string

var (
	filterRegex = regexp.MustCompile(`^(.+)(==|!=|=~|~~)([\^\$a-zA-Z0-9-_.*+]+)$`)
)

const (
	OpEqual    FilterOp = "=="
	OpNotEqual FilterOp = "!="
	OpRegex    FilterOp = "=~"
	OpFullText FilterOp = "~~"
)

func (op FilterOp) String() string {
	return string(op)
}

func (op FilterOp) IsValid() bool {
	switch op {
	case OpEqual, OpNotEqual, OpRegex, OpFullText:
		return true
	}
	return false
}

type Filter struct {
	Path  string   `json:"path"`
	Op    FilterOp `json:"op"`
	Value string   `json:"value"`
}

func (f Filter) String() string {
	return fmt.Sprintf("%s%s%s", f.Path, f.Op, f.Value)
}

func ParseFilter(s string) (Filter, error) {
	matches := filterRegex.FindStringSubmatch(s)
	if len(matches) != 4 {
		return Filter{}, fmt.Errorf("invalid filter %s", s)
	}
	f := Filter{
		Path:  strings.ReplaceAll(matches[1], "@", "#"), // TODO: remove this hack
		Op:    FilterOp(matches[2]),
		Value: matches[3],
	}
	if !f.Op.IsValid() {
		return f, fmt.Errorf("invalid filter operator %s", f.Op)
	}
	return f, nil
}

type SortOrder string

func (o SortOrder) IsValid() bool {
	switch o {
	case SortOrderAsc, SortOrderDesc:
		return true
	}
	return false
}

const (
	SortOrderAsc  SortOrder = "asc"
	SortOrderDesc SortOrder = "desc"
)

type Sorter struct {
	Path  string
	Order SortOrder
}

func (s Sorter) String() string {
	return fmt.Sprintf("%s:%s", s.Path, s.Order)
}

func ParseSorter(s string) (Sorter, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return Sorter{}, errors.New("invalid sorter")
	}
	order := SortOrder(parts[1])
	if !order.IsValid() {
		return Sorter{}, fmt.Errorf("invalid sort order %s", parts[1])
	}
	return Sorter{
		Path:  parts[0],
		Order: order,
	}, nil
}

type ListOpts struct {
	Filters []Filter
	Sorters []Sorter
	Prefix  string
	Cursor  string
	Limit   int
}

func NewListOpts() ListOpts {
	return ListOpts{
		Limit: DefaultPageSize,
	}
}

func (o ListOpts) UrlEncoded() string {
	s := fmt.Sprintf("prefix=%s&cursor=%s&limit=%d", o.Prefix, o.Cursor, o.Limit)
	for _, filter := range o.Filters {
		s += fmt.Sprintf("&filter=%s%s%s", filter.Path, filter.Op, filter.Value)
	}
	for _, sorter := range o.Sorters {
		s += fmt.Sprintf("&sort=%s:%s", sorter.Path, sorter.Order)
	}
	return s
}

type ListResponse[T Object] struct {
	Links ListResponseLinks `json:"_links"`
	Items []T               `json:"items"`
}

type ListResponseLinks struct {
	Self string `json:"self"`
	Next string `json:"next"`
}

var _ Object = &unstructured.Unstructured{}

type Object interface {
	metav1.Object
	runtime.Object
}

type ObjectStore[T Object] interface {
	Info() (schema.GroupVersionResource, schema.GroupVersionKind)
	Ready() bool
	Get(ctx context.Context, namespace, name string) (T, error)
	List(ctx context.Context, opts ListOpts) (*ListResponse[T], error)
	Delete(ctx context.Context, namespace, name string) error
	CreateOrReplace(ctx context.Context, obj T) error
	Patch(ctx context.Context, namespace, name string, ops ...Patch) (T, error)
}

// ParseLimit parses a string into an integer, returning DefaultPageSize if the
// string is empty or cannot be parsed.
func ParseLimit(s string) int {
	limit, err := strconv.Atoi(s)
	if err != nil {
		return DefaultPageSize
	}
	if limit < 0 {
		return DefaultPageSize
	}
	return limit
}

func EqualGVK(expected, actual schema.GroupVersionKind) problems.Problem {
	fields := map[string]string{}
	if expected.GroupVersion() != actual.GroupVersion() {
		fields["apiVersion"] = fmt.Sprintf("expected %s, got %s", expected.GroupVersion(), actual.GroupVersion())
	}
	if expected.Kind != actual.Kind {
		fields["kind"] = fmt.Sprintf("expected %s, got %s", expected.Kind, actual.Kind)
	}
	if len(fields) > 0 {
		return problems.ValidationErrors(fields)
	}
	return nil
}

func EnforcePrefix(prefix any, listOpts *ListOpts) {
	if prefix == nil {
		return
	}
	ps, ok := prefix.(string)
	if !ok || prefix == "" {
		return
	}
	if listOpts.Prefix == "" {
		listOpts.Prefix = ps
		return
	}

	if !strings.HasPrefix(listOpts.Prefix, ps) {
		listOpts.Prefix = ps
	}
}
