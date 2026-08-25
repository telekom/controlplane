<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# GitHub Actions Workflows

This document provides an overview of the GitHub Actions workflows used in this repository.

## Workflow Overview

The repository uses a comprehensive CI/CD pipeline with multiple specialized workflows organized into the following categories:

### 1. Core CI Pipeline

#### **CI Workflow** (`ci.yaml`)
**Triggers:** Push to main, tags (v*), pull requests, daily schedule, manual dispatch

The main CI workflow orchestrates the entire build and test process for the monorepo:

**Sequence:**
1. **Prepare** - Detects changed modules using `monutil` to optimize subsequent jobs (currently not used for CI)
2. **Module CI Jobs** - Runs reusable Go CI workflow for each module in parallel
3. **Helm Release** - Publishes Helm charts for modules on version tags (e.g., common-server-helm)

**Permissions:** Contents (read), pull-requests (write), checks (write), security-events (write), packages (write), actions (read)

---

#### **Reusable Go CI Workflow** (`reusable-go-ci.yaml`)
**Type:** Reusable workflow called by other workflows

This workflow provides standardized CI operations for Go modules with configurable steps:

**Job Sequence:**
1. **Static Checks**
   - Run golangci-lint
   - Check generated files (manifests and code generation)

2. **Tests & Coverage** (runs in parallel with static checks)
   - Execute unit tests with gotestfmt
   - Generate HTML and Cobertura coverage reports
   - Upload test logs and reports as artifacts
   - Publish test report as PR check

3. **Source Vulnerability Scan** (runs in parallel)
   - Run govulncheck on Go dependencies

4. **CodeQL Analysis** (runs in parallel)
   - Static security analysis

5. **Build Image** (runs after tests pass)
   - Build container image using Ko
   - Push to container registry (ghcr.io)
   - Output image digest

6. **Image Vulnerability Scan** (runs after build)
   - Scan container image with Trivy
   - Check for CRITICAL and HIGH vulnerabilities

**Configurable Options:**
- Module path
- Ko build path
- Enable/disable specific checks
- Allow test failures
- Custom image tags

---

### 2. Dependency Management

#### **Dependabot Configuration** (`dependabot.yml`)
Manages automated dependency updates across the monorepo with ecosystem-specific grouping strategies.

**GitHub Actions** (monthly, multiple groups)
- **Actions group** - GitHub's official actions (actions/*)
- **Docker group** - Docker build/login/setup actions (docker/*)
- **CodeQL group** - CodeQL actions (github/codeql-action*) - kept aligned across init/analyze
- **Third-party group** - All remaining third-party actions

**npm** (monthly, 2 groups)
- **Minor & Patch** - Grouped dependency updates for non-breaking changes
- **Major** - Separate group for breaking changes

**Go modules** (monthly, 2 groups per directory)
- **Minor & Patch** - Grouped by dependency name for non-breaking changes
- **Major** - Grouped by dependency name for breaking changes
- Covers 40+ module directories (including nested `*/api` modules and `tools/*`); see `.github/dependabot.yml` for the authoritative list.

**Grouping Strategy:**
- Reduces PR noise by combining minor/patch updates
- Isolates major version bumps for separate review
- Docker and CodeQL actions kept in sync to prevent compatibility issues
- Official GitHub actions reviewed together for consistency

---

#### **Dependabot Tidy Workflow** (`dependabot-tidy.yaml`)
**Triggers:** Pull requests with go.mod/go.sum changes, manual dispatch (with PR number input)
**Conditions:** Only runs for Dependabot PRs or manual triggers

Ensures all Go modules remain tidy after dependency updates by running `go mod tidy` automatically.

**Security Model:**
- Uses `pull_request_target` to run in base branch context (safe for Dependabot PRs)
- Checks out PR head by immutable SHA to prevent privilege escalation
- Guards execution strictly to `dependabot[bot]` user
- Uses GitHub App token when configured to re-trigger downstream CI workflows
- Falls back to standard `GITHUB_TOKEN` when App token unavailable (note: pushes won't automatically re-trigger downstream workflows)

**Sequence:**
1. **Token Setup**
   - Generate GitHub App token (if `vars.APP_ID` configured)
   - Enables downstream CI re-runs after tidy commit
   
2. **PR Resolution**
   - Resolve PR head SHA and ref from webhook or workflow dispatch input
   - Support both automatic (webhook) and manual (workflow_dispatch) triggers
   
3. **Checkout**
   - Checkout PR branch at exact immutable head SHA
   - Uses App token (preferred) or GITHUB_TOKEN for attribution
   
4. **Go Setup & Tidy**
   - Setup Go with stable version (check-latest: true)
   - Auto-discover all modules with go.mod files
   - Run `go mod tidy` on each module sequentially
   
5. **Change Detection & Commit**
   - Check for changes after tidy
   - Commit with descriptive message if changes detected
   - Push to PR branch with explicit target (required for detached HEAD)
   
6. **PR Comment**
   - Add comment confirming tidy completion (if changes made)

**Key Features:**
- **Automatic module discovery** - Finds all go.mod files dynamically
- **Immutable SHA checkout** - Prevents privilege escalation via ref hijacking
- **GitHub App integration** - Triggers downstream CI when properly configured
- **Graceful fallback** - Works with standard GITHUB_TOKEN if App not configured

---

### 3. Documentation

#### **Docs Build Workflow** (`docs-build.yaml`)
**Triggers:** Pull requests affecting docs or workflow files

**Sequence:**
1. Setup Node.js v20
2. Install dependencies (npm ci)
3. Build Docusaurus site

Validates documentation builds correctly before merging.

#### **Docs Deploy Workflow** (`docs-deploy.yaml`)
**Triggers:** Push to main (docs changes), manual dispatch

**Sequence:**
1. **Build Job**
   - Setup Node.js v20
   - Install dependencies
   - Build Docusaurus site
   - Upload build artifact

2. **Deploy Job**
   - Deploy artifact to GitHub Pages
   - Update deployment environment

---

### 4. Helm Chart Management

#### **Helm Publish Workflow** (`helm-publish.yaml`)
**Triggers:** Manual dispatch, push to common-server/helm

Entry point for manual Helm chart publishing, delegates to the reusable helm-release workflow.

**Inputs:**
- Chart path (default: common-server/helm)
- Optional version override

#### **Helm Release Workflow** (`helm-release.yaml`)
**Type:** Reusable workflow

**Sequence:**
1. Setup Helm
2. Login to GitHub Container Registry (GHCR)
3. Lint Helm chart
4. Determine version (from input, tag, or commit hash)
5. Package chart with version and app-version
6. Push chart to GHCR (oci://ghcr.io/...)

---

### 5. Build & Image Management

#### **Rover-CTL Base Image Workflow** (`rover-ctl-base-image.yaml`)
**Triggers:** Reusable workflow (called by ci.yaml and release.yaml), manual dispatch

Builds and publishes the rover-ctl base image (containing bash/jq/yq) to the internal Artifactory Docker registry.

**Invocation:**
- **From CI Workflow** - Triggered only when `rover-ctl/Dockerfile.base` changes
- **From Release Workflow** - Runs unconditionally before every release
- **Manual** - Via workflow_dispatch for ad-hoc security patch rebuilds

**Sequence:**
1. Checkout code
2. Setup Docker Buildx
3. Authenticate to Artifactory registry
4. Build and push base image with tags:
   - `latest`
   - Custom version tag (configured via `vars.ROVERCTL_BASE_IMAGE_TAG`)

**Important Notes:**
- Publishes to **internal Artifactory registry** (not GHCR)
- Requires repository variables and secrets:
  - `vars.REGISTRY_HOST` - Artifactory registry host
  - `vars.REGISTRY_ROVERCTL_BASE_REPO` - Repository path in Artifactory
  - `vars.ROVERCTL_BASE_IMAGE_TAG` - Base image tag (e.g., bash-jq-yq-v1)
  - `secrets.ARTIFACTORY_O28M_PUSH_USER` - Push credentials
  - `secrets.ARTIFACTORY_O28M_PUSH_TOKEN` - Push credentials
- Keep registry values synchronized with:
  - `.goreleaser.yaml` - `base_image` value in rover-ctl entry
  - `ci.yaml` - `base_image` input parameter

---

### 6. Release Management

#### **Release Workflow** (`release.yaml`)
**Triggers:** Manual dispatch only

**Sequence:**
1. Build and push rover-ctl base image (via `rover-ctl-base-image` job)
2. Generate GitHub App token for authentication
3. Setup Go and caching
4. Install tools (cosign, syft, goreleaser)
5. Login to GHCR and Artifactory registries
6. Run Semantic Release
   - Analyzes commits
   - Determines version bump
   - Generates changelog
   - **Executes versioning scripts** (see Unified Versioning below)
   - Creates GitHub release
7. Run GoReleaser (if new release published)
   - Build binaries for multiple platforms
   - Build container images using Ko
   - Sign artifacts with cosign
   - Generate SBOM with syft
   - Publish to GitHub release
8. **Mirror release images to Artifactory** (if new release published)
   - Uses skopeo to copy all module images from GHCR to internal Artifactory registry
   - Mirrors version tag (v*), `latest`, and `stable` (for non-prerelease versions)
   - Copies exact multi-arch manifests for internal deployments
   - Covers all modules except rover-ctl (handled separately below)
   - Modules: admin, agentic, api, application, approval, common-server, controlplane-api, discovery-server, event, file-manager, gateway, identity, notification, organization, organization-server, permission, projector, pubsub, rover, rover-server, secret-manager
9. Build and push final rover-ctl image (if new release published)
   - Layers rover-ctl binary (from GHCR) onto bash/jq/yq base image (from Artifactory)
   - Combines base with roverctl binary
   - Publishes to internal Artifactory registry

**Permissions:** Packages (write), contents (write), issues (write), pull-requests (write), id-token (write)

**Key Features:**
- **Dual registry distribution** - GHCR for open-source binaries, Artifactory for GPL-bundled images
- **Image mirroring** - Synchronizes multi-arch manifests across registries for consistent deployments
- **Automated versioning** - Unified version across all components and Helm charts

---

### Unified Versioning Strategy

The repository uses a **unified versioning approach** where all modules and Helm charts share the same version number after each release, regardless of which components were actually changed.

#### **Versioning Scripts** (`.github/scripts/`)

Two scripts ensure consistent versioning across install overlays and Helm charts:

1. **`update_install.sh`**
   - Updates `install/overlays/default/kustomization.yaml` to the newly released tag
   - Rewrites both remote `ref` values and image `newTag` values
   - Called by semantic-release during the prepare phase

2. **`update_chart_version.sh`**
   - Updates `version` and `appVersion` in Helm Chart.yaml files
   - Called by semantic-release during the prepare phase
   - Example: Updates `common-server/helm/Chart.yaml`

#### **Integration with Semantic Release**

These scripts are executed automatically during the release process via `.releaserc.mjs`:
- Runs during the `prepare` phase before creating the release
- Updates version references in install overlays and Helm charts
- Commits changes back to the repository
- Modified files: `CHANGELOG.md`, `install/overlays/default/kustomization.yaml`, `common-server/helm/Chart.yaml`

#### **Benefits**

- **Simplified version selection** - Users only need to select a single version for the entire monorepo
- **Guaranteed compatibility** - All components with the same version are tested together and known to work together
- **Easier deployment** - No need to track individual component versions
- **Consistent release process** - Automated versioning reduces manual errors

---

### 7. Security & Compliance

#### **Scorecard Supply-Chain Security Workflow** (`scorecard.yml`)
**Triggers:** Branch protection rule changes, scheduled weekly (Monday 1:42 AM UTC), push to main

Performs OpenSSF supply-chain security scoring and analysis on the repository.

**Sequence:**
1. Checkout code (without credentials)
2. Run OpenSSF Scorecard analysis
   - Analyze repository security posture
   - Check branch protection settings
   - Verify maintenance status
   - Generate SARIF report
3. Upload results as artifact (retention: 5 days)
4. Upload SARIF results to GitHub Code Scanning dashboard

**Permissions:** security-events (write), id-token (write)

**Benefits:**
- Continuous monitoring of supply-chain security health
- Automatic detection of branch protection issues
- Verification of repository maintenance status
- Compliance with OpenSSF best practices
- Public repository badge eligibility

**Configuration:**
- Results published to OpenSSF REST API (public repos)
- Optional: Fine-grained PAT token (`secrets.SCORECARD_TOKEN`) for enhanced checks

---

#### **ORT Scanning Workflow** (`ort.yaml`)
**Triggers:** Manual dispatch

**Sequence:**
1. Configure Git to use HTTPS
2. Setup ORT config (merge default config with custom `.github/ort/config.yml`)
3. Run ORT CI action
   - Cache dependencies
   - Cache scan results
   - Analyze dependencies
   - Scan for vulnerabilities
   - Run advisor checks
   - Evaluate results
   - Generate reports
   - Upload results

Performs comprehensive open-source license and vulnerability scanning.

---

#### **REUSE Compliance Workflow** (`reuse-compliance.yml`)
**Triggers:** Push and pull requests

**Sequence:**
1. Checkout repository
2. Run REUSE compliance check

Ensures all files have proper SPDX license headers.

---

## Workflow Execution Patterns

### Pull Request Flow
```
PR Created/Updated
├─ REUSE Compliance Check
├─ Scorecard Analysis (on main only, skip for PRs)
└─ CI Workflow
   ├─ Prepare (detect changes)
   └─ Module CI Jobs (parallel)
      ├─ Static Checks
      ├─ Tests & Coverage
      ├─ Vulnerability Scan
      ├─ CodeQL Analysis
      ├─ Build Image
      ├─ Image Scan
      └─ Rover-CTL Base Image (if rover-ctl/Dockerfile.base changed)
├─ Docs Build (if docs changed)
└─ Dependabot Tidy (if dependabot PR)
```

### Main Branch Push Flow
```
Push to Main
├─ REUSE Compliance Check
├─ Scorecard Analysis
├─ CI Workflow (all modules)
│  └─ Rover-CTL Base Image (if rover-ctl/Dockerfile.base changed)
└─ Docs Deploy (if docs changed)
```

### Release Flow
```
Manual Trigger: Release Workflow
├─ Rover-CTL Base Image (unconditionally)
├─ Semantic Release (analyze commits)
├─ Generate Changelog
├─ Create GitHub Release
├─ GoReleaser (build & sign artifacts)
│  └─ Push images to GHCR
├─ Mirror release images to Artifactory
│  └─ Copy multi-arch manifests (v*, latest, stable)
└─ Build & push final rover-ctl image
   └─ Layer binary + base image → Artifactory
```

### Version Tag Flow
```
Push Tag (v*)
├─ REUSE Compliance Check
├─ CI Workflow (all modules)
└─ Helm Release (for modules with charts)
```

### Scheduled Security Checks
```
Weekly Schedule (Monday 1:42 AM UTC)
└─ Scorecard Analysis
```

---

## Key Technologies

- **Ko** - Container image builder for Go applications
- **GoReleaser** - Release automation for Go projects
- **Semantic Release** - Automated versioning and changelog generation
- **Trivy** - Container vulnerability scanner
- **CodeQL** - Code security analysis
- **govulncheck** - Go vulnerability checker
- **golangci-lint** - Go linter aggregator
- **ORT** - OSS Review Toolkit for license compliance
- **REUSE** - License compliance tool
- **OSSF Scorecard** - Supply-chain security assessment
- **Helm** - Kubernetes package manager
- **Docusaurus** - Documentation site generator
- **Docker Buildx** - Multi-platform container builder

---

## Concurrency Control

- **CI Workflow**: Concurrent runs per ref, cancels in-progress on new push
- **Helm Publish**: Concurrent runs per ref, cancels in-progress on new push

---

## Artifacts & Outputs

### Generated Artifacts
- Test reports (JUnit XML, coverage reports)
- Test logs (gotest.log)
- Container images (pushed to GHCR)
- Helm charts (pushed to GHCR OCI registry)
- Release binaries (attached to GitHub releases)
- SBOM files (software bill of materials)
- ORT scan results

### Key Outputs
- Image digests from builds
- Test coverage metrics
- Vulnerability scan results
- License compliance reports

---

## Permissions Model

Workflows follow the principle of least privilege:
- Most workflows have **read-only** content access by default
- Write permissions are explicitly granted where needed (packages, releases, deployments)
- PR workflows can write checks and comments but not merge
- Release workflow requires elevated permissions for publishing

---

## Maintenance Notes

- **Dependabot** manages dependencies monthly for both GitHub Actions and Go modules
- **Pin commitments** are used for critical actions (security best practice)
- **Reusable workflows** reduce duplication and ensure consistency across modules
- **Conditional execution** optimizes CI runtime by running only necessary jobs
