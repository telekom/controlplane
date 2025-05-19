package tree

import (
	"sync"
)

type Set map[TreeResourceInfo]bool

type ResourceHierarchy struct {
	lock       sync.RWMutex
	Owned      map[string]Set
	Referenced map[string]Set
}

func (h *ResourceHierarchy) AddChild(parent GVK, child TreeResourceInfo) {
	h.lock.Lock()
	defer h.lock.Unlock()

	id := parent.GetAPIVersion() + "." + parent.GetKind()
	if _, ok := h.Owned[id]; !ok {
		h.Owned[id] = map[TreeResourceInfo]bool{}
	}
	h.Owned[id][child] = true
}

func (h *ResourceHierarchy) GetChildren(parent GVK) []TreeResourceInfo {
	h.lock.RLock()
	defer h.lock.RUnlock()

	id := parent.GetAPIVersion() + "." + parent.GetKind()
	children := []TreeResourceInfo{}
	for child := range h.Owned[id] {
		children = append(children, child)
	}
	return children
}

var LookupResourceHierarchy = &ResourceHierarchy{
	Owned:      map[string]Set{},
	Referenced: map[string]Set{},
}

func init() {
	roverRef := TreeResourceInfo{APIVersion: "rover.apimanager.telekom.de/v1", Kind: "Rover"}

	LookupResourceHierarchy.AddChild(roverRef, TreeResourceInfo{APIVersion: "stargate.cp.ei.telekom.de/v1", Kind: "ApiExposure"})
	LookupResourceHierarchy.AddChild(roverRef, TreeResourceInfo{APIVersion: "stargate.cp.ei.telekom.de/v1", Kind: "ApiSubscription"})
}
