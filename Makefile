.PHONY: run test vet build controls e2e
run:
	go run ./cmd/rcode
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
