// GCP Eventarc Emulator - REST API
//
// A REST/HTTP emulator of the Google Cloud Eventarc API for local testing.
// This server runs a gRPC backend on an internal port with an HTTP/REST
// gateway frontend. External clients use HTTP only.
//
// Usage:
//
//	gcp-eventarc-emulator-rest --http-port 8085 --grpc-port 9086
//
// Environment Variables:
//
//	EVENTARC_HTTP_PORT  - HTTP port to listen on (default: 8085)
//	GCP_MOCK_LOG_LEVEL  - Log level: debug, info, warn, error (default: info)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/dispatcher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/gateway"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/publisher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/router"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/server"
)

var (
	httpPort = flag.Int("http-port", getEnvInt("EVENTARC_HTTP_PORT", 8085), "HTTP port to listen on")
	grpcPort = flag.Int("grpc-port", 9086, "gRPC port to listen on (internal only)")
	logLevel = flag.String("log-level", getEnv("GCP_MOCK_LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	version  = "0.1.0"
)

func main() {
	flag.Parse()

	log.Printf("GCP Eventarc Emulator v%s (REST)", version)
	log.Printf("Starting gRPC backend on internal port %d", *grpcPort)
	log.Printf("Starting HTTP gateway on port %d", *httpPort)
	log.Printf("Log level: %s", *logLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start gRPC server on internal (non-exposed) port
	grpcAddr := fmt.Sprintf("localhost:%d", *grpcPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("Failed to listen on gRPC port: %v", err)
	}

	// Create emulator service (Eventarc CRUD + LRO store)
	srv, err := server.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Wire up router and dispatcher
	rtr := router.NewRouter(srv.Storage())
	dsp := dispatcher.NewDispatcher(nil)

	// Create publisher gRPC service
	pub := publisher.NewServer(rtr, dsp, srv.Storage())

	// Create the shared gRPC server with all services registered
	grpcSrv := server.NewGRPCServer(srv, pub)

	// Start gRPC server in background (internal only — not exposed to clients)
	go func() {
		log.Printf("gRPC backend listening at %v (internal)", lis.Addr())
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// Start REST gateway proxying to the internal gRPC server
	gw, err := gateway.New(grpcAddr)
	if err != nil {
		log.Fatalf("Failed to create REST gateway: %v", err)
	}

	httpAddr := fmt.Sprintf(":%d", *httpPort)
	go func() {
		log.Printf("HTTP gateway listening at %s", httpAddr)
		log.Printf("Ready to accept REST requests")
		if err := gw.Start(httpAddr); err != nil {
			log.Fatalf("Failed to serve HTTP: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down servers...")

	// Shutdown REST gateway first, then gRPC backend
	if err := gw.Stop(ctx); err != nil {
		log.Printf("Error stopping HTTP gateway: %v", err)
	}
	grpcSrv.GracefulStop()

	log.Println("Servers stopped")
}

// getEnv returns environment variable value or default.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt returns an integer environment variable or the default value.
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return defaultValue
}
