// GCP Eventarc Emulator
//
// A lightweight emulator of the Google Cloud Eventarc API for local testing.
// This server implements the gRPC Eventarc API without requiring GCP credentials.
//
// When to use: gRPC-only workloads. The REST gateway is not included.
//
// Usage:
//
//	gcp-eventarc-emulator --grpc-port 9085
//
// Environment Variables:
//
//	EVENTARC_EMULATOR_HOST  - Host and port for the gRPC server (e.g. localhost:9085)
//	GCP_MOCK_LOG_LEVEL      - Log level: debug, info, warn, error (default: info)
//	IAM_MODE                - IAM enforcement: off, permissive, strict (default: off)
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/dispatcher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/logger"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/publisher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/router"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/server"
)

var (
	// Keep --port for backwards compatibility; add --grpc-port as the canonical name.
	port     = flag.Int("port", getEnvPort("EVENTARC_EMULATOR_HOST", 9085), "Port to listen on (deprecated: use --grpc-port)")
	grpcPort = flag.Int("grpc-port", getEnvPort("EVENTARC_EMULATOR_HOST", 9085), "gRPC port to listen on")
	logLevel = flag.String("log-level", getEnv("GCP_MOCK_LOG_LEVEL", "info"), "Log level (debug, info, warn, error) (env: GCP_MOCK_LOG_LEVEL; flag takes precedence)")
	version  = "0.1.0"
)

// validateLogLevel returns an error if the log level is not one of the
// accepted values.
func validateLogLevel(level string) error {
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("invalid --log-level %q: must be one of: debug, info, warn, error", level)
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "GCP Eventarc Emulator v%s — gRPC-only server\n", version)
		fmt.Fprintf(os.Stderr, "When to use: gRPC-only workloads. The REST gateway is not included.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: server [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  EVENTARC_EMULATOR_HOST  gRPC host:port for clients (e.g. localhost:9085)\n")
		fmt.Fprintf(os.Stderr, "  GCP_MOCK_LOG_LEVEL      Log level: debug, info, warn, error (default: info)\n")
		fmt.Fprintf(os.Stderr, "  IAM_MODE                IAM enforcement: off, permissive, strict (default: off)\n")
	}

	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("GCP Eventarc Emulator v%s (server)\n", version)
		os.Exit(0)
	}

	if err := validateLogLevel(*logLevel); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	portWasSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "port" {
			portWasSet = true
		}
	})
	if portWasSet {
		fmt.Fprintln(os.Stderr, "WARNING: --port is deprecated; use --grpc-port instead")
	}

	if *grpcPort < 1 || *grpcPort > 65535 {
		fmt.Fprintf(os.Stderr, "invalid --grpc-port %d: must be in range 1-65535\n", *grpcPort)
		os.Exit(1)
	}

	lgr := logger.New(*logLevel)

	// Resolve listen port: --grpc-port takes precedence if explicitly set;
	// fall back to --port (deprecated) for backwards compatibility.
	listenPort := *grpcPort
	if listenPort == 9085 && *port != 9085 {
		// --port was explicitly set but --grpc-port was not
		listenPort = *port
	}

	lgr.Info("GCP Eventarc Emulator v%s", version)
	lgr.Info("Log level: %s", *logLevel)

	// Create listener for gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", listenPort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create emulator service (Eventarc CRUD + LRO store)
	srv, err := server.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	lgr.Info("IAM mode: %s", srv.IAMMode())
	if token := os.Getenv("EVENTARC_EMULATOR_TOKEN"); token != "" {
		lgr.Info("Bearer token injection: enabled")
	}

	// Wire up router and dispatcher
	rtr := router.NewRouter(srv.Storage(), lgr)
	dsp := dispatcher.NewDispatcher(nil, lgr)

	// Create publisher gRPC service
	pub := publisher.NewServer(rtr, dsp, srv.Storage(), lgr)

	// Create the shared gRPC server with all services registered:
	//   - eventarcpb.EventarcServer (Trigger/Provider CRUD)
	//   - longrunningpb.OperationsServer (LRO polling)
	//   - publishingpb.PublisherServer (event publishing)
	//   - grpc/reflection
	grpcServer := server.NewGRPCServer(srv, pub, lgr)

	readyCh := make(chan struct{})

	// Start gRPC server
	go func() {
		lgr.Info("gRPC server listening at %v", lis.Addr())
		close(readyCh)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	<-readyCh
	lgr.Info("Ready to accept gRPC connections")
	lgr.Info("gRPC: localhost:%d", listenPort)

	// Wait for interrupt signal to gracefully shut down
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
