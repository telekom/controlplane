# Phase C - Task 1 Report: Extend RoverSpec with Listeners fields

## Status: DONE

## Changes Made

### New file: `rover/api/v1/listener_types.go`
- `RoverListener` struct: consumer, provider, apiBasePath, eventType, requestFilter, responseFilter, eventFilter
- `ListenerFilter` struct: trigger (map[string]string), payload ([]string)
- `ListenerSubscription` struct: deliveryType (enum: callback, server_sent_event; default: server_sent_event), callback (uri)

### Modified: `rover/api/v1/rover_types.go`
- Added to `RoverSpec`:
  - `Listeners []RoverListener` (json: listeners, optional)
  - `ListenerSubscription *ListenerSubscription` (json: listenerSubscription, optional)
- Added to `RoverStatus`:
  - `SpectreApplications []types.ObjectRef` (json: spectreApplications)
  - `SpectreListeners []types.ObjectRef` (json: spectreListeners)

### Auto-generated
- `rover/api/v1/zz_generated.deepcopy.go` — DeepCopy methods for RoverListener, ListenerFilter, ListenerSubscription
- `rover/config/crd/bases/rover.cp.ei.telekom.de_rovers.yaml` — CRD updated with new spec/status fields

## Verification
- `make manifests generate build` passed successfully from `rover/`
- CRD contains enum validation (callback, server_sent_event) with default server_sent_event
- DeepCopy generated for all three new types including map and slice fields
