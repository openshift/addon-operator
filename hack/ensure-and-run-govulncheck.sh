#!/bin/bash
set -euo pipefail

# this script ensures that the `govulncheck` dependency is present
# and then executes govulncheck

./mage dependency:govulncheck
export GOFLAGS=""
exec .deps/bin/govulncheck ./...
