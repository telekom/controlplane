## Phase B Task 2 Report: SpectreApplicationHandler (Tier B)

### Status: DONE

### Files Created/Modified

- **Modified:** `spectre/internal/handler/spectreapplication_handler.go` - Main handler with CreateOrUpdate flow
- **Created:** `spectre/internal/handler/spectreapplication_publisher.go` - Publisher + Subscriber creation logic
- **Created:** `spectre/internal/handler/spectreapplication_route.go` - SSE Route creation logic
- **Created:** `spectre/internal/handler/suite_test.go` - Ginkgo v2 test suite bootstrap
- **Created:** `spectre/internal/handler/spectreapplication_handler_test.go` - 12 unit tests

### Implementation Summary

**CreateOrUpdate flow:**
1. Resolve Application via `spec.application` TypedObjectRef -> get `appId` (Application.Name)
2. Resolve Zone from Application.Spec.Zone -> ensure ready
3. Get EventConfig for zone (via existing `util.GetEventConfig`)
4. Find EventStore in zone namespace (List + take first)
5. Ensure Publisher (EventType=`de.telekom.ei.listener.<appId>`, PublisherId=`"gateway"`, EventStore ref)
6. Ensure Subscriber (Publisher ref, SubscriberId=appId, Delivery mapped from spec)
7. If SSE: Ensure gateway Route (GatewayRef from zone.Status.Gateway, path `/sse/v1/<eventType>`, DisableAccessControl + DisableResponseBuffering, upstream from EventConfig.Spec.Local.ServerSendEventUrl)
8. Set Ready/NotReady condition based on `c.AllReady()`

**Key design decisions:**
- AppId = Application.Name (not ClientId) - matches the naming convention in `util.BuildListenerEventType`
- Zone resolved from Application.Spec.Zone (SpectreApplication has no zone in its own spec)
- SSE Route created directly in zone namespace (peer-domain pattern, same as event domain)
- Route name: `spectre-sse--<normalized-appId>` (distinct from event domain's `sse--<eventType>` to avoid collision)
- Route path: `/sse/v1/<eventType>` (matches event domain convention)
- Hostnames resolved via zone's default gateway preset (same as event domain)

### Test Coverage

12 tests covering:
- Publisher creation with correct EventType, PublisherId, EventStore ref
- Subscriber creation with correct delivery type (SSE vs callback)
- Subscriber callback URL set correctly for callback delivery
- SSE Route created with correct GatewayRef, paths, upstream, DisableAccessControl, DisableResponseBuffering
- SSE Route NOT created for callback delivery
- Ready condition set when all children ready
- NotReady condition set when AllReady returns false
- Error on missing Application
- Error on missing EventStore

Coverage: 82.8% of handler statements.

### Build Verification

```
cd spectre && make build test  # PASS
```
