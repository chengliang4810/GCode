#!/bin/sh
# On macOS this also creates the platform's signed/ad-hoc signed app and DMG.
# On other hosts the same command creates the Mach-O app bundles, including
# universal executable/library slices; signing and DMG steps remain unavailable.
set -eu
TASK_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$TASK_ROOT"
export CGO_ENABLED=0
export NO_COLOR=1
export GOWORK=off
TASK_GO_VERSION=$(go env GOVERSION)
[ "$TASK_GO_VERSION" = go1.27.3 ] || { echo "Expected go1.27.3, found $TASK_GO_VERSION" >&2; exit 1; }
TASK_MYGO_VERSION=$(go list -m -f '{{.Version}}' github.com/egoist/mygo)
[ "$TASK_MYGO_VERSION" = v0.6.4 ] || { echo "Expected MyGo v0.6.4, found $TASK_MYGO_VERSION" >&2; exit 1; }
MYGO_LDFLAGS=$(go run ./scripts/check-framework.go -ldflags)
export MYGO_LDFLAGS
go run ./scripts/check-distribution.go
go test ./...
go vet ./...
go tool mygo vet .
go tool mygo build -platform darwin/amd64,darwin/arm64,darwin/universal -o build -skip-notarize
go run ./scripts/verify-package.go -root build -targets darwin/amd64,darwin/arm64,darwin/universal
