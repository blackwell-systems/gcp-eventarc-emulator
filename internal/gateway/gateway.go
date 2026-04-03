// Package gateway provides HTTP/REST gateway access to the gRPC Eventarc service
// using grpc-gateway v2 to transcode HTTP/JSON ↔ gRPC.
package gateway

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	publishingv1 "github.com/blackwell-systems/gcp-eventarc-emulator/internal/gen/google/cloud/eventarc/publishing/v1"
	eventarcv1 "github.com/blackwell-systems/gcp-eventarc-emulator/internal/gen/google/cloud/eventarc/v1"
	longrunninggw "github.com/blackwell-systems/gcp-eventarc-emulator/internal/gen/google/longrunning"
)

// Gateway transcodes HTTP/JSON requests to gRPC via grpc-gateway.
type Gateway struct {
	mux       *runtime.ServeMux
	httpSrv   *http.Server
	conn      *grpc.ClientConn
	opsClient longrunninggw.OperationsClient // for project-scoped LRO rewriter
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

	opsClient := longrunninggw.NewOperationsClient(conn)
	if err := longrunninggw.RegisterOperationsHandlerClient(ctx, mux, opsClient); err != nil {
		conn.Close()
		return nil, err
	}

	return &Gateway{mux: mux, conn: conn, opsClient: opsClient}, nil
}

// Start starts the HTTP gateway on the given address (non-blocking).
func (g *Gateway) Start(httpAddr string) error {
	ln, err := net.Listen("tcp", httpAddr)
	if err != nil {
		return err
	}
	g.httpSrv = &http.Server{Handler: projectScopedLRORewriter(g.opsClient, g.mux)}
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

// projectScopedLRORewriter returns an http.Handler that intercepts GET requests
// for project-scoped LRO paths and handles them appropriately before falling
// through to the grpc-gateway mux.
//
// The generated pb.gw.go only covers /v1/operations/... patterns, but LRO
// names are generated as projects/P/locations/L/operations/ID. This middleware
// bridges the gap:
//
//   - GET /v1/projects/{p}/locations/{l}/operations/{id}
//     Rewrites to GET /v1/operations/projects/{p}/locations/{l}/operations/{id}
//     so the generated handler passes the full name to GetOperation, which
//     matches the stored key exactly (lro.Store.GetOperation does TrimPrefix
//     of "operations/" which is a no-op for project-scoped names).
//
//   - GET /v1/projects/{p}/locations/{l}/operations
//     Calls ListOperations gRPC directly with Name="projects/{p}/locations/{l}",
//     which the store uses as a prefix filter.
func projectScopedLRORewriter(opsClient longrunninggw.OperationsClient, mux *runtime.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const v1prefix = "/v1/"
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, v1prefix) {
			// path is everything after /v1/, e.g. "projects/P/locations/L/operations/ID"
			path := r.URL.Path[len(v1prefix):]
			lastOp := strings.LastIndex(path, "/operations")
			if lastOp >= 0 && strings.HasPrefix(path, "projects/") {
				suffix := path[lastOp+len("/operations"):] // "" or "/ID"
				if suffix == "" || suffix == "/" {
					// ListOperations: parent is the portion before /operations
					parent := path[:lastOp] // "projects/P/locations/L"
					_, outboundMarshaler := runtime.MarshalerForRequest(mux, r)
					resp, err := opsClient.ListOperations(r.Context(),
						&longrunninggw.ListOperationsRequest{Name: parent})
					if err != nil {
						runtime.DefaultHTTPErrorHandler(r.Context(), mux,
							outboundMarshaler, w, r, err)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_ = outboundMarshaler.NewEncoder(w).Encode(resp)
					return
				}
				// GetOperation: rewrite /v1/projects/.../operations/ID
				// to /v1/operations/projects/.../operations/ID so the generated
				// handler receives the full project-scoped name as the `name` field.
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/v1/operations/" + path
				mux.ServeHTTP(w, r2)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
