.PHONY: run test vet build controls e2e

DESKTOP_BINARY := .mygo/run/GCode
ifeq ($(OS),Windows_NT)
DESKTOP_BINARY := $(DESKTOP_BINARY).exe
endif

run:
	go build -o "$(DESKTOP_BINARY)" ./cmd/rcode
	"$(DESKTOP_BINARY)"
test:
	go test ./...
vet:
	go vet ./...
	go tool mygo vet .
build:
	go tool mygo build
controls:
	go test ./internal/desktop -run 'Test(SettingsControls|InspectSettingsControls)'
e2e:
	go run ./scripts/desktop-e2e
