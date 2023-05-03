test:
	go test ./...

build:
	go build

lint:
	golangci-lint run
