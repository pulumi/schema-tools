VERSION:=$(shell git describe --tags)
LDFLAGS=-ldflags="-X github.com/pulumi/schema-tools/version.Version=$(VERSION)"

test:
	go test ./...

build:
	go build $(LDFLAGS)

lint:
	golangci-lint run

install:
	go install $(LDFLAGS)
