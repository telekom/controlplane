<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Application Processing Condition

## Problem

The discovery API reports every healthy Application as unprocessed:

```json
"status": {
  "state": "none",
  "processingState": "none",
  "warnings": [{ "message": "Processing condition not found" }]
}
```

The old Java implementation (prod) returned `"state": "complete", "processingState": "done"` for the same resource.

The response is valid per the spec (`common.yaml` `State` / `ProcessingState` enums both allow `none`), but it is wrong: the Application is ready.

## Root cause

- `discovery-server/internal/mapper/status/status.go` (`fillStateInfo`) derives the status from the `Processing` condition first. When it is missing, it returns `none`/`none` plus the warning and never looks at `Ready`.
- `common/pkg/controller/controller.go` does not set `Processing=Done` on success. Each handler must set it.
- The api and event handlers do (e.g. `api/internal/handler/apiexposure/handler.go`: `NewDoneProcessingCondition`).
- `application/internal/handler/application/handler.go` only sets `Ready`/`NotReady`. A `Processing` condition only appears when a blocked error is handled (`Processing=False, Reason=Blocked`).
- The discovery-server fixture `test/mocks/data/application.json` contains a `Processing: Done` condition the real controller never writes, so the tests did not catch this.

## Fix

In `application/internal/handler/application/handler.go` (`CreateOrUpdate`), set `Processing` next to every `Ready` change, following the apiexposure handler:

| Branch | `Ready` (unchanged) | `Processing` (new) |
| --- | --- | --- |
| `c.AnyChanged()` | `NotReady`, sub-resource changed | `NewProcessingCondition(ReasonProcessing, msg)` |
| primary identity client not ready | `NotReady`, waiting for client | `NewProcessingCondition(ReasonProcessing, msg)` |
| all sub-resources up to date | `Ready`, provisioned | `NewDoneProcessingCondition(msg)` |

Resulting discovery status:

| Application state | `state` / `processingState` |
| --- | --- |
| converging | `blocked` / `processing` |
| ready | `complete` / `done` |
| blocked error | `blocked` / `done` + warning (unchanged) |

No discovery-server change is needed. Existing Applications are fixed on their next reconcile (periodic requeue with jitter).

## Not doing

- A discovery-server fallback to `Ready` when `Processing` is missing. It would hide the same bug in other controllers.

## Validation

- Application handler tests: assert `Processing` is `True/Processing` while converging and `False/Done` when ready.
- `make test` in `application`.
- On playground: `kubectl get application <name> -n <ns> -o jsonpath='{.status.conditions}'` shows `Processing: Done`, and `GET /application/v2/applications/<id>` returns `complete`/`done`.
