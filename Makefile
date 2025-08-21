BIN=wlim

.PHONY: build test fmt lint

build:
	go build -ldflags "-s -w -X wlim/internal/version.Version=$$(git describe --tags --always 2>/dev/null || echo dev)" -o $(BIN)

test:
	GOCACHE=$$(pwd)/.gocache GOMODCACHE=$$(pwd)/.gomodcache go test ./...

fmt:
	go fmt ./...
