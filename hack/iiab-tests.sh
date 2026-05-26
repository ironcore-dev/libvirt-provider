#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

IIAB_DIR="${IIAB_DIR:-${REPO_ROOT}/.iiab}"
IIAB_REPO="${IIAB_REPO:-https://github.com/ironcore-dev/ironcore-in-a-box.git}"
IIAB_BRANCH="${IIAB_BRANCH:-main}"
IIAB_LOCAL="${IIAB_LOCAL:-}"
CONTAINER_TOOL="${CONTAINER_TOOL:-docker}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-iiab-libvirt-provider-test}"

LIBVIRT_PROVIDER_CONFIG_DIR="${REPO_ROOT}/config/default"
LIBVIRT_PROVIDER_IMAGE_TAG="local"

clean() {
    echo "==> Deleting kind cluster '${KIND_CLUSTER_NAME}'..."
    kind delete cluster --name "${KIND_CLUSTER_NAME}" 2>/dev/null || true
    if [ -n "${IIAB_LOCAL}" ]; then
        echo "    IIAB_LOCAL is set, keeping ${IIAB_DIR}"
    else
        echo "==> Removing ${IIAB_DIR}..."
        rm -rf "${IIAB_DIR}"
    fi
}

if [ "${1:-}" = "clean" ]; then
    clean
    exit 0
fi

if [ -n "${IIAB_LOCAL}" ]; then
    echo "==> Using local ironcore-in-a-box at ${IIAB_DIR}"
    if [ ! -d "${IIAB_DIR}" ]; then
        echo "ERROR: IIAB_LOCAL is set but ${IIAB_DIR} does not exist" >&2
        exit 1
    fi
else
    echo "==> Cloning/updating ironcore-in-a-box (branch: ${IIAB_BRANCH})..."
    if [ -d "${IIAB_DIR}" ]; then
        cd "${IIAB_DIR}" && git fetch && git checkout "${IIAB_BRANCH}" && git pull && git submodule update --init --recursive
    else
        git clone --recurse-submodules --branch "${IIAB_BRANCH}" "${IIAB_REPO}" "${IIAB_DIR}"
    fi
fi

echo "==> Building libvirt-provider image..."
${CONTAINER_TOOL} build -t "ghcr.io/ironcore-dev/libvirt-provider:${LIBVIRT_PROVIDER_IMAGE_TAG}" "${REPO_ROOT}"

cd "${IIAB_DIR}"

echo "==> Cleaning up any existing test cluster '${KIND_CLUSTER_NAME}'..."
kind delete cluster --name "${KIND_CLUSTER_NAME}" 2>/dev/null || true

echo "==> Running tests..."
make test \
    KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME}" \
    LIBVIRT_PROVIDER_CONFIG_DIR="${LIBVIRT_PROVIDER_CONFIG_DIR}" \
    LIBVIRT_PROVIDER_IMAGE_TAG="${LIBVIRT_PROVIDER_IMAGE_TAG}"
