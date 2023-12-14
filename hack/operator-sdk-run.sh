#!/bin/bash
set -euo pipefail

make operatorSDK

file_path="$(pwd)/bundle/manifests/addon-operator.clusterserviceversion.yaml"

if [[ -e "$file_path" ]]; then
line=$(git diff -U0 -I'^   createdAt: ')
  if [[  -n "$line" ]] ;then
      make generate-bundle
  fi
else
      make generate-bundle
fi
