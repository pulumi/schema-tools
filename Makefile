VERSION:=$(shell git describe --tags --dirty 2>/dev/null)
LDFLAGS:=$(if $(VERSION),-ldflags="-X github.com/pulumi/schema-tools/version.Version=$(VERSION)")

test:
	go test ./...

build:
	go build $(LDFLAGS)

lint:
	golangci-lint run

install:
	go install $(LDFLAGS)
