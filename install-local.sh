#!/bin/bash

set -e

# Get the directory where the script is located and change to it
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Usage: ./install-local.sh [options]
# Example: ./install-local.sh --helm-release-name my-gateway

# Default values
HELM_RELEASE_NAME="local-dev"
DOCKER_IMAGE_TAG="dev"
UNINSTALL=false

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --uninstall)
            UNINSTALL=true
            shift
            ;;
        --helm-release-name)
            HELM_RELEASE_NAME="$2"
            shift 2
            ;;
        --docker-image-tag)
            DOCKER_IMAGE_TAG="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --uninstall                        Uninstall the helm release"
            echo "  --helm-release-name <name>         Helm release name (default: local-dev)"
            echo "  --docker-image-tag <tag>           Docker image tag (default: local-dev)"
            echo "  -h, --help                         Show this help message"
            exit 0
            ;;
        *)
            echo "Error: Unknown parameter $1"
            echo "Use --help to see available options"
            exit 1
            ;;
    esac
done

# Handle uninstall
if [ "$UNINSTALL" = true ]; then
    echo "======================================"
    echo "ArgoCD Gateway Uninstall"
    echo "======================================"
    echo "Helm Release Name: $HELM_RELEASE_NAME"
    echo "Namespace: octo-argo-gateway-local-dev"
    echo "======================================"
    echo ""

    echo "Uninstalling Helm release..."
    helm uninstall "$HELM_RELEASE_NAME" --namespace octo-argo-gateway-local-dev

    echo ""
    echo "======================================"
    echo "✓ Uninstall completed successfully!"
    echo "======================================"
    echo ""
    echo "Note: The namespace 'octo-argo-gateway-local-dev' still exists."
    echo "To delete it, run: kubectl delete namespace octo-argo-gateway-local-dev"
    exit 0
fi

echo "======================================"
echo "ArgoCD Gateway Local Installation"
echo "======================================"
echo "Helm Release Name: $HELM_RELEASE_NAME"
echo "Docker Image Tag: $DOCKER_IMAGE_TAG"
echo "======================================"
echo ""

# Step 1: Build Docker image
echo "Step 1: Building Docker image..."
docker build -t "octopusdeploy/octopus-argocd-gateway:${DOCKER_IMAGE_TAG}" .
echo "✓ Docker image built successfully"
echo ""


# Step 2: Install with Helm
echo "Step 3: Installing ArgoCD Gateway with Helm..."

# Determine which values file to use
VALUES_FILE="install-local-values-default.yaml"
if [ -f "install-local-values-override.yaml" ]; then
    VALUES_FILE="install-local-values-override.yaml"
    echo "Using override values file: install-local-values-override.yaml"
else
    echo "Using default values file: install-local-values-default.yaml"
fi

# Check if the values file exists
if [ ! -f "$VALUES_FILE" ]; then
    echo "Error: Values file not found: $VALUES_FILE"
    exit 1
fi

helm upgrade --install --atomic \
--create-namespace --namespace octo-argo-gateway-local-dev \
--version "*.*" \
--reset-then-reuse-values \
-f "$VALUES_FILE" \
--set image.tag="${DOCKER_IMAGE_TAG}" \
"${HELM_RELEASE_NAME}" \
charts/octopus-argocd-gateway

echo ""
echo "======================================"
echo "✓ Installation completed successfully!"
echo "======================================"