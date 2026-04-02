// GCP Eventarc Emulator - Dual Protocol
//
// Provides both gRPC and REST/HTTP APIs for the Google Cloud Eventarc service.
// This server exposes both protocols simultaneously for maximum flexibility.
//
// Usage:
//
//	gcp-eventarc-emulator-dual --grpc-port 9085 --http-port 8085
//
// Environment Variables:
//
//	EVENTARC_EMULATOR_HOST  - Host and port for the gRPC server (e.g. localhost:9085)
//	EVENTARC_HTTP_PORT      - HTTP port to listen on (default: 8085)
//	GCP_MOCK_LOG_LEVEL      - Log level: debug, info, warn, error (default: info)
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
	grpcPort = flag.Int("grpc-port", getEnvPort("EVENTARC_EMULATOR_HOST", 9085), "gRPC port to listen on")
	httpPort = flag.Int("http-port", getEnvInt("EVENTARC_HTTP_PORT", 8085), "HTTP port to listen on")
	logLevel = flag.String("log-level", getEnv("GCP_MOCK_LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	version  = "0.1.0"
)

func main() {
	flag.Parse()

	log.Printf("GCP Eventarc Emulator v%s (gRPC + REST)", version)
	log.Printf("Log level: %s", *logLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start gRPC server on the external-facing port
	grpcAddr := fmt.Sprintf(":%d", *grpcPort)
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
	pub := publisher.NewServer(rtr, dsp)

	// Create the shared gRPC server with all services registered
	grpcSrv := server.NewGRPCServer(srv, pub)

	// Start gRPC server in background (externally exposed)
	go func() {
		log.Printf("gRPC server listening at %v", lis.Addr())
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// Start REST gateway proxying to the local gRPC server
	gw, err := gateway.New(fmt.Sprintf("localhost:%d", *grpcPort))
	if err != nil {
		log.Fatalf("Failed to create REST gateway: %v", err)
	}

	httpAddr := fmt.Sprintf(":%d", *httpPort)
	go func() {
		log.Printf("HTTP gateway listening at %s", httpAddr)
		log.Printf("Ready to accept both gRPC and REST requests")
		log.Printf("gRPC: localhost:%d", *grpcPort)
		log.Printf("REST: http://localhost:%d/v1/projects/{project}/locations/{location}/triggers", *httpPort)
		if err := gw.Start(httpAddr); err != nil {
			log.Fatalf("Failed to serve HTTP: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down servers...")

	// Shutdown REST gateway first, then gRPC server
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

// getEnvPort extracts the port number from a "host:port" environment variable.
// Falls back to defaultPort if the variable is unset or unparseable.
func getEnvPort(key string, defaultPort int) int {
	if value := os.Getenv(key); value != "" {
		var host string
		var p int
		if n, err := fmt.Sscanf(value, "%s:%d", &host, &p); n == 2 && err == nil {
			return p
		}
		// Try just a bare port number
		var bare int
		if _, err := fmt.Sscanf(value, "%d", &bare); err == nil {
			return bare
		}
	}
	return defaultPort
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
