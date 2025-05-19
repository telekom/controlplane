package openapi

const (
	metadataSchema = `{
		"type": "object",
		"required": ["name", "namespace", "uid", "creationTimestamp", "resourceVersion"],
		"properties": {
			"name": {
				"type": "string"
			},
			"namespace": {
				"type": "string"
			},
			"uid": {
				"type": "string"
			},
			"creationTimestamp": {
				"type": "string"
			},
			"deletionTimestamp": {
				"type": "string"
			},
			"resourceVersion": {
				"type": "string"
			},
			"labels": {
				"type": "object",
				"additionalProperties": {
					"type": "string"
				}
			},
			"annotations": {
				"type": "object",
				"additionalProperties": {
					"type": "string"
				}
			}
		}
	}`

	crdSchema = `{
		"type": "object",
		"required": ["apiVersion", "kind", "metadata", "spec", "status"],
		"properties": {
			"apiVersion": {
				"type": "string"
			},
			"kind": {
				"type": "string"
			},
			"metadata": {
				"$ref": "%s"
			},
			"spec": {
				"$ref": "%s"
			},
			"status": {
				"$ref": "%s"
			}
		}
	}`

	patchRequestBodySchema = `{
		"type": "array",
		"items": {
			"type": "object",
			"required": ["op", "path"],
			"properties": {
				"op": {
					"type": "string"
				},
				"path": {
					"type": "string"
				},
				"value": {
					"type": "string"
				}
			}
		}
	}`

	ApiProblemSchema = `{
	"type": "object",
	"required": ["type", "status", "title"],
	"description": "Based on https://www.rfc-editor.org/rfc/rfc9457.html",
	"properties": {
		"type": {
			"type": "string"
		},
		"status": {
			"type": "integer"
		},
		"title": {
			"type": "string"
		},
		"detail": {
			"type": "string"
		},
		"instance": {
			"type": "string"
		},
		"fields": {
			"type": "array",
			"items": {
				"type": "object",
				"required": ["field", "detail"],
				"properties": {
					"field": {
						"type": "string"
					},
					"detail": {
						"type": "string"
					}
				}
			}
		}
	}
}`
)
