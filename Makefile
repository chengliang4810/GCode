.DEFAULT_GOAL := build

.PHONY: build dev package test test-race vet fmt fmt-check tidy clean

build:
	mkdir -p build
	go build -buildvcs=false -o build/gcode ./cmd/gcode

dev:
	GOFLAGS=-buildvcs=false go tool mygo dev

package:
	GOFLAGS=-buildvcs=false go tool mygo build --skip-dmg

test:
	go test ./...

test-race:
	go test -race ./internal/controller ./internal/runtime ./internal/application ./internal/execution ./internal/workspace ./internal/worktrees ./internal/tools

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal

fmt-check:
	@test -z "$$(gofmt -l cmd internal)"

tidy:
	go mod tidy

clean:
	rm -rf build
