<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

# API Generation

```
  application-openapi.yaml ──┐
  stargate-openapi.yaml ─────┼── openapi-merge.json ──▶ uber-openapi.yaml
  event-openapi.yaml ────────┤
  common.yaml ───────────────┘
                                         │
                                         ▼
                              tools/generate.go
                              (go:generate directive)
                                         │
                                         ▼
                              tools/server.yaml
                              (oapi-codegen config)
                                         │
                                         ▼
                            internal/api/server.gen.go
                                (ALL types combined)
```

```bash
# Create merged OpenAPI spec called uber-openapi.yaml
openapi-merge-cli --config openapi-merge.json

# Replace all $ref to common.yaml with '#/ to make it work with oapi-codegen
sed -i "s|common\.yaml#/\(.*\)|'#/\1'|g" uber-openapi.yaml

cd ..
# Generate Go server code from the merged OpenAPI spec
go generate ./...
```