package gateway

import (
	"testing"
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
