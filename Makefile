.PHONY: test build install

test:
	go test ./...

build:
	go build -ldflags "-s -w" -o bin/quota ./cmd/quota

install: build
	./install.sh
