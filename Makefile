.PHONY: build build-dual build-grpc build-rest \
        run run-dual run-grpc run-rest \
        test test-race test-integration \
        fmt fmt-check lint clean \
        docker-build docker-build-dual docker-build-grpc docker-build-rest \
        demo proto

# ── Build ─────────────────────────────────────────────────────────────────────

build: build-dual build-grpc build-rest

build-dual:
	go build -o bin/server-dual ./cmd/server-dual

build-grpc:
	go build -o bin/server ./cmd/server

build-rest:
	go build -o bin/server-rest ./cmd/server-rest

# ── Run ───────────────────────────────────────────────────────────────────────

run: run-dual

run-dual:
	go run ./cmd/server-dual

run-grpc:
	go run ./cmd/server

run-rest:
	go run ./cmd/server-rest

# ── Test ──────────────────────────────────────────────────────────────────────

test:
	go test ./...

test-race:
	go test -race ./...

test-integration:
	go test -v -run TestIntegration ./...

# ── Format & Lint ─────────────────────────────────────────────────────────────

fmt:
	@find . -name '*.go' -not -path '*/vendor/*' | xargs gofmt -w

fmt-check:
	@out=$$(find . -name '*.go' -not -path '*/vendor/*' | xargs gofmt -l); \
	if [ -n "$$out" ]; then printf "unformatted files:\n$$out\n"; exit 1; fi

lint:
	go vet ./...

# ── Docker ────────────────────────────────────────────────────────────────────

docker-build: docker-build-dual docker-build-grpc docker-build-rest

docker-build-dual:
	docker build --build-arg VARIANT=dual -t gcp-eventarc-emulator:dual .

docker-build-grpc:
	docker build --build-arg VARIANT=grpc -t gcp-eventarc-emulator:grpc .

docker-build-rest:
	docker build --build-arg VARIANT=rest -t gcp-eventarc-emulator:rest .

# ── Demo ──────────────────────────────────────────────────────────────────────

demo:
	docker compose up --build

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/

# ── Proto ─────────────────────────────────────────────────────────────────────

proto:
	buf generate
