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
download_logo "https://dl.min.io/logo/Minio_logo_light/Minio_logo_light.svg" "minio-logo.svg"
download_logo "https://upload.wikimedia.org/wikipedia/commons/3/38/Prometheus_software_logo.svg" "prometheus-logo.svg"
download_logo "https://raw.githubusercontent.com/gofiber/docs/master/static/img/logo-dark.svg" "gofiber-logo.svg"
download_logo "https://raw.githubusercontent.com/uber-go/zap/master/assets/logo.png" "zap-logo.png"
download_logo "https://raw.githubusercontent.com/kubernetes/kubernetes/master/logo/logo.svg" "kubernetes-logo.svg"
download_logo "https://raw.githubusercontent.com/kubernetes-sigs/kubebuilder/master/docs/book/src/logos/logo-single-line.png" "kubebuilder-logo.png"
download_logo "https://raw.githubusercontent.com/onsi/ginkgo/master/docs/images/ginkgo.png" "ginkgo-logo.png"
download_logo "https://raw.githubusercontent.com/onsi/gomega/master/docs/images/gomega.png" "gomega-logo.png"
download_logo "https://upload.wikimedia.org/wikipedia/commons/6/61/OpenAPI_Logo_Pantone.svg" "oapi-codegen-logo.svg"
#download_logo "https://raw.githubusercontent.com/stretchr/testify/master/logo/testify.png" "testify-logo.png"
download_logo "https://raw.githubusercontent.com/spf13/cobra/main/assets/CobraMain.png" "cobra-logo.png"
download_logo "https://upload.wikimedia.org/wikipedia/commons/3/35/JWT-Logo.svg" "jwt-logo.svg"

echo "Logo download complete"