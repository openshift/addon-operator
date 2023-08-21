#!/bin/bash
set -euo pipefail

# this script ensures that the `govulncheck` dependency is present
# and then executes govulncheck targeting the root of the repository

./mage dependency:govulncheck
export GOFLAGS=""
exec .deps/bin/govulncheck ./...
