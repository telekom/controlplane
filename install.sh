#!/bin/bash

# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

set -eo pipefail

REPO_NAME="telekom/controlplane"
CONTROLPLANE_VERSION="latest"

CONTROLPLANE_NAMESPACE="controlplane-system"

CERT_MANAGER_VERSION="v1.18.2"
TRUST_MANAGER_VERSION="v0.19.0"
PROM_OPERATOR_CRDS_VERSION="v23.0.0"

WITH_CERT_MANAGER=false
WITH_TRUST_MANAGER=false
WITH_MONITORING_CRDS=false

function print_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Downloads the Control Plane kustomization and optionally installs prerequisites."
    echo "After running, apply the kustomization via kubectl or a GitOps tool (e.g. ArgoCD)."
    echo ""
    echo "Options:"
    echo "  --with-cert-manager       Install Cert-Manager"
    echo "  --with-trust-manager      Install Trust-Manager"
    echo "  --with-monitoring-crds    Install Prometheus Operator CRDs"
    echo "  -h, --help                Show this help message and exit"
    echo ""
    echo "Environment variables:"
    echo "  GITHUB_TOKEN              GitHub token for API access (recommended to avoid rate limits)"
    echo ""
    echo "Example:"
    echo "  $0 --with-cert-manager --with-monitoring-crds"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --with-cert-manager)
            WITH_CERT_MANAGER=true
            shift
            ;;
        --with-trust-manager)
            WITH_TRUST_MANAGER=true
            shift
            ;;
        --with-monitoring-crds)
            WITH_MONITORING_CRDS=true
            shift
            ;;
        -h|--help)
            print_help
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use '$0 --help' to see available options."
            exit 1
            ;;
    esac
done

function check_github_token() {
    if [ -z "$GITHUB_TOKEN" ]; then
        echo "Warning: GITHUB_TOKEN environment variable is not set. This may be required due to GitHub API rate limits."
        echo ""
    fi
}

function request_user_input() {
    local prompt="$1"
    local default_value="$2"

    read -rp "$prompt [$default_value]: " input
    if [ -z "$input" ]; then
        echo "$default_value"
    else
        echo "$input"
    fi
}

function check_binary_exists() {
    local binary="$1"
    if ! command -v "$binary" &> /dev/null; then
        echo "$binary is not installed. Please install it first."
        exit 1
    fi
}

# Build curl arguments, conditionally including the Authorization header.
function build_curl_args() {
    CURL_AUTH_ARGS=()
    if [ -n "$GITHUB_TOKEN" ]; then
        CURL_AUTH_ARGS+=(-H "Authorization: Bearer $GITHUB_TOKEN")
    fi
}

function get_latest_release() {
    local repo="$1"

    LATEST_RELEASE_INFO_URL="https://api.github.com/repos/${repo}/releases/latest"
    LATEST_RELEASE_JSON_FILE=$(mktemp)
    trap 'rm -f "$LATEST_RELEASE_JSON_FILE"' EXIT

    if ! curl -sSL --fail \
        "${CURL_AUTH_ARGS[@]}" \
        -H "Accept: application/vnd.github.v3+json" \
        -o "${LATEST_RELEASE_JSON_FILE}" \
        "${LATEST_RELEASE_INFO_URL}"; then
        echo "Failed to fetch latest release information from GitHub."
        exit 1
    fi

    TAG_NAME=$(jq -r .tag_name "${LATEST_RELEASE_JSON_FILE}")
    echo "$TAG_NAME"
}

function install_cert_manager() {
    local version="$1"
    echo "Installing Cert-Manager version $version..."

    helm --kube-context "$ACTIVE_KUBE_CONTEXT" \
        upgrade cert-manager jetstack/cert-manager \
        --install \
        --namespace cert-manager \
        --create-namespace \
        --version "$version" \
        --set crds.enabled=true \
        --wait
}

function install_trust_manager() {
    local version="$1"
    echo "Installing Trust-Manager version $version..."

    helm --kube-context "$ACTIVE_KUBE_CONTEXT" \
        upgrade trust-manager jetstack/trust-manager \
        --install \
        --namespace cert-manager \
        --version "$version" \
        --set "app.trust.namespace=$CONTROLPLANE_NAMESPACE" \
        --wait
}

function install_monitoring_crds() {
    local version="$1"
    echo "Installing Prometheus Operator CRDs version $version..."

    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update

    helm --kube-context "$ACTIVE_KUBE_CONTEXT" \
        upgrade prometheus-operator-crds prometheus-community/prometheus-operator-crds \
        --install \
        --namespace monitoring \
        --create-namespace \
        --version "$version" \
        --wait
}

function install_controlplane() {
    local version="$1"
    if [ "$version" == "latest" ]; then
        version=$(get_latest_release "$REPO_NAME")
    fi

    if [ -z "$version" ] || [ "$version" == "null" ]; then
        echo "Failed to get the latest version of controlplane."
        exit 1
    fi

    echo "Downloading ControlPlane kustomization version $version..."

    ROOT_KUSTOMIZE_FILE="kustomization.yaml"
    KUSTOMIZE_FILE_URL="https://raw.githubusercontent.com/${REPO_NAME}/${version}/install/overlays/default/kustomization.yaml"

    if ! curl -sSL --fail \
        "${CURL_AUTH_ARGS[@]}" \
        -H "Accept: application/yaml" \
        -o "${ROOT_KUSTOMIZE_FILE}" \
        "${KUSTOMIZE_FILE_URL}"; then
        echo "Failed to download kustomization.yaml from GitHub."
        exit 1
    fi

    echo ""
    echo "============================================================"
    echo "  ControlPlane kustomization downloaded (version $version)"
    echo "============================================================"
    echo ""
    echo "  The file 'kustomization.yaml' has been saved to the current directory."
    echo ""
    echo "  Next steps:"
    echo ""
    echo "  1. Review and customize the kustomization.yaml as needed."
    echo ""
    echo "  2. Apply with kubectl:"
    echo "       kubectl apply -k ."
    echo ""
    echo "  3. Or use a GitOps tool such as ArgoCD:"
    echo "       Point your ArgoCD Application at this directory."
    echo ""
    echo "  4. To enable the eventing subsystem (event + pubsub controllers),"
    echo "     uncomment the 'components' and eventing image entries in kustomization.yaml."
    echo ""
    echo "============================================================"
}


function main() {
    check_github_token
    build_curl_args

    check_binary_exists "kubectl"
    check_binary_exists "helm"
    check_binary_exists "jq"
    check_binary_exists "curl"

    ACTIVE_KUBE_CONTEXT=$(kubectl config current-context)
    ACTIVE_KUBE_CONTEXT=$(request_user_input "Install on which Kubernetes context?" "$ACTIVE_KUBE_CONTEXT")

    echo "Using Kubernetes context: $ACTIVE_KUBE_CONTEXT"

    # Add Helm repos once before installing any charts.
    if [ "$WITH_CERT_MANAGER" = true ] || [ "$WITH_TRUST_MANAGER" = true ]; then
        helm repo add jetstack https://charts.jetstack.io --force-update
    fi

    # Install Cert-Manager
    if [ "$WITH_CERT_MANAGER" = true ]; then
        install_cert_manager "$CERT_MANAGER_VERSION"
    fi

    # Install Trust-Manager
    if [ "$WITH_TRUST_MANAGER" = true ]; then
        install_trust_manager "$TRUST_MANAGER_VERSION"
    fi

    # Install Prometheus Operator CRDs
    if [ "$WITH_MONITORING_CRDS" = true ]; then
        install_monitoring_crds "$PROM_OPERATOR_CRDS_VERSION"
    fi

    # Download ControlPlane kustomization
    install_controlplane "$CONTROLPLANE_VERSION"
}


main "$@"
