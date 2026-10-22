<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

# API Generation

```
  metadata-openapi.yaml ─────┐
  stargate-openapi.yaml ─────┤
  application-openapi.yaml ──┼── openapi-merge.json ──▶ uber-openapi.yaml
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

`metadata-openapi.yaml` is the first merge input and contains no paths. The merger takes
`info` and `servers` from the first input that defines them, so this file sets the metadata
of `uber-openapi.yaml` (title `Discovery API`, legacy base URL
`https://api.telekom.de/stargate/v2`). Discovery is the new name of the API; Stargate is the
legacy name and is still supported. `stargate-openapi.yaml` stays unchanged as the legacy
source and still provides the `security` requirement.

Run `make generate` here to merge and regenerate the Go code.

```bash
# Create merged OpenAPI spec called uber-openapi.yaml
openapi-merge-cli --config openapi-merge.json

# Replace all $ref to common.yaml with '#/ to make it work with oapi-codegen
sed -i "s|common\.yaml#/\(.*\)|'#/\1'|g" uber-openapi.yaml

cd ..
# Generate Go server code from the merged OpenAPI spec
go generate ./...
```