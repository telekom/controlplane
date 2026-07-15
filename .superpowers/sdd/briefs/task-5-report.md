# Task 5 Report: RBAC markers + verify end-to-end

## Status: DONE

## Changes

### `spectre/internal/controller/listener_controller.go`
Added 7 cross-domain RBAC markers after the existing 3 spectre-own markers:
- `core` events (create, patch) — for recording Kubernetes events
- `pubsub.cp.ei.telekom.de` publishers + subscribers (full CRUD)
- `gateway.cp.ei.telekom.de` routelisteners + routes (full CRUD)
- `approval.cp.ei.telekom.de` approvalrequests + approvals (full CRUD)
- `application.cp.ei.telekom.de` applications (read-only)
- `admin.cp.ei.telekom.de` zones (read-only)
- `event.cp.ei.telekom.de` eventconfigs (read-only)

### `spectre/internal/controller/spectreapplication_controller.go`
Already had correct markers (3 spectre-own). No changes needed.

## Generated Output

`spectre/config/rbac/role.yaml` regenerated via `make manifests`. Contains all expected ClusterRole rules with controller-gen merging both controllers' markers into a single role.

## Verification

- `make manifests` — OK (regenerated role.yaml)
- `make build` — OK (compiles cleanly)
- `make test` — OK (all tests pass)
