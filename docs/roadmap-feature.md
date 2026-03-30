# Roadmap CRD Feature

## Overview

The **Roadmap** Custom Resource Definition (CRD) provides a generic way to track timeline and roadmap information for various resource types in the controlplane system. This feature replaces and generalizes the old Java-based `ApiRoadmap` functionality.

**Key Benefits:**
- **Generic Design**: Supports multiple resource types (APIs, Events, and extensible to others)
- **File-Manager Integration**: Stores roadmap items externally, keeping CRD lightweight
- **Kubernetes-Native**: Follows standard Kubernetes patterns with proper reconciliation
- **REST API Access**: Full CRUD operations via HTTP endpoints

## Architecture

### Two-Layer Design

The Roadmap implementation follows a two-layer architecture similar to ApiSpecification:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Client / User                             │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                    REST API Server (rover-server)                │
│  • Receives: resourceName, resourceType, items array            │
│  • Marshals items to JSON                                       │
│  • Uploads to file-manager → gets file ID and hash              │
│  • Creates/updates CRD with file references                     │
│  • Downloads from file-manager when serving GET requests        │
│  • Handles duplicate removal (by resourceName + resourceType)   │
└──────────────┬──────────────────────────────┬───────────────────┘
               │                              │
               ▼                              ▼
┌─────────────────────────────┐  ┌───────────────────────────────┐
│  File-Manager                │  │  Kubernetes API (etcd)        │
│  • Stores items JSON         │  │  • Stores Roadmap CRD:        │
│  • Returns file ID & hash    │  │    - resourceName (string)    │
│  • Content-Type: JSON        │  │    - resourceType (enum)      │
│                              │  │    - roadmap (file ID)        │
│                              │  │    - hash (SHA-256)           │
└─────────────────────────────┘  └───────────┬───────────────────┘
                                              │
                                              ▼
                              ┌───────────────────────────────────┐
                              │  Kubernetes Operator (rover)      │
                              │  • Watches Roadmap CRD            │
                              │  • Minimal reconciliation         │
                              │  • Sets conditions                │
                              │  • No file-manager interaction    │
                              └───────────────────────────────────┘
```

### Data Storage

**CRD (Kubernetes)** stores:
- `resourceName`: Generic identifier (e.g., `/eni/my-api/v1` for APIs, `de.telekom.eni.event.v1` for Events)
- `resourceType`: Enum value (`"API"` or `"Event"`)
- `roadmap`: File ID reference from file-manager
- `hash`: SHA-256 hash for integrity verification

**File-Manager** stores:
- JSON array of roadmap items
- Format: `application/json`
- File ID pattern: `<env>--<resourceId>`

## CRD Specification

### Resource Type

```yaml
apiVersion: rover.cp.ei.telekom.de/v1
kind: Roadmap
metadata:
  name: my-api-roadmap
  namespace: test--eni--team
  labels:
    cp.ei.telekom.de/environment: test
spec:
  resourceName: "/eni/my-api/v1"      # Required - generic identifier
  resourceType: "API"                  # Required - enum: "API", "Event"
  roadmap: "test--eni--team--roadmap"  # Required - file ID reference
  hash: "abc123hash"                   # Required - SHA-256 hash
status:
  conditions:
    - type: Ready
      status: "True"
      reason: Ready
      message: "Roadmap is ready"
    - type: Processing
      status: "False"
      reason: Done
      message: "Roadmap processed"
```

### Roadmap Items Structure

The items array is stored in file-manager as JSON:

```json
[
  {
    "date": "Q1 2024",
    "title": "MVP Release",
    "description": "Initial release with core features",
    "titleUrl": "https://example.com/mvp"
  },
  {
    "date": "Q2 2024",
    "title": "Performance Improvements",
    "description": "Optimize response times"
  }
]
```

**Fields:**
- `date` (required): Timeline date (flexible format: "Q1 2024", "2024-03-15", etc.)
- `title` (required): Title of the roadmap item
- `description` (required): Detailed description
- `titleUrl` (optional): URL link for the title

## REST API Endpoints

All endpoints are secured with JWT authentication and follow the `/rover/v3/roadmaps` path.

### Create Roadmap

```http
POST /rover/v3/roadmaps
Content-Type: application/json
Authorization: Bearer <token>

{
  "resourceName": "/eni/my-api/v1",
  "resourceType": "API",
  "items": [...]
}
```

**Response:** `501 Not Implemented`

**Note:** POST is not implemented. This endpoint returns "Create not implemented. Use PUT /roadmaps/{resourceId} instead".

**To create a roadmap, use PUT** (see Update Roadmap below). This follows the declarative API pattern where PUT works for both creation and updates.

### Get Roadmap

```http
GET /rover/v3/roadmaps/{resourceId}
Authorization: Bearer <token>
```

**Response:** `200 OK` with full roadmap including items array

### List Roadmaps

```http
GET /rover/v3/roadmaps
GET /rover/v3/roadmaps?resourceType=API
GET /rover/v3/roadmaps?resourceType=Event
Authorization: Bearer <token>
```

**Query Parameters:**
- `resourceType` (optional): Filter by resource type (`API` or `Event`)
- `cursor` (optional): Pagination cursor

**Response:** `200 OK`
```json
{
  "items": [...],
  "_links": {
    "self": "...",
    "next": "..."
  }
}
```

### Update (or Create) Roadmap

```http
PUT /rover/v3/roadmaps/{resourceId}
Content-Type: application/json
Authorization: Bearer <token>

{
  "resourceName": "/eni/my-api/v1",
  "resourceType": "API",
  "items": [...]
}
```

**Response:** `200 OK` with roadmap (created or updated)

**Declarative PUT:** This endpoint follows the declarative API pattern:
- If the roadmap exists, it updates it
- If the roadmap doesn't exist, it creates it
- This is the **recommended way to create roadmaps** (not POST)

### Delete Roadmap

```http
DELETE /rover/v3/roadmaps/{resourceId}
Authorization: Bearer <token>
```

**Response:** `204 No Content`

**Note:** Always use the REST API for deletion. Direct `kubectl delete` will orphan files in file-manager.

## Usage Examples

### Creating a Roadmap for an API

Use PUT to create a new roadmap (declarative approach):

```bash
curl -X PUT https://api.example.com/rover/v3/roadmaps/eni--team--my-api \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "resourceName": "/eni/my-api/v1",
    "resourceType": "API",
    "items": [
      {
        "date": "Q1 2024",
        "title": "MVP Release",
        "description": "Initial release with core features",
        "titleUrl": "https://wiki.example.com/mvp"
      },
      {
        "date": "Q2 2024",
        "title": "Enhanced Features",
        "description": "Add advanced capabilities"
      }
    ]
  }'
```

### Creating a Roadmap for an Event

```bash
curl -X PUT https://api.example.com/rover/v3/roadmaps/eni--team--my-event \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "resourceName": "de.telekom.eni.myevent.v1",
    "resourceType": "Event",
    "items": [
      {
        "date": "Q2 2024",
        "title": "Event Schema Update",
        "description": "New fields added to event payload"
      }
    ]
  }'
```

### Listing Roadmaps for APIs Only

```bash
curl -X GET "https://api.example.com/rover/v3/roadmaps?resourceType=API" \
  -H "Authorization: Bearer $TOKEN"
```

### Updating a Roadmap

```bash
curl -X PUT https://api.example.com/rover/v3/roadmaps/eni--team--my-api \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "resourceName": "/eni/my-api/v1",
    "resourceType": "API",
    "items": [
      {
        "date": "Q3 2024",
        "title": "Updated Roadmap",
        "description": "Revised timeline and features"
      }
    ]
  }'
```

### Deleting a Roadmap

```bash
curl -X DELETE https://api.example.com/rover/v3/roadmaps/eni--team--my-api \
  -H "Authorization: Bearer $TOKEN"
```

## Kubernetes Direct Access (Not Recommended)

While you can interact with Roadmap CRDs directly via `kubectl`, **this is not recommended** because:
1. You need to manually upload to file-manager first
2. You must compute and provide the correct hash
3. Direct deletion orphans files in file-manager

However, for debugging or inspection:

```bash
# List roadmaps
kubectl get roadmaps -n test--eni--team

# Get roadmap details (metadata only, not items)
kubectl get roadmap my-api-roadmap -n test--eni--team -o yaml

# Check status
kubectl get roadmap my-api-roadmap -n test--eni--team \
  -o jsonpath='{.status.conditions}'
```

## Key Features

### 1. Generic Resource Support

The Roadmap CRD is not limited to APIs. It supports:
- **APIs**: Use API basePath as `resourceName` (e.g., `/eni/my-api/v1`)
- **Events**: Use event type name as `resourceName` (e.g., `de.telekom.eni.event.v1`)
- **Future**: Extensible to other resource types by adding to the `ResourceType` enum

### 2. Duplicate Removal

The system automatically ensures **one roadmap per resource**:
- When creating/updating, existing roadmaps with the same `resourceName + resourceType` combination are automatically deleted
- This prevents confusion and ensures data consistency
- Follows the behavior of the original Java-based ApiRoadmap system

### 3. Hash Optimization

The system optimizes file uploads:
- Computes SHA-256 hash of the items JSON
- Compares with stored hash before uploading
- Skips file-manager upload if content hasn't changed
- Reduces bandwidth and improves performance

### 4. File-Manager Integration

Roadmap items are stored externally:
- **Advantages**: Keeps CRD lightweight, supports large roadmaps, efficient storage
- **Format**: JSON array (not YAML)
- **Content-Type**: `application/json`
- **File ID Pattern**: `<environment>--<resourceId>`

## Migration from ApiRoadmap

The new Roadmap CRD replaces the old Java-based ApiRoadmap:

| Old (ApiRoadmap) | New (Roadmap) |
|------------------|---------------|
| API-specific only | Generic (API, Event, extensible) |
| `basePath` field | `resourceName` field (more generic) |
| No `resourceType` | `resourceType` enum field |
| Java system | Go/Kubernetes-native |

**Migration Steps:**
1. Export existing ApiRoadmap data
2. Transform to new format with `resourceType: "API"`
3. Create via REST API (duplicate removal handles cleanup)

## Limitations and Known Issues

### File Deletion with kubectl

**Issue:** Direct `kubectl delete roadmap <name>` orphans files in file-manager.

**Reason:** The Kubernetes operator doesn't have file-manager access (architectural separation).

**Workaround:** Always use REST API `DELETE /rover/v3/roadmaps/{id}` for proper cleanup.

**Future Improvement:** Could add finalizer with external cleanup service.

### Validation

The webhook validates CRD fields but not the items content. Item validation happens in the REST API layer.

## Files Reference

### Kubernetes CRD Layer (rover/)

| File | Description |
|------|-------------|
| `rover/api/v1/roadmap_types.go` | CRD type definitions |
| `rover/internal/controller/roadmap_controller.go` | Kubernetes controller |
| `rover/internal/handler/roadmap/handler.go` | Minimal handler (sets conditions) |
| `rover/internal/webhook/v1/roadmap_webhook.go` | Validation webhook |
| `rover/config/crd/bases/rover.cp.ei.telekom.de_roadmaps.yaml` | Generated CRD manifest |

### REST API Layer (rover-server/)

| File | Description |
|------|-------------|
| `rover-server/internal/controller/roadmap.go` | REST API controller with CRUD operations |
| `rover-server/internal/server/roadmap_server.go` | HTTP handlers for REST endpoints |
| `rover-server/internal/server/server.go` | Route registration |
| `rover-server/pkg/store/stores.go` | Store initialization |

### Tests

| File | Description |
|------|-------------|
| `rover/api/v1/roadmap_types_test.go` | CRD type tests |
| `rover/internal/controller/roadmap_controller_test.go` | Kubernetes controller tests |
| `rover/internal/handler/roadmap/handler_test.go` | Handler unit tests |
| `rover/internal/webhook/v1/roadmap_webhook_test.go` | Webhook validation tests |
| `rover-server/internal/controller/roadmap_test.go` | REST API controller tests |
| `rover-server/test/mocks/mocks_Roadmap.go` | Test mocks |
| `rover-server/test/mocks/data/roadmap.json` | Test data fixture |

## Testing

### Run Kubernetes Layer Tests

```bash
cd rover
make test
```

### Run REST API Tests

```bash
cd rover-server
make test
```

### Manual Testing

See [Usage Examples](#usage-examples) above for curl commands.

## Contributing

When extending the Roadmap feature:

1. **Adding Resource Types**: Update the `ResourceType` enum in `rover/api/v1/roadmap_types.go`
2. **Adding Validation**: Update webhook in `rover/internal/webhook/v1/roadmap_webhook.go`
3. **Adding Tests**: Follow existing test patterns in respective test files
4. **Update Documentation**: Keep this document up to date

## Support

For issues or questions:
- Check existing GitHub issues: https://github.com/telekom/controlplane/issues
- Create new issue with label `roadmap`

---

**Version:** 1.0.0
**Last Updated:** 2026-03-29
**Status:** Production Ready
