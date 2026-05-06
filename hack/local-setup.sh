#!/bin/bash

# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Local development setup script for Control Plane.
#
# Creates a kind cluster, installs prerequisites, builds all controller
# images with ko, loads them into kind, and deploys everything.
#
# Usage:
#   ./hack/local-setup.sh                              # full setup
#   ./hack/local-setup.sh --build-only                 # rebuild all images, load into kind, restart deployments
#   ./hack/local-setup.sh --build-only --only gateway  # rebuild one controller, restart its deployment
#   ./hack/local-setup.sh --build-only --only gateway,rover  # rebuild specific controllers
#   ./hack/local-setup.sh --deploy-only                # redeploy kustomization only
#   ./hack/local-setup.sh --jobs 2                     # limit parallel builds (default: nproc)

set -eo pipefail

# ── Configuration ──────────────────────────────────────────────────────
CLUSTER_NAME="${CLUSTER_NAME:-controlplane}"
CONTROLPLANE_NAMESPACE="controlplane-system"
IMAGE_TAG="latest"

CERT_MANAGER_VERSION="v1.18.2"
TRUST_MANAGER_VERSION="v0.19.0"
PROM_OPERATOR_CRDS_VERSION="v23.0.0"

# Repo root is one level up from hack/
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Controllers to build — name:working_dir:image_name
ALL_CONTROLLERS=(
    "admin:admin/cmd:ghcr.io/telekom/controlplane/admin"
    "api:api/cmd:ghcr.io/telekom/controlplane/api"
    "event:event/cmd:ghcr.io/telekom/controlplane/event"
    "pubsub:pubsub/cmd:ghcr.io/telekom/controlplane/pubsub"
    "application:application/cmd:ghcr.io/telekom/controlplane/application"
    "approval:approval/cmd:ghcr.io/telekom/controlplane/approval"
    "file-manager:file-manager/cmd/server:ghcr.io/telekom/controlplane/file-manager"
    "gateway:gateway/cmd:ghcr.io/telekom/controlplane/gateway"
    "identity:identity/cmd:ghcr.io/telekom/controlplane/identity"
    "notification:notification/cmd:ghcr.io/telekom/controlplane/notification"
    "organization:organization/cmd:ghcr.io/telekom/controlplane/organization"
    "rover:rover/cmd:ghcr.io/telekom/controlplane/rover"
    "rover-server:rover-server/cmd:ghcr.io/telekom/controlplane/rover-server"
    "secret-manager:secret-manager/cmd/server:ghcr.io/telekom/controlplane/secret-manager"
)

# ── Flags ──────────────────────────────────────────────────────────────
BUILD_ONLY=false
DEPLOY_ONLY=false
ONLY_CONTROLLERS=""
MAX_JOBS=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --build-only)
            BUILD_ONLY=true
            shift
            ;;
        --deploy-only)
            DEPLOY_ONLY=true
            shift
            ;;
        --only)
            ONLY_CONTROLLERS="$2"
            shift 2
            ;;
        --jobs)
            MAX_JOBS="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --build-only          Skip cluster creation and prereqs; rebuild images and restart deployments"
            echo "  --deploy-only         Skip cluster creation, prereqs, and image build; redeploy only"
            echo "  --only name[,name]    Only build/deploy the specified controllers (comma-separated)"
            echo "                        Available: admin, api, application, approval, event, file-manager,"
            echo "                        gateway, identity, notification, organization, pubsub, rover,"
            echo "                        rover-server, secret-manager"
            echo "  --jobs N              Max parallel builds (default: number of CPUs, min 1)"
            echo "  -h, --help            Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                                    # full setup from scratch"
            echo "  $0 --build-only                       # rebuild all + restart deployments"
            echo "  $0 --build-only --only gateway        # rebuild gateway + restart its deployment"
            echo "  $0 --build-only --only gateway,rover  # rebuild two controllers + restart"
            echo "  $0 --build-only --jobs 2              # limit to 2 parallel builds"
            echo "  $0 --only gateway                     # full setup, but only build/deploy gateway"
            echo "  $0 --deploy-only --only gateway       # restart gateway deployment only"
            exit 0
            ;;
        *)
            echo "Unknown option: $1 (use -h for help)"
            exit 1
            ;;
    esac
done

# ── Helpers ────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()    { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

check_binary() {
    if ! command -v "$1" &>/dev/null; then
        fail "'$1' is not installed. Please install it first."
    fi
}

# Resolve which controllers to build based on --only filter.
resolve_controllers() {
    if [ -z "${ONLY_CONTROLLERS}" ]; then
        CONTROLLERS=("${ALL_CONTROLLERS[@]}")
        return
    fi

    CONTROLLERS=()
    IFS=',' read -ra SELECTED <<< "${ONLY_CONTROLLERS}"
    for sel in "${SELECTED[@]}"; do
        local found=false
        for entry in "${ALL_CONTROLLERS[@]}"; do
            IFS=':' read -r name _ _ <<< "${entry}"
            if [ "${name}" = "${sel}" ]; then
                CONTROLLERS+=("${entry}")
                found=true
                break
            fi
        done
        if [ "${found}" = false ]; then
            fail "Unknown controller '${sel}'. Use -h to see available names."
        fi
    done
}

# ── Step 1: Check prerequisites ───────────────────────────────────────
step_check_prereqs() {
    info "Checking prerequisites..."
    check_binary docker
    check_binary kubectl
    check_binary kind
    check_binary ko
    check_binary helm
    success "All prerequisites found."
}

# ── Step 2: Create kind cluster ───────────────────────────────────────
step_create_cluster() {
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        success "Kind cluster '${CLUSTER_NAME}' already exists, skipping creation."
    else
        info "Creating kind cluster '${CLUSTER_NAME}'..."
        kind create cluster --name "${CLUSTER_NAME}"
        success "Kind cluster '${CLUSTER_NAME}' created."
    fi

    # Ensure kubectl context is set
    kubectl cluster-info --context "kind-${CLUSTER_NAME}" &>/dev/null \
        || fail "Cannot connect to kind cluster '${CLUSTER_NAME}'."
    info "Using kubectl context: kind-${CLUSTER_NAME}"
}

# ── Step 3: Install prerequisites (cert-manager, etc.) ────────────────
step_install_prereqs() {
    local ctx="kind-${CLUSTER_NAME}"

    # Create the controlplane namespace early — trust-manager needs it
    # (app.trust.namespace) and it's referenced before kustomize runs.
    if kubectl --context "${ctx}" get namespace "${CONTROLPLANE_NAMESPACE}" &>/dev/null; then
        success "Namespace '${CONTROLPLANE_NAMESPACE}' already exists."
    else
        info "Creating namespace '${CONTROLPLANE_NAMESPACE}'..."
        kubectl --context "${ctx}" create namespace "${CONTROLPLANE_NAMESPACE}"
        success "Namespace '${CONTROLPLANE_NAMESPACE}' created."
    fi

    # Add Helm repos once before installing charts.
    helm repo add jetstack https://charts.jetstack.io --force-update

    # cert-manager
    if kubectl --context "${ctx}" get namespace cert-manager &>/dev/null; then
        success "cert-manager namespace exists, skipping install."
    else
        info "Installing cert-manager ${CERT_MANAGER_VERSION}..."
        helm --kube-context "${ctx}" \
            upgrade cert-manager jetstack/cert-manager \
            --install \
            --namespace cert-manager \
            --create-namespace \
            --version "${CERT_MANAGER_VERSION}" \
            --set crds.enabled=true \
            --wait
        success "cert-manager installed."
    fi

    # trust-manager
    if kubectl --context "${ctx}" get deployment -n cert-manager trust-manager &>/dev/null 2>&1; then
        success "trust-manager already installed, skipping."
    else
        info "Installing trust-manager ${TRUST_MANAGER_VERSION}..."
        helm --kube-context "${ctx}" \
            upgrade trust-manager jetstack/trust-manager \
            --install \
            --namespace cert-manager \
            --version "${TRUST_MANAGER_VERSION}" \
            --set app.trust.namespace="${CONTROLPLANE_NAMESPACE}" \
            --wait
        success "trust-manager installed."
    fi

    # Prometheus Operator CRDs
    if kubectl --context "${ctx}" get crd prometheuses.monitoring.coreos.com &>/dev/null 2>&1; then
        success "Prometheus Operator CRDs already installed, skipping."
    else
        info "Installing Prometheus Operator CRDs ${PROM_OPERATOR_CRDS_VERSION}..."
        helm repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update
        helm --kube-context "${ctx}" \
            upgrade prometheus-operator-crds prometheus-community/prometheus-operator-crds \
            --install \
            --namespace monitoring \
            --create-namespace \
            --version "${PROM_OPERATOR_CRDS_VERSION}" \
            --wait
        success "Prometheus Operator CRDs installed."
    fi
}

# ── Step 4: Build images with ko (parallel) and load into kind ────────
step_build_and_load_images() {
    local total=${#CONTROLLERS[@]}

    # Detect host platform — only build for the architecture the kind
    # cluster actually runs on. This overrides .ko.yaml's defaultPlatforms
    # (linux/arm64 + linux/amd64) and roughly halves build time.
    local host_arch
    host_arch="$(go env GOARCH 2>/dev/null || echo "amd64")"
    local platform="linux/${host_arch}"
    info "Building ${total} controller images (platform: ${platform})"

    # Determine parallelism.
    local jobs="${MAX_JOBS}"
    if [ -z "${jobs}" ]; then
        jobs="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)"
    fi
    # Cap at the number of controllers to build.
    if [ "${jobs}" -gt "${total}" ]; then
        jobs="${total}"
    fi
    info "Parallel jobs: ${jobs}"

    # Temp directory for per-job logs so parallel output doesn't interleave.
    local log_dir
    log_dir="$(mktemp -d)"
    trap 'rm -rf "${log_dir}"' RETURN

    # Track background PIDs and their controller names.
    local -a pids=()
    local -a names=()
    local running=0
    local failed=0

    for entry in "${CONTROLLERS[@]}"; do
        IFS=':' read -r name working_dir image_name <<< "${entry}"
        local full_image="${image_name}:${IMAGE_TAG}"
        local log_file="${log_dir}/${name}.log"

        # Wait for a slot if we're at max parallelism.
        while [ "${running}" -ge "${jobs}" ]; do
            # Wait for any child to finish, then reap it.
            wait -n -p DONE_PID 2>/dev/null || true
            # Find which controller just finished.
            for i in "${!pids[@]}"; do
                if [ "${pids[$i]}" = "${DONE_PID}" ]; then
                    # Check exit status by waiting for the specific PID.
                    if wait "${pids[$i]}" 2>/dev/null; then
                        success "${names[$i]} ready."
                    else
                        warn "${names[$i]} FAILED — see log below."
                        cat "${log_dir}/${names[$i]}.log" >&2
                        failed=$((failed + 1))
                    fi
                    unset 'pids[$i]' 'names[$i]'
                    running=$((running - 1))
                    break
                fi
            done
        done

        # Launch build+load in background.
        (
            set -e
            echo "[build] ${name} -> ${full_image}"
            cd "${REPO_ROOT}/${working_dir}"
            KO_DOCKER_REPO="${image_name}" ko build --bare --local . \
                --tags "${IMAGE_TAG}" \
                --platform "${platform}" 2>&1

            echo "[load]  ${name} -> kind"
            kind load docker-image "${full_image}" --name "${CLUSTER_NAME}" 2>&1
            echo "[done]  ${name}"
        ) > "${log_file}" 2>&1 &

        pids+=($!)
        names+=("${name}")
        running=$((running + 1))
        info "Started ${name} (pid $!)"
    done

    # Wait for remaining builds to finish.
    for i in "${!pids[@]}"; do
        if wait "${pids[$i]}" 2>/dev/null; then
            success "${names[$i]} ready."
        else
            warn "${names[$i]} FAILED — see log below."
            cat "${log_dir}/${names[$i]}.log" >&2
            failed=$((failed + 1))
        fi
    done

    if [ "${failed}" -gt 0 ]; then
        fail "${failed} of ${total} builds failed. Logs are above."
    fi

    success "All ${total} controller images built and loaded."
}

# ── Step 5: Restart deployments (used by --build-only and --only) ─────
step_restart_deployments() {
    local ctx="kind-${CLUSTER_NAME}"

    for entry in "${CONTROLLERS[@]}"; do
        IFS=':' read -r name _ _ <<< "${entry}"
        local deploy_name="${name}-controller-manager"
        if kubectl --context "${ctx}" -n "${CONTROLPLANE_NAMESPACE}" \
            get deployment "${deploy_name}" &>/dev/null 2>&1; then
            info "Restarting deployment/${deploy_name}..."
            kubectl --context "${ctx}" -n "${CONTROLPLANE_NAMESPACE}" \
                rollout restart deployment/"${deploy_name}"
        else
            warn "Deployment '${deploy_name}' not found, skipping restart."
        fi
    done

    success "Rollout restart triggered for selected controllers."
}

# ── Step 6: Deploy with kustomize ─────────────────────────────────────
step_deploy() {
    local ctx="kind-${CLUSTER_NAME}"

    info "Deploying Control Plane from install/overlays/local/..."
    kubectl --context "${ctx}" apply -k "${REPO_ROOT}/install/overlays/local" 2>&1
    success "Kustomization applied."

    # cert-manager must issue Certificates before trust-manager can create
    # the trust-bundle ConfigMaps that most controllers mount as volumes.
    # Without this wait, pods fail with "configmap not found" on the
    # trust-bundle volume mount.
    info "Waiting for cert-manager Certificates to be issued..."
    kubectl --context "${ctx}" -n "${CONTROLPLANE_NAMESPACE}" \
        wait --for=condition=Ready --timeout=120s certificate --all 2>&1 \
        || warn "Some Certificates did not become Ready within 120s."

    info "Waiting for trust-bundle ConfigMaps to be created..."
    local trust_timeout=60
    local trust_elapsed=0
    while [ "${trust_elapsed}" -lt "${trust_timeout}" ]; do
        if kubectl --context "${ctx}" -n "${CONTROLPLANE_NAMESPACE}" \
            get configmap secret-manager-trust-bundle &>/dev/null; then
            success "secret-manager-trust-bundle ConfigMap exists."
            break
        fi
        sleep 2
        trust_elapsed=$((trust_elapsed + 2))
    done
    if [ "${trust_elapsed}" -ge "${trust_timeout}" ]; then
        warn "secret-manager-trust-bundle ConfigMap not found after ${trust_timeout}s."
        warn "trust-manager may not have processed the Bundle CR yet."
        warn "Pods that mount trust-bundle will fail until the ConfigMap appears."
    fi

    info "Waiting for controller pods to start..."
    kubectl --context "${ctx}" -n "${CONTROLPLANE_NAMESPACE}" \
        wait --for=condition=Available --timeout=180s deployment --all 2>&1 \
        || warn "Some deployments did not become Available within 180s. Check: kubectl get pods -n ${CONTROLPLANE_NAMESPACE}"

    success "Deployment complete."
}

# ── Step 7: Print summary ─────────────────────────────────────────────
step_summary() {
    echo ""
    echo "============================================================"
    echo "  Control Plane is running"
    echo "============================================================"
    echo ""
    echo "  Controllers:  kubectl get pods -n ${CONTROLPLANE_NAMESPACE}"
    echo ""
    echo "  Connect to Secret-Manager:"
    echo "    kubectl port-forward -n ${CONTROLPLANE_NAMESPACE} svc/secret-manager 8443:8443"
    echo "    # Fetch a token first:"
    echo "    kubectl create token -n ${CONTROLPLANE_NAMESPACE} secret-manager --audience secret-manager"
    echo ""
    echo "  Connect to File-Manager:"
    echo "    kubectl port-forward -n ${CONTROLPLANE_NAMESPACE} svc/file-manager 8444:8443"
    echo "    # Fetch a token first:"
    echo "    kubectl create token -n ${CONTROLPLANE_NAMESPACE} file-manager --audience file-manager"
    echo ""
    echo "  Mailpit (SMTP mock — email inbox):"
    echo "    kubectl port-forward -n ${CONTROLPLANE_NAMESPACE} svc/mailpit 8025:8025"
    echo "    # Browse http://localhost:8025 to view captured emails"
    echo ""
    echo "  Next steps:"
    echo "    1. Apply sample resources:"
    echo "       kubectl apply -k install/overlays/local/resources/admin"
    echo "       kubectl apply -k install/overlays/local/resources/org"
    echo "       kubectl apply -k install/overlays/local/resources/rover"
    echo ""
    echo "  Re-run after code changes:"
    echo "    ./hack/local-setup.sh --build-only"
    echo "    ./hack/local-setup.sh --build-only --only gateway"
    echo "============================================================"
}

# ── Main ───────────────────────────────────────────────────────────────
main() {
    echo ""
    info "Control Plane local setup"
    echo ""

    step_check_prereqs
    resolve_controllers

    if [ "${DEPLOY_ONLY}" = true ]; then
        if [ -n "${ONLY_CONTROLLERS}" ]; then
            step_restart_deployments
        else
            step_deploy
        fi
        step_summary
        return
    fi

    if [ "${BUILD_ONLY}" = true ]; then
        step_build_and_load_images
        step_restart_deployments
        return
    fi

    # Full setup
    step_create_cluster
    step_install_prereqs
    step_build_and_load_images

    if [ -n "${ONLY_CONTROLLERS}" ]; then
        step_restart_deployments
    else
        step_deploy
    fi
    step_summary
}

main "$@"
