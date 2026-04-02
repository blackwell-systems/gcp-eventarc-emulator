// GCP Eventarc Emulator
//
// A lightweight emulator of the Google Cloud Eventarc API for local testing.
// This server implements the gRPC Eventarc API without requiring GCP credentials.
//
// Usage:
//
//	gcp-eventarc-emulator --port 9085
//
// Environment Variables:
//
//	EVENTARC_EMULATOR_HOST - Host and port to listen on (e.g. localhost:9085)
//	GCP_MOCK_LOG_LEVEL     - Log level: debug, info, warn, error (default: info)
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/server"
)

var (
	port     = flag.Int("port", getEnvPort("EVENTARC_EMULATOR_HOST", 9085), "Port to listen on")
	logLevel = flag.String("log-level", getEnv("GCP_MOCK_LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	version  = "0.1.0"
)

func main() {
	flag.Parse()

	log.Printf("GCP Eventarc Emulator v%s", version)
	log.Printf("Starting on port %d with log level: %s", *port, *logLevel)

	// Create listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create and register emulator service
	srv, err := server.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// NewGRPCServer wired by Wave 3 Agent C once publisher exists
	grpcServer := grpc.NewServer()
	eventarcpb.RegisterEventarcServer(grpcServer, srv)
	reflection.Register(grpcServer)

	log.Printf("Server listening at %v", lis.Addr())
	log.Printf("Ready to accept connections")

	// Start server in goroutine
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	grpcServer.GracefulStop()
	log.Println("Server stopped")
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
