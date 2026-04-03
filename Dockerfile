# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server-dual ./cmd/server-dual

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates
COPY --from=builder /server-dual /usr/local/bin/server-dual

EXPOSE 9085 8085

ENTRYPOINT ["server-dual"]
