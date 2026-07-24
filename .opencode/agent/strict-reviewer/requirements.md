<!--
Copyright 2026 Deutsche Telekom IT GmbH

SPDX-License-Identifier: CC0-1.0
-->

# Gateway Replacement — Requirements Specification

> This is a draft to be discussed in the team.

**Project:** Kong + Jumper Replacement
**Date:** 2026-06-02
**Author:** Stargate / O28M Team — Deutsche Telekom AG
**Status:** Draft — Hackathon Preparation

## 1. Motivation

The current Stargate API gateway runs as a 3-container pod (Kong + Jumper + Issuer Service) built on Kong OSS 3.9.1 with a Spring Cloud Gateway sidecar (Jumper) and a Go-based token validation service (Issuer Service). This architecture has served well, but introduces:

- Operational complexity from maintaining three separate codebases (Lua, Java, Go) in a single pod
- Inter-process latency between Kong and Jumper for every proxied request
- Licensing risk — Kong OSS licensing and feature restrictions must be monitored; the replacement must use a clearly open-source-licensed gateway
- Limited extensibility — Lua plugin development in Kong is niche; Jumper's Spring Cloud Gateway filters require Java expertise

The goal is to identify a single-process gateway that consolidates core routing and token handling, supported by purpose-built microservices for specialized features.

## 2. Architecture Principles

| Principle | Description |
|---|---|
| Single-process core | All request processing (routing, auth, token generation, rate limiting) runs in one process to minimize latency |
| Feature microservices | Specialized capabilities (e.g., Spectre/event mirroring) are implemented as separate microservices, invoked via standard gateway mechanisms (mirroring, external processing) |
| Stateless gateway | No primary database required; configuration via Kubernetes CRDs or declarative files. Shared state (rate limit counters, token cache) via optional Redis or in-memory |
| Control plane separation | Rover remains the customer-facing control plane; it translates API configurations into whatever config format the new gateway requires (CRDs, declarative config, API calls) |
| Open-source licensing | The gateway core must be available under a permissive or copyleft open-source license (Apache 2.0, MIT, MPL, GPL). No vendor lock-in or proprietary feature gates |

## 3. Functional Requirements

### 3.1 Routing and Traffic Management

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| RT-01 | Path-based routing with prefix, exact, and regex matching | Must | Core routing |
| RT-02 | Host-based routing (virtual hosts) | Must | Multiple gateway hostnames per zone |
| RT-03 | Header-based routing | Should | Conditional routing based on request headers |
| RT-04 | Weighted load balancing across upstream targets | Must | Required for mesh zone failover and canary deployments |
| RT-05 | Active and/or passive health checks on upstreams | Must | Detect unhealthy upstream targets; essential for cross-zone mesh routing |
| RT-06 | Request/response mirroring (traffic shadowing) | Must | Native capability to mirror traffic to configurable endpoints (replaces Spectre embedding) |
| RT-07 | Canary / blue-green deployment support | Should | Progressive rollout of upstream versions |
| RT-08 | Request retries with configurable backoff | Should | Retry on upstream failure |
| RT-09 | Circuit breaker support | Should | Prevent cascading failures |
| RT-10 | Configurable load-balancing algorithms | Must | Support round-robin, least-connections, consistent hashing (cookie, header, IP), and random algorithms per upstream/service |
| RT-11 | Upstream weight configuration | Must | Assign weights to individual upstream targets for proportional traffic distribution; supports canary, zone-preference, and gradual migration scenarios |
| RT-12 | Session affinity / sticky sessions | Should | Route repeat requests from the same client to the same upstream target via cookie or header-based affinity |
| RT-13 | Slow-start for new upstream targets | Should | Gradually ramp traffic to newly added upstream targets to avoid cold-start overload |
| RT-14 | Connection and request limits per upstream target | Should | Configurable max connections and max pending requests per target to prevent single-target overload |
| RT-15 | Request/response wiretapping (tap/inspection) | Must | Ability to tap (copy) full request and response payloads including headers and body to a configurable sink (file, HTTP endpoint, logging backend) for debugging, auditing, or compliance purposes. Must be activatable per route, per consumer, or globally without impacting live traffic performance |
| RT-16 | Conditional wiretap activation | Should | Activate wiretapping based on dynamic criteria (header presence, consumer identity, percentage sampling, specific status codes) to limit overhead in production |

### 3.2 Authentication and Token Handling

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| AU-01 | JWT validation with JWKS endpoint support | Must | Validate incoming consumer tokens against configurable issuers (replaces jwt-keycloak plugin) |
| AU-02 | Issuer allowlist and blocklist per route | Must | Per-route allowed issuers; zone-wide issuer blocklist for emergency revocation |
| AU-03 | OneToken generation — generate a new JWT from incoming token claims + request context, signed with gateway's private key | Must | Core last-mile security feature. Token must contain: sub, clientId, azp, originZone, originStargate, env, operation, requestPath, iss, exp, iat. Configurable additional claims (scope, publisherId, subscriberId) |
| AU-04 | Key rotation support for token signing (active, next, previous keys) | Must | Seamless key rotation without downtime |
| AU-05 | Upstream OAuth token fetching — client_credentials grant | Must | Fetch tokens from external IdPs before forwarding upstream; with token caching and in-flight request coalescing |
| AU-06 | Upstream OAuth — JWT bearer assertion (private_key_jwt) | Must | Sign JWT with client private key for external IdP authentication |
| AU-07 | Upstream OAuth — password grant | Should | Legacy support for external IdPs requiring password grant |
| AU-08 | Upstream OAuth — refresh_token grant | Should | Token refresh support for long-lived sessions |
| AU-09 | Per-consumer credential overrides for upstream OAuth | Must | Different consumers may use different client credentials for the same upstream provider |
| AU-10 | BasicAuth forwarding to upstream | Must | Forward Basic Auth credentials (per-consumer or default) to upstream providers |
| AU-11 | ACL / consumer group authorization | Must | Restrict route access by consumer group |
| AU-12 | Access token forwarding (passthrough) | Must | Option to forward the original consumer token unchanged to upstream |

### 3.3 Header Manipulation

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| HD-01 | Add, remove, rename, rewrite headers on request and response | Must | Full request/response header transformation |
| HD-02 | Conditional header manipulation (per-consumer, per-route) | Must | Different header rules based on consumer identity or route |
| HD-03 | Generic header-to-header mapping | Must | Configurable mapping of any incoming header to any outgoing header (generalizes X-Token-Exchange) |
| HD-04 | Standard X-Forwarded-* header handling | Must | Proper X-Forwarded-For, X-Forwarded-Proto, X-Forwarded-Host, X-Forwarded-Port, X-Forwarded-Path |
| HD-05 | Custom enrichment headers | Must | Add gateway context headers: origin zone, origin stargate, environment, consumer identity |
| HD-06 | Header sanitization (remove internal headers before upstream) | Must | Strip internal gateway headers (e.g., jumper_config, routing_config) before forwarding |

### 3.4 Rate Limiting

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| RL-01 | Per-consumer rate limiting | Must | Limit requests per consumer identity (second, minute, hour) |
| RL-02 | Per-service/route rate limiting | Must | Limit total requests to a service regardless of consumer |
| RL-03 | Multi-dimensional rate limiting (consumer + service simultaneously) | Must | Both limits enforced together; standard RateLimit- and X-RateLimit- response headers |
| RL-04 | Configurable rate limit periods (second, minute, hour) | Must | At minimum: second, minute, hour granularity |
| RL-05 | Fault-tolerant mode | Should | Continue serving when rate limit backend (Redis) is unavailable |
| RL-06 | Local and distributed rate limiting (in-memory and Redis) | Should | Local counters for single-pod, Redis for cross-pod consistency |
| RL-07 | Consumer omission for specific identities | Should | Exclude specific consumers (e.g., "gateway" internal consumer) from rate limit counting |

### 3.5 Request Validation and Transformation

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| RV-01 | Request size limiting | Must | Configurable max payload size per route |
| RV-02 | Request body transformation | Should | Transform request body before forwarding (e.g., JSON manipulation) |
| RV-03 | Response body transformation | Should | Transform response body before returning to consumer |
| RV-04 | URL path rewriting | Must | Rewrite request path before forwarding upstream |

## 4. Non-Functional Requirements

### 4.1 Performance

| ID | Requirement | Priority | Target |
|---|---|---|---|
| PF-01 | Sub-millisecond added gateway latency (p99) | Must | < 1ms added latency for passthrough requests |
| PF-02 | High throughput per pod | Must | > 50,000 RPS per pod for simple proxy |
| PF-03 | Minimal GC pauses / predictable latency | Must | No long tail latency from garbage collection |
| PF-04 | Lower resource footprint than current setup | Should | Current: 1500m CPU + 3500Mi memory per pod (Kong alone) |
| PF-05 | Efficient connection pooling to upstreams | Must | HTTP/1.1 keep-alive and HTTP/2 multiplexing |
| PF-06 | Efficient token caching with in-flight coalescing | Must | Avoid duplicate token fetches for the same credentials |

### 4.2 Protocol Support

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| PR-01 | HTTP/1.1 | Must | |
| PR-02 | HTTP/2 (h2, h2c) | Must | Both TLS and cleartext |
| PR-03 | HTTP/3 (QUIC) | Should | Modern client support |
| PR-04 | gRPC proxying | Must | Full gRPC support including streaming |
| PR-05 | WebSocket support | Must | Connection upgrade handling |
| PR-06 | Server-Sent Events (SSE) | Must | Long-lived streaming responses |

### 4.3 Security

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| SC-01 | TLS termination with configurable protocols and cipher suites | Must | TLSv1.2 + TLSv1.3, configurable cipher list |
| SC-02 | Upstream TLS (mTLS optional) | Must | TLS to upstream with optional client certificate |
| SC-03 | Request size limiting | Must | Prevent oversized payloads |
| SC-04 | Hardened container security | Must | Non-root, read-only filesystem, drop all capabilities, seccomp |
| SC-05 | Zero-trust architecture support | Must | Every request authenticated and authorized; no implicit trust between zones |
| SC-06 | SPIFFE/SPIRE integration for workload identity | Should | Standard workload identity framework for zero-trust |
| SC-07 | Certificate auto-rotation | Should | Automated TLS certificate renewal |
| SC-08 | Via header suppression | Must | Do not expose gateway identity in Via response header |

### 4.4 Observability

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| OB-01 | OpenTelemetry native tracing (OTLP) | Must | W3C Trace Context as primary propagation format |
| OB-02 | B3 propagation backward compatibility | Should | Support B3 headers for migration period |
| OB-03 | Custom span attributes | Must | Environment, zone, consumer, business context headers as span attributes |
| OB-04 | Prometheus metrics endpoint | Must | Native /metrics endpoint with standard gateway metrics |
| OB-05 | Custom metric labels | Must | Consumer, zone, environment labels on all request metrics |
| OB-06 | Configurable latency histogram buckets | Should | Custom bucket boundaries for latency histograms |
| OB-07 | OTLP metrics push | Should | Push metrics via OTLP in addition to Prometheus scraping |
| OB-08 | Structured JSON access logs | Must | Configurable structured logging with consumer, trace ID, upstream status, etc. |
| OB-09 | Health check endpoints (liveness, readiness, startup) | Must | Standard Kubernetes probe endpoints |

### 4.5 Configuration and Operations

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| CO-01 | Kubernetes CRD-based configuration | Should | Define routes, services, plugins via Kubernetes Custom Resources |
| CO-02 | Declarative file-based configuration | Should | YAML/JSON config files loadable from ConfigMaps or filesystem |
| CO-03 | Hot-reload without restart | Must | Apply configuration changes without dropping connections |
| CO-04 | Kubernetes Gateway API support | Nice to have | Standard gateway.networking.k8s.io resources |
| CO-05 | API for configuration (REST or gRPC) | Should | Programmable config interface for Rover control plane integration |
| CO-06 | Graceful shutdown with connection draining | Must | Drain in-flight requests on SIGTERM before pod termination |
| CO-07 | Pre-stop hook compatibility | Must | Support configurable sleep before shutdown for load balancer deregistration |

### 4.6 Extensibility

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| EX-01 | Plugin/filter/extension mechanism | Must | Ability to add custom processing logic at various points in the request lifecycle |
| EX-02 | External processing support | Should | Call external services (gRPC or HTTP) for custom auth, transformation, or enrichment |
| EX-03 | Request lifecycle hooks | Must | Pre-route, pre-upstream, post-upstream, pre-response hooks for custom logic |

### 4.7 Scalability and Deployment

| ID | Requirement | Priority | Notes |
|---|---|---|---|
| SD-01 | Horizontal Pod Autoscaling (HPA) | Must | Scale based on CPU/memory |
| SD-02 | KEDA integration | Should | Scale based on custom metrics (request rate, queue depth, cron schedules) |
| SD-03 | Argo Rollouts compatibility | Should | Progressive delivery with analysis templates |
| SD-04 | Multi-zone deployment | Must | Deploy independently per zone with zone-specific configuration |
| SD-05 | Helm chart deployment | Must | Deployable via Helm with per-environment value overrides |

## 5. Current Feature Mapping

This section maps current Stargate features to their disposition in the new gateway.

| Current Feature | Current Component | New Gateway Disposition |
|---|---|---|
| HTTP/gRPC reverse proxy | Kong | Native — core routing |
| JWT validation (jwt-keycloak) | Kong plugin | Native — JWT validation with JWKS |
| Rate limiting (multi-dimensional) | Kong plugin (rate-limiting-merged) | Native — per-consumer + per-service rate limiting |
| Prometheus metrics | Kong plugin (prometheus) | Native — Prometheus + OTLP metrics |
| Distributed tracing (Zipkin) | Kong plugin (zipkin) | Native — OpenTelemetry tracing |
| Request size limiting | Kong plugin | Native — request validation |
| Request transformation | Kong plugin | Native — header/body transformation |
| ACL | Kong plugin | Native — consumer authorization |
| Admin API + PostgreSQL | Kong core | Replaced — CRD/declarative config; Rover adapts to new config model |
| OneToken generation | Jumper | Native — built into gateway core |
| Mesh token fetching | Jumper | Replaced — mesh uses OneToken approach; upstream OAuth covers external IdP |
| External OAuth (client_credentials, JWT bearer, etc.) | Jumper | Native — upstream OAuth with multiple grant types |
| BasicAuth forwarding | Jumper | Native — header transformation |
| X-Token-Exchange | Jumper | Generalized — generic header-to-header mapping |
| Spectre event listening | Jumper | Externalized — gateway provides request mirroring; dedicated Spectre microservice handles Horizon event encapsulation |
| Zone failover routing | Jumper | Open — weighted load balancing + health checks cover this; exact failover pattern TBD |
| Header enrichment | Jumper | Native — conditional header manipulation |
| Token caching + coalescing | Jumper | Native — efficient caching built into upstream OAuth |
| JWKS/discovery/certs endpoints | Issuer Service | Open — evaluate during hackathon whether to embed in gateway or keep separate |
| Key rotation | Issuer Service + cert-manager | Native — key rotation support required |

## 6. Open Points for Hackathon Discussion

| Topic | Question |
|---|---|
| Issuer Service disposition | Embed JWKS/discovery endpoints in the gateway, keep as separate microservice, or replace with standard IdP? |
| Zone failover pattern | Weighted load balancing with health checks covers basic failover. Is active zone health checking + force-skip-zone logic still needed at the gateway level, or should DNS/infra handle it? |
| Spectre microservice design | Define the interface between gateway request mirroring and the new Spectre microservice (CloudEvents format, Horizon event emission) |
| Rover integration | How does Rover translate API configurations for the new gateway? CRD generation? Declarative config files? Direct API calls? |
| Token cache sharing | Should token cache be per-pod (in-memory) or shared across pods (Redis)? What about cache invalidation on 4xx responses? |
| Consumer identity model | How are consumers identified in the new gateway? JWT claims? API keys? mTLS certificates? All of the above? |
| Migration strategy | How to migrate from Kong+Jumper to the new gateway? Big-bang per zone? Gradual traffic shifting? Shadow mode? |

## 7. Constraints

| Constraint | Description |
|---|---|
| Open-source license | Gateway must be available under Apache 2.0, MIT, MPL, or similar permissive/copyleft license. No SSPL, BSL, or proprietary feature gates. |
| Kubernetes-native | Must run on Kubernetes. Container image must support linux/amd64 and linux/arm64. |
| Cloud-agnostic | Must work on AWS, Azure, and on-premise (CaaS) deployments. |
| No vendor lock-in | Must not depend on a single vendor's commercial offering for core functionality. |
| Backward compatibility | Consumer-facing API contracts (token format, header names, error responses) must remain compatible during migration. |
