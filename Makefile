.PHONY: run test vet build
run:
	go run ./cmd/zeron
test:
	go test ./...
vet:
	go vet ./...
	go tool mygo vet .
build:
	go tool mygo build
