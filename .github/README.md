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
2. **Module CI Jobs** - Runs reusable Go CI workflow for each module in parallel:
   - common
   - common-server
   - secret-manager
   - approval
   - file-manager
   - gateway
   - identity
   - organization
   - admin
   - application
   - api
   - rover
   - rover-server
   - rover-ctl
   - notification
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
Automated dependency updates for:
- GitHub Actions (monthly)
- Go modules across all module directories (monthly)
- Groups minor and patch updates together

#### **Dependabot Tidy Workflow** (`dependabot-tidy.yaml`)
**Triggers:** Pull requests with go.mod/go.sum changes, manual dispatch
**Conditions:** Only runs for Dependabot PRs or manual triggers

**Sequence:**
1. Identify all modules with go.mod files
2. Run `go mod tidy` on each module
3. Commit and push changes if detected
4. Add comment to PR confirming completion

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

### 5. Release Management

#### **Release Workflow** (`release.yaml`)
**Triggers:** Manual dispatch only

**Sequence:**
1. Generate GitHub App token for authentication
2. Setup Go and caching
3. Install tools (cosign, syft, goreleaser)
4. Run Semantic Release
   - Analyzes commits
   - Determines version bump
   - Generates changelog
   - **Executes versioning scripts** (see Unified Versioning below)
   - Creates GitHub release
5. Run GoReleaser (if new release published)
   - Build binaries for multiple platforms
   - Sign artifacts with cosign
   - Generate SBOM with syft
   - Publish to GitHub release

**Permissions:** Packages (write), contents (write), issues (write), pull-requests (write), id-token (write)

---

### Unified Versioning Strategy

The repository uses a **unified versioning approach** where all modules and Helm charts share the same version number after each release, regardless of which components were actually changed.

#### **Versioning Scripts** (`.github/scripts/`)

Two scripts ensure consistent versioning across the monorepo:

1. **`update_chart_version.sh`**
   - Updates `version` and `appVersion` in Helm Chart.yaml files
   - Called by semantic-release during the prepare phase
   - Example: Updates `common-server/helm/Chart.yaml`

2. **`update_install.sh`**
   - Updates the install kustomization file with new version references
   - Modifies `install/kustomization.yaml` to point to the new release tag
   - Updates both `ref` and `newTag` fields

#### **Integration with Semantic Release**

These scripts are executed automatically during the release process via `.releaserc.mjs`:
- Runs during the `prepare` phase before creating the release
- Updates version references in install files and Helm charts
- Commits changes back to the repository
- Modified files: `CHANGELOG.md`, `install/kustomization.yaml`, `common-server/helm/Chart.yaml`

#### **Benefits**

- **Simplified version selection** - Users only need to select a single version for the entire monorepo
- **Guaranteed compatibility** - All components with the same version are tested together and known to work together
- **Easier deployment** - No need to track individual component versions
- **Consistent release process** - Automated versioning reduces manual errors

---

### 6. Security & Compliance

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
├─ CI Workflow
│  ├─ Prepare (detect changes)
│  └─ Module CI Jobs (parallel)
│     ├─ Static Checks
│     ├─ Tests & Coverage
│     ├─ Vulnerability Scan
│     ├─ CodeQL Analysis
│     ├─ Build Image
│     └─ Image Scan
├─ Docs Build (if docs changed)
└─ Dependabot Tidy (if dependabot PR)
```

### Main Branch Push Flow
```
Push to Main
├─ REUSE Compliance Check
├─ CI Workflow (all modules)
└─ Docs Deploy (if docs changed)
```

### Release Flow
```
Manual Trigger: Release Workflow
├─ Semantic Release (analyze commits)
├─ Generate Changelog
├─ Create GitHub Release
└─ GoReleaser (build & sign artifacts)
```

### Version Tag Flow
```
Push Tag (v*)
├─ CI Workflow (all modules)
└─ Helm Release (for modules with charts)
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
- **Helm** - Kubernetes package manager
- **Docusaurus** - Documentation site generator

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
