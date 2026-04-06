# Dockerfile for GCP Eventarc Emulator
# Multi-stage build with variant selection via build args
#
# Build variants:
#   docker build --build-arg VARIANT=dual -t emulator:dual .   # gRPC + REST (default)
#   docker build --build-arg VARIANT=grpc -t emulator:grpc .   # gRPC only
#   docker build --build-arg VARIANT=rest -t emulator:rest .   # REST only

# Build stage
FROM golang:1.24-alpine AS builder

ARG VARIANT=dual

WORKDIR /build

# Install git for private Go module downloads
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN case "${VARIANT}" in \
    dual) \
        CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server-dual \
        ;; \
    grpc) \
        CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server \
        ;; \
    rest) \
        CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server-rest \
        ;; \
    *) \
        echo "Invalid VARIANT: ${VARIANT}. Must be dual, grpc, or rest" && exit 1 \
        ;; \
    esac

# Runtime stage
FROM alpine:3.21

ARG VARIANT=dual

RUN apk --no-cache add --no-scripts ca-certificates && \
    update-ca-certificates || true

WORKDIR /app

COPY --from=builder /build/server .

RUN addgroup -g 1000 gcpmock && \
    adduser -D -u 1000 -G gcpmock gcpmock && \
    chown -R gcpmock:gcpmock /app

USER gcpmock

EXPOSE 9085 8085

ENV GCP_MOCK_LOG_LEVEL=info

LABEL org.opencontainers.image.title="GCP Eventarc Emulator (${VARIANT})"
LABEL org.opencontainers.image.description="Local implementation of the GCP Eventarc API"
LABEL org.opencontainers.image.variant="${VARIANT}"
LABEL org.opencontainers.image.source="https://github.com/blackwell-systems/gcp-eventarc-emulator"

ENTRYPOINT ["/app/server"]
