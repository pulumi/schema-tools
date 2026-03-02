VERSION:=$(shell git describe --tags)
LDFLAGS=-ldflags="-X github.com/pulumi/schema-tools/version.Version=$(VERSION)"

test:
	go test ./...

build:
	go build $(LDFLAGS)

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.0 run

install:
	go install $(LDFLAGS)
