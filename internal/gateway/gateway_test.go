package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	longrunninggw "github.com/blackwell-systems/gcp-eventarc-emulator/internal/gen/google/longrunning"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestNew_NoServer verifies that gateway.New succeeds when given an address
// for a non-running gRPC server. grpc.NewClient is lazy — it does not attempt
// an actual TCP connection at construction time — so this test confirms the
// gateway initialises (handler registration, mux setup) without error.
func TestNew_NoServer(t *testing.T) {
	g, err := New("localhost:9999")
	if err != nil {
		t.Fatalf("New(\"localhost:9999\") returned unexpected error: %v", err)
	}
	if g == nil {
		t.Fatal("New returned nil gateway")
	}
	// Clean up — no server is listening so Stop may return a benign error;
	// we only care that the gateway object was created correctly.
	_ = g.conn.Close()
}

// fakeOpsClient is a minimal OperationsClient for unit-testing the LRO rewriter.
type fakeOpsClient struct {
	longrunninggw.OperationsClient // embed to satisfy interface

	getOp  func(ctx context.Context, req *longrunningpb.GetOperationRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error)
	listOp func(ctx context.Context, req *longrunningpb.ListOperationsRequest, opts ...grpc.CallOption) (*longrunningpb.ListOperationsResponse, error)
}

func (f *fakeOpsClient) GetOperation(ctx context.Context, req *longrunningpb.GetOperationRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error) {
	if f.getOp != nil {
		return f.getOp(ctx, req, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeOpsClient) ListOperations(ctx context.Context, req *longrunningpb.ListOperationsRequest, opts ...grpc.CallOption) (*longrunningpb.ListOperationsResponse, error) {
	if f.listOp != nil {
		return f.listOp(ctx, req, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// TestProjectScopedLRORewriter_GetOperation verifies that a GET for a
// project-scoped operation path is rewritten and forwarded to the underlying
// mux with the full project-scoped name embedded in the new path.
func TestProjectScopedLRORewriter_GetOperation(t *testing.T) {
	const opName = "projects/my-project/locations/us-central1/operations/abc123"

	// A fake inner mux that records the rewritten path and returns a 200.
	var gotPath string
	innerMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	// Build the rewriter with the real ServeMux for marshaler extraction, but
	// override the fallthrough to our fake inner mux.
	gwMux := runtime.NewServeMux()
	fake := &fakeOpsClient{} // GetOperation not called via rewriter for GET path
	handler := projectScopedLRORewriterFallback(fake, gwMux, innerMux)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/"+opName, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	want := "/v1/operations/" + opName
	if gotPath != want {
		t.Errorf("rewritten path = %q, want %q", gotPath, want)
	}
}

// TestProjectScopedLRORewriter_ListOperations verifies that a GET request for a
// project-scoped operations collection calls ListOperations with the correct parent.
func TestProjectScopedLRORewriter_ListOperations(t *testing.T) {
	const parent = "projects/my-project/locations/us-central1"
	const opName = parent + "/operations/abc123"

	var gotParent string
	fake := &fakeOpsClient{
		listOp: func(_ context.Context, req *longrunningpb.ListOperationsRequest, _ ...grpc.CallOption) (*longrunningpb.ListOperationsResponse, error) {
			gotParent = req.GetName()
			return &longrunningpb.ListOperationsResponse{
				Operations: []*longrunningpb.Operation{{Name: opName, Done: true}},
			}, nil
		},
	}

	gwMux := runtime.NewServeMux()
	handler := projectScopedLRORewriter(fake, gwMux)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/projects/my-project/locations/us-central1/operations", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotParent != parent {
		t.Errorf("ListOperations received parent %q, want %q", gotParent, parent)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	ops, _ := body["operations"].([]interface{})
	if len(ops) != 1 {
		t.Errorf("expected 1 operation in response, got %d", len(ops))
	}
}

// TestProjectScopedLRORewriter_NonLROPath verifies that paths not matching the
// project-scoped LRO pattern are forwarded unchanged to the mux.
func TestProjectScopedLRORewriter_NonLROPath(t *testing.T) {
	var gotPath string
	innerMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	fake := &fakeOpsClient{}
	gwMux := runtime.NewServeMux()
	handler := projectScopedLRORewriterFallback(fake, gwMux, innerMux)

	req := httptest.NewRequest(http.MethodGet, "/v1/triggers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if gotPath != "/v1/triggers" {
		t.Errorf("non-LRO path was modified: got %q, want %q", gotPath, "/v1/triggers")
	}
}

// TestHealthz verifies that GET /healthz returns HTTP 200 with {"status":"ok"}.
func TestHealthz(t *testing.T) {
	g, err := New("localhost:9999")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer g.conn.Close() //nolint:errcheck

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	g.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, body["status"])
	}
}

// TestReadyz verifies that GET /readyz returns HTTP 200 with {"status":"ok"}.
func TestReadyz(t *testing.T) {
	g, err := New("localhost:9999")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer g.conn.Close() //nolint:errcheck

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	g.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, body["status"])
	}
}

// projectScopedLRORewriterFallback is a test-only variant of projectScopedLRORewriter
// that accepts a custom fallback handler so tests can intercept forwarded requests.
func projectScopedLRORewriterFallback(opsClient longrunninggw.OperationsClient, mux *runtime.ServeMux, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const v1prefix = "/v1/"
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, v1prefix) {
			path := r.URL.Path[len(v1prefix):]
			lastOp := strings.LastIndex(path, "/operations")
			if lastOp >= 0 && strings.HasPrefix(path, "projects/") {
				suffix := path[lastOp+len("/operations"):]
				if suffix == "" || suffix == "/" {
					_, outboundMarshaler := runtime.MarshalerForRequest(mux, r)
					resp, err := opsClient.ListOperations(r.Context(),
						&longrunninggw.ListOperationsRequest{Name: path[:lastOp]})
					if err != nil {
						runtime.DefaultHTTPErrorHandler(r.Context(), mux, outboundMarshaler, w, r, err)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_ = outboundMarshaler.NewEncoder(w).Encode(resp)
					return
				}
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/v1/operations/" + path
				fallback.ServeHTTP(w, r2)
				return
			}
		}
		fallback.ServeHTTP(w, r)
	})
}
