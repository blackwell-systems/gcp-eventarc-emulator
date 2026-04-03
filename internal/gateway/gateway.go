// Package gateway provides HTTP/REST gateway access to the gRPC Eventarc service
// using grpc-gateway v2 to transcode HTTP/JSON ↔ gRPC.
package gateway

import (
	"context"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	publishingv1 "github.com/blackwell-systems/gcp-eventarc-emulator/internal/gen/google/cloud/eventarc/publishing/v1"
	eventarcv1 "github.com/blackwell-systems/gcp-eventarc-emulator/internal/gen/google/cloud/eventarc/v1"
	longrunninggw "github.com/blackwell-systems/gcp-eventarc-emulator/internal/gen/google/longrunning"
)

// Gateway transcodes HTTP/JSON requests to gRPC via grpc-gateway.
type Gateway struct {
	mux     *runtime.ServeMux
	httpSrv *http.Server
	conn    *grpc.ClientConn
}

// New creates a Gateway that proxies REST requests to the gRPC server at grpcAddr.
func New(grpcAddr string) (*Gateway, error) {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	mux := runtime.NewServeMux()
	ctx := context.Background()

	if err := eventarcv1.RegisterEventarcHandlerClient(ctx, mux, eventarcv1.NewEventarcClient(conn)); err != nil {
		conn.Close()
		return nil, err
	}

	if err := publishingv1.RegisterPublisherHandlerClient(ctx, mux, publishingv1.NewPublisherClient(conn)); err != nil {
		conn.Close()
		return nil, err
	}

	if err := longrunninggw.RegisterOperationsHandlerClient(ctx, mux, longrunninggw.NewOperationsClient(conn)); err != nil {
		conn.Close()
		return nil, err
	}

	return &Gateway{mux: mux, conn: conn}, nil
}

// Start starts the HTTP gateway on the given address (non-blocking).
func (g *Gateway) Start(httpAddr string) error {
	ln, err := net.Listen("tcp", httpAddr)
	if err != nil {
		return err
	}
	g.httpSrv = &http.Server{Handler: g.mux}
	go g.httpSrv.Serve(ln) //nolint:errcheck
	return nil
}

// Stop gracefully shuts down the HTTP gateway and closes the gRPC connection.
func (g *Gateway) Stop(ctx context.Context) error {
	var httpErr error
	if g.httpSrv != nil {
		httpErr = g.httpSrv.Shutdown(ctx)
	}
	if g.conn != nil {
		if err := g.conn.Close(); err != nil && httpErr == nil {
			return err
		}
	}
	return httpErr
}
