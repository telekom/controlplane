// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tree

import (
	"strings"
	"sync"
)

type ResourceHierarchy interface {
	AddChild(parent GVK, child TreeResourceInfo)
	GetChildren(parent GVK) []TreeResourceInfo
	GetOwner(child GVK) (TreeResourceInfo, bool)
}

// DynamicResourceHierachy determines the hierarchy at runtime
// with minimal manual configuration
type DynamicResourceHierachy struct {
}

type StaticResourceHierarchy struct {
	lock       sync.RWMutex
	Owned      map[string]Set `json:"owned" yaml:"owned"`
	Referenced map[string]Set `json:"referenced" yaml:"referenced"`
}

func NewStaticResourceHierarchy() *StaticResourceHierarchy {
	return &StaticResourceHierarchy{
		Owned:      map[string]Set{},
		Referenced: map[string]Set{},
	}
}

func (h *StaticResourceHierarchy) makeId(gvk GVK) string {
	return gvk.GetAPIVersion() + "." + gvk.GetKind()
}

func (h *StaticResourceHierarchy) parseId(id string) TreeResourceInfo {
	// Find first "." after "/" to split apiVersion from Kind
	slashIdx := strings.LastIndex(id, "/")
	dotIdx := strings.Index(id[slashIdx+1:], ".") + slashIdx + 1

	return TreeResourceInfo{
		APIVersion: id[:dotIdx],
		Kind:       id[dotIdx+1:],
	}
}

func (h *StaticResourceHierarchy) AddChild(parent GVK, child TreeResourceInfo) {
	h.lock.Lock()
	defer h.lock.Unlock()

	id := h.makeId(parent)
	if _, ok := h.Owned[id]; !ok {
		h.Owned[id] = map[TreeResourceInfo]bool{}
	}
	h.Owned[id][child] = true
}

func (h *StaticResourceHierarchy) GetChildren(parent GVK) []TreeResourceInfo {
	h.lock.RLock()
	defer h.lock.RUnlock()

	id := h.makeId(parent)
	children := []TreeResourceInfo{}
	for child := range h.Owned[id] {
		children = append(children, child)
	}
	return children
}

func (h *StaticResourceHierarchy) GetOwner(child GVK) (ownerInfo TreeResourceInfo, found bool) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	childId := h.makeId(child)

	for parentId, children := range h.Owned {
		for childInfo := range children {
			if h.makeId(childInfo) == childId {
				return h.parseId(parentId), true
			}
		}
	}

	return
}
