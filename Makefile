SHELL := /bin/sh

.PHONY: test test-race vet fmt check-fmt check run build coverage

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

check-fmt:
	test -z "$$(gofmt -l .)"

check: check-fmt vet test-race build

run:
	go run ./cmd/secureledger

build:
	mkdir -p bin
	go build -buildvcs=false -trimpath -ldflags="-s -w" -o bin/secureledger ./cmd/secureledger

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
