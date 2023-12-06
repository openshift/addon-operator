#!/bin/bash
set -euo pipefail

make operatorSDK

file_path="$(pwd)/bundle/manifests/addon-operator.clusterserviceversion.yaml"

if [ -e "$file_path" ]; then
line=$(git diff -I'^    createdAt: ' bundle/manifests/addon-operator.clusterserviceversion.yaml | wc -l)
  if $(line)>0 ;then
  make generate-bundle
  fi
else
    make generate-bundle
fi


