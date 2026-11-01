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
if [ -z "${MYGO_UPDATER_PRIVATE_KEY:-}" ]; then
    TASK_UPDATE_KEY_FILE=${RCODE_UPDATE_PRIVATE_KEY_FILE:-"$HOME/.rcode/update-keys/mygo-update.key"}
    [ -f "$TASK_UPDATE_KEY_FILE" ] || { echo "Release signing key missing; see docs/updates.md." >&2; exit 1; }
    MYGO_UPDATER_PRIVATE_KEY=$(cat "$TASK_UPDATE_KEY_FILE")
    export MYGO_UPDATER_PRIVATE_KEY
fi
go run ./scripts/check-updates.go -check-key
TASK_GO_VERSION=$(go env GOVERSION)
[ "$TASK_GO_VERSION" = go1.27.3 ] || { echo "Expected go1.27.3, found $TASK_GO_VERSION" >&2; exit 1; }
TASK_MYGO_VERSION=$(go list -m -f '{{.Version}}' github.com/egoist/mygo)
[ "$TASK_MYGO_VERSION" = v0.7.3 ] || { echo "Expected MyGo v0.7.3, found $TASK_MYGO_VERSION" >&2; exit 1; }
MYGO_LDFLAGS=$(go run ./scripts/check-framework.go -ldflags)
export MYGO_LDFLAGS
go run ./scripts/check-distribution.go
go test ./...
go vet ./...
go tool mygo vet .
go tool mygo build -platform darwin/amd64,darwin/arm64,darwin/universal -o build -skip-notarize
go run ./scripts/verify-package.go -root build -targets darwin/amd64,darwin/arm64,darwin/universal
go run ./scripts/check-updates.go -root build -targets darwin/amd64,darwin/arm64,darwin/universal
