VERSION=$(shell git describe --tags)

test:
	go test ./...

build:
	go build \
		-ldflags="-X github.com/pulumi/schema-tools/version.Version=$(VERSION)"

lint:
	golangci-lint run
