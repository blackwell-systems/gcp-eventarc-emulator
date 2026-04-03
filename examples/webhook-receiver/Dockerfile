FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -o /webhook-receiver .

FROM alpine:3.21
COPY --from=builder /webhook-receiver /usr/local/bin/webhook-receiver
EXPOSE 3000
ENTRYPOINT ["webhook-receiver"]
