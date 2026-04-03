.PHONY: build test test-race test-integration lint clean proto

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

test-integration:
	go test -v -run TestIntegration ./...

lint:
	go vet ./...

clean:
	rm -f gcp-eventarc-emulator

proto:
	buf generate
