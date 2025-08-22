---
sidebar_position: 2
---

# Helm Charts

Helm is used for packaging, versioning, and deploying the Control Plane components to Kubernetes clusters.

## Overview

[Helm](https://helm.sh/) is the package manager for Kubernetes. The Control Plane uses Helm to:

- Package components as charts
- Manage application configuration
- Simplify deployment process
- Handle upgrades and rollbacks

## Chart Structure

The Control Plane components follow a standard Helm chart structure:

```
common-server/
├── Chart.yaml             # Chart metadata
├── values.yaml            # Default values
└── templates/             # Kubernetes manifests templates
    ├── _helpers.tpl       # Template helpers
    ├── deployment.yaml    # Deployment template
    ├── service.yaml       # Service template
    ├── configmap.yaml     # ConfigMap template
    ├── ingress.yaml       # Ingress template
    ├── servicemonitor.yaml # Prometheus ServiceMonitor
    └── rbac/              # RBAC templates
        ├── binding.yaml
        ├── clusterrole.yaml
        └── serviceaccount.yaml
```

## Chart.yaml Example

```yaml
apiVersion: v2
name: common-server
description: Common Server components for the Control Plane
type: application
version: 0.1.0
appVersion: "0.1.0"
dependencies:
  - name: postgresql
    version: 10.3.18
    repository: https://charts.bitnami.com/bitnami
    condition: postgresql.enabled
```

## Values.yaml Example

```yaml
# Default configuration values
replicaCount: 1

image:
  repository: controlplane/common-server
  tag: latest
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 80

ingress:
  enabled: false
  className: nginx
  hosts:
    - host: controlplane.example.com
      paths:
        - path: /
          pathType: Prefix

resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 50m
    memory: 64Mi

serviceMonitor:
  enabled: true
  interval: 15s

postgresql:
  enabled: false
  auth:
    username: controlplane
    database: controlplane
```

## Template Example

```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "common-server.fullname" . }}
  labels:
    {{- include "common-server.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "common-server.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "common-server.selectorLabels" . | nindent 8 }}
    spec:
      serviceAccountName: {{ include "common-server.serviceAccountName" . }}
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: 8080
              protocol: TCP
          {{- if .Values.configMap.enabled }}
          volumeMounts:
            - name: config-volume
              mountPath: /etc/controlplane
          {{- end }}
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
      {{- if .Values.configMap.enabled }}
      volumes:
        - name: config-volume
          configMap:
            name: {{ include "common-server.fullname" . }}-config
      {{- end }}
```

## Deployment Process

To deploy a Control Plane component using Helm:

```bash
# Add the Control Plane Helm repository
helm repo add controlplane https://charts.controlplane.example.com

# Update repositories
helm repo update

# Install a component
helm install file-manager controlplane/file-manager \
  --namespace controlplane-system \
  --create-namespace \
  --values custom-values.yaml

# Upgrade an existing installation
helm upgrade file-manager controlplane/file-manager \
  --namespace controlplane-system \
  --values custom-values.yaml

# Rollback to a previous version
helm rollback file-manager 1 \
  --namespace controlplane-system
```

## Template Helpers

The Control Plane Helm charts use common template helpers:

```
{{/* Generate basic labels */}}
{{- define "common-server.labels" -}}
helm.sh/chart: {{ include "common-server.chart" . }}
{{ include "common-server.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/* Selector labels */}}
{{- define "common-server.selectorLabels" -}}
app.kubernetes.io/name: {{ include "common-server.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
```

## Chart Dependencies

The Control Plane uses Helm chart dependencies to include common infrastructure:

```bash
# Update dependencies
helm dependency update

# Build dependencies
helm dependency build
```

## Best Practices

The Control Plane Helm charts follow these best practices:

- Parameterize all configurable values
- Use conditionals for optional features
- Include NOTES.txt with usage instructions
- Validate chart templates with `helm lint`
- Use helper functions for common patterns
- Maintain consistent label schema