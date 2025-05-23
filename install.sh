#!/bin/bash

# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

REPO_NAME="telekom/controlplane"
CONTROLPLANE_VERSION="latest"

CERT_MANAGER_VERSION="v1.17.2"
TRUST_MANAGER_VERSION="v0.17.1"

ACTIVE_KUBE_CONTEXT=$(kubectl config current-context)

function request_user_input() {
    local prompt="$1"
    local default_value="$2"

    read -p "$prompt [$default_value]: " input
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

function get_latest_release() {
    local repo="$1"

    LATEST_RELEASE_INFO_URL="https://api.github.com/repos/${repo}/releases/latest"
    LATEST_RELEASE_JSON_FILE=$(mktemp)

    curl -sSL -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github.v3+json" -o "${LATEST_RELEASE_JSON_FILE}" "${LATEST_RELEASE_INFO_URL}"

    TAG_NAME=$(jq -r .tag_name "${LATEST_RELEASE_JSON_FILE}")
    echo "$TAG_NAME"
}

function install_cert_manager() {
    local version="$1"
    echo "Installing Cert-Manager version $version..."

    helm repo add jetstack https://charts.jetstack.io --force-update

    helm upgrade cert-manager jetstack/cert-manager \
        --namespace cert-manager \
        --create-namespace \
        --version $version \
        --set crds.enabled=true \
        --wait
}

function install_trust_manager() {
    local version="$1"
    echo "Installing Trust-Manager version $version..."

    helm repo add jetstack https://charts.jetstack.io --force-update

    helm upgrade trust-manager jetstack/trust-manager \
        --install \
        --namespace cert-manager \
        --version $version \
        --wait
}


function install_controlplane() {
    local version="$1"
    if [ "$version" == "latest" ]; then
        version=$(get_latest_release $REPO_NAME)
    fi

    if [ -z "$version" ] || [ "$version" == "null" ]; then
        echo "Failed to get the latest version of controlplane."
        exit 1
    fi

    echo "Installing ControlPlane version $version..."

    ROOT_KUSTOMIZE_FILE="kustomization.yaml"
    KUSTOMIZE_FILE_URL="https://raw.githubusercontent.com/${REPO_NAME}/${version}/install/kustomization.yaml"

    curl -sSL -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/yaml" -o "${ROOT_KUSTOMIZE_FILE}" "${KUSTOMIZE_FILE_URL}"

}


function main() {
    check_binary_exists "kubectl"
    check_binary_exists "helm"
    check_binary_exists "jq"

    request_user_input "Install on which Kubernetes context?" "$ACTIVE_KUBE_CONTEXT"

    # Install Cert-Manager
    install_cert_manager "$CERT_MANAGER_VERSION"

    # Install Trust-Manager
    install_trust_manager "$TRUST_MANAGER_VERSION"

    # Install ControlPlane
    install_controlplane "$CONTROLPLANE_VERSION"
}


main "$@"