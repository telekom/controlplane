#!/bin/bash

# Script to download technology logos for the documentation
# Creates a logos directory and downloads logos from official sources

LOGOS_DIR="../static/img/logos"
mkdir -p $LOGOS_DIR

# Function to download a logo
# Usage: download_logo URL FILENAME
download_logo() {
  echo "Downloading $2..."
  curl -s "$1" -o "$LOGOS_DIR/$2"
  if [ $? -eq 0 ]; then
    echo "✅ Successfully downloaded $2"
  else
    echo "❌ Failed to download $2"
  fi
}

# Download logos
download_logo "https://helm.sh/img/helm-logo.svg" "helm-logo.svg"
download_logo "https://min.io/resources/img/logo/MINIO_wordmark.png" "minio-logo.png"
download_logo "https://prometheus.io/assets/prometheus_logo_orange_circle.svg" "prometheus-logo.svg"
download_logo "https://github.com/gofiber/docs/raw/master/static/fiber_v2_github_assets/fiber_logo.svg" "gofiber-logo.svg" 
download_logo "https://raw.githubusercontent.com/uber-go/zap/master/.github/logo.png" "zap-logo.png"
download_logo "https://raw.githubusercontent.com/kubernetes/kubernetes/master/logo/logo.svg" "kubernetes-logo.svg"
download_logo "https://raw.githubusercontent.com/kubernetes-sigs/kubebuilder/master/docs/book/src/logos/logo.svg" "kubebuilder-logo.svg"
download_logo "https://raw.githubusercontent.com/onsi/ginkgo/master/ginkgo.png" "ginkgo-logo.png"
download_logo "https://raw.githubusercontent.com/onsi/gomega/master/gomega.png" "gomega-logo.png"
download_logo "https://raw.githubusercontent.com/deepmap/oapi-codegen/master/deepmap-logo.svg" "oapi-codegen-logo.svg"
download_logo "https://raw.githubusercontent.com/go-testify/testify/master/.github/testify-banner.svg" "testify-logo.svg"
download_logo "https://raw.githubusercontent.com/spf13/cobra/master/logo.png" "cobra-logo.png"

echo "Logo download complete"