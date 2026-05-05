#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

IIAB_DIR="${IIAB_DIR:-${REPO_ROOT}/.iiab}"
IIAB_REPO="${IIAB_REPO:-https://github.com/ironcore-dev/ironcore-in-a-box.git}"
IIAB_IMG="${IIAB_IMG:-libvirt-provider:local}"
IIAB_CLUSTER_NAME="${IIAB_CLUSTER_NAME:-ironcore-in-a-box}"
CONTAINER_TOOL="${CONTAINER_TOOL:-docker}"

clone() {
    echo "==> Cloning/updating ironcore-in-a-box..."
    if [ -d "${IIAB_DIR}" ]; then
        cd "${IIAB_DIR}" && git pull && git submodule update --init --recursive
    else
        git clone --recurse-submodules "${IIAB_REPO}" "${IIAB_DIR}"
    fi
}

build() {
    echo "==> Building libvirt-provider image..."
    ${CONTAINER_TOOL} build -t "${IIAB_IMG}" "${REPO_ROOT}"
}

patch() {
    echo "==> Patching iiab kustomization..."
    local kustomization="${IIAB_DIR}/base/libvirt-provider/kustomization.yaml"
    local kustomization_dir
    kustomization_dir="$(cd "$(dirname "${kustomization}")" && pwd)"
    local config_path
    config_path="$(python3 -c "import os.path; print(os.path.relpath('${REPO_ROOT}/config/default', '${kustomization_dir}'))")"

    # Reset any previous patches by restoring from git
    cd "${IIAB_DIR}" && git checkout -- base/libvirt-provider/kustomization.yaml && cd "${REPO_ROOT}"

    if [[ "$(uname)" == "Darwin" ]]; then
        sed -i '' "s|github.com/ironcore-dev/libvirt-provider/config/default?ref=.*|${config_path}|" "${kustomization}"
        sed -i '' "s|newName: ghcr.io/ironcore-dev/libvirt-provider|newName: libvirt-provider|" "${kustomization}"
        sed -i '' "/newName: libvirt-provider/{n;s|newTag:.*|newTag: local|;}" "${kustomization}"
    else
        sed -i "s|github.com/ironcore-dev/libvirt-provider/config/default?ref=.*|${config_path}|" "${kustomization}"
        sed -i "s|newName: ghcr.io/ironcore-dev/libvirt-provider|newName: libvirt-provider|" "${kustomization}"
        sed -i "/newName: libvirt-provider/{n;s|newTag:.*|newTag: local|;}" "${kustomization}"
    fi

    echo "    Patched to use config from: ${config_path}"
    echo "    Patched to use image: ${IIAB_IMG}"
}

deploy() {
    echo "==> Building kind node image..."
    ${CONTAINER_TOOL} build -t ironcore-dev/kind-node:local "${IIAB_DIR}"

    echo "==> Deleting existing kind cluster (if any)..."
    kind delete cluster --name "${IIAB_CLUSTER_NAME}" 2>/dev/null || true

    echo "==> Deploying ironcore stack..."
    make -C "${IIAB_DIR}" prepare ironcore ironcore-net apinetlet setup-network metalnetlet KIND_IMAGE=ironcore-dev/kind-node:local

    echo "==> Loading libvirt-provider image into kind cluster..."
    kind load docker-image "${IIAB_IMG}" --name "${IIAB_CLUSTER_NAME}"

    echo "==> Deploying libvirt-provider..."
    make -C "${IIAB_DIR}" libvirt-provider
}

run_tests() {
    echo "==> Running tests..."
    make -C "${IIAB_DIR}" test
}

clean() {
    echo "==> Cleaning up..."
    kind delete cluster --name "${IIAB_CLUSTER_NAME}" || true
    rm -rf "${IIAB_DIR}"
}

usage() {
    echo "Usage: $0 {all|clone|build|patch|deploy|test|clean}"
    echo ""
    echo "Commands:"
    echo "  all     Run the full iiab test flow (clone, build, patch, deploy, test)"
    echo "  clone   Clone or update ironcore-in-a-box"
    echo "  build   Build the local libvirt-provider Docker image"
    echo "  patch   Patch iiab kustomization to use local image and configs"
    echo "  deploy  Deploy the full ironcore stack with local libvirt-provider"
    echo "  test    Run iiab lint and tests"
    echo "  clean   Delete kind cluster and remove iiab clone"
    echo ""
    echo "Environment variables:"
    echo "  IIAB_DIR            Directory to clone iiab into (default: .iiab)"
    echo "  IIAB_REPO           Git URL for iiab (default: github.com/ironcore-dev/ironcore-in-a-box)"
    echo "  IIAB_IMG            Docker image tag (default: libvirt-provider:local)"
    echo "  IIAB_CLUSTER_NAME   Kind cluster name (default: ironcore-in-a-box)"
    echo "  CONTAINER_TOOL      Container runtime (default: docker)"
}

case "${1:-all}" in
    all)
        clone
        build
        patch
        deploy
        run_tests
        ;;
    clone)   clone ;;
    build)   build ;;
    patch)   patch ;;
    deploy)  deploy ;;
    test)    run_tests ;;
    clean)   clean ;;
    -h|--help|help)
        usage
        ;;
    *)
        echo "Unknown command: $1"
        usage
        exit 1
        ;;
esac
