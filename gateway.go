package gcp_eventarc_emulator

import (
	"net/http"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/gateway"
)

// NewGatewayHandler returns an http.Handler that proxies REST requests to the
// Eventarc gRPC service at grpcAddr. Used by gcp-emulator to mount the
// Eventarc REST API onto a unified HTTP server.
func NewGatewayHandler(grpcAddr string) (http.Handler, error) {
	gw, err := gateway.New(grpcAddr)
	if err != nil {
		return nil, err
	}
	return gw.Handler(), nil
}
