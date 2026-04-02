// Package gateway provides HTTP/REST gateway access to the gRPC Eventarc service.
//
// NOTE: This is a build scaffold stub created by Wave 4 Agent C to allow
// cmd/server-rest and cmd/server-dual to compile. The full implementation
// is owned by Wave 4 Agent B (internal/gateway/gateway.go) and will replace
// this stub during integration.
package gateway

import (
	"context"
	"fmt"
	"net/http"
)

// Gateway proxies REST/HTTP requests to a backend gRPC server.
type Gateway struct {
	grpcAddr   string
	httpServer *http.Server
}

// New creates a new Gateway that proxies REST requests to the gRPC server at grpcAddr.
func New(grpcAddr string) (*Gateway, error) {
	return &Gateway{grpcAddr: grpcAddr}, nil
}

// Start starts the HTTP gateway on the given address.
// This method blocks until the server is stopped or encounters an error.
func (g *Gateway) Start(httpAddr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"healthy"}`)
	})

	g.httpServer = &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}
	return g.httpServer.ListenAndServe()
}

// Stop gracefully shuts down the HTTP gateway.
func (g *Gateway) Stop(ctx context.Context) error {
	if g.httpServer != nil {
		return g.httpServer.Shutdown(ctx)
	}
	return nil
}
