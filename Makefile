.PHONY: build test lint clean proto

build:
	go build ./...

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -f gcp-eventarc-emulator

proto:
	buf generate
