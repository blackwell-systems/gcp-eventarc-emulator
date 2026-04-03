// GCP Eventarc Emulator - Dual Protocol
//
// Provides both gRPC and REST/HTTP APIs for the Google Cloud Eventarc service.
// This server exposes both protocols simultaneously for maximum flexibility.
//
// When to use: recommended for all local development; provides both gRPC and REST access.
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
//	IAM_MODE                - IAM enforcement: off, permissive, strict (default: off)
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
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/logger"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/publisher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/router"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/server"
)

var (
	grpcPort = flag.Int("grpc-port", getEnvPort("EVENTARC_EMULATOR_HOST", 9085), "gRPC port to listen on")
	httpPort = flag.Int("http-port", getEnvInt("EVENTARC_HTTP_PORT", 8085), "HTTP port to listen on")
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
		fmt.Fprintf(os.Stderr, "GCP Eventarc Emulator v%s — gRPC + REST server\n", version)
		fmt.Fprintf(os.Stderr, "When to use: recommended for all local development; provides both gRPC and REST access.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: server-dual [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  EVENTARC_EMULATOR_HOST   gRPC host:port for clients (e.g. localhost:9085)\n")
		fmt.Fprintf(os.Stderr, "  EVENTARC_HTTP_PORT       HTTP port (default: 8085)\n")
		fmt.Fprintf(os.Stderr, "  GCP_MOCK_LOG_LEVEL       Log level: debug, info, warn, error (default: info)\n")
		fmt.Fprintf(os.Stderr, "  IAM_MODE                 IAM enforcement: off, permissive, strict (default: off)\n")
		fmt.Fprintf(os.Stderr, "  EVENTARC_EMULATOR_TOKEN  Bearer token added to dispatched webhook requests\n")
		fmt.Fprintf(os.Stderr, "\nHealth endpoints:\n")
		fmt.Fprintf(os.Stderr, "  GET /healthz  returns {\"status\":\"ok\"} HTTP 200\n")
		fmt.Fprintf(os.Stderr, "  GET /readyz   returns {\"status\":\"ok\"} HTTP 200\n")
	}

	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("GCP Eventarc Emulator v%s (server-dual)\n", version)
		os.Exit(0)
	}

	if err := validateLogLevel(*logLevel); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *grpcPort < 1 || *grpcPort > 65535 {
		fmt.Fprintf(os.Stderr, "invalid --grpc-port %d: must be in range 1-65535\n", *grpcPort)
		os.Exit(1)
	}
	if *httpPort < 1 || *httpPort > 65535 {
		fmt.Fprintf(os.Stderr, "invalid --http-port %d: must be in range 1-65535\n", *httpPort)
		os.Exit(1)
	}

	lgr := logger.New(*logLevel)

	lgr.Info("GCP Eventarc Emulator v%s (gRPC + REST)", version)
	lgr.Info("Log level: %s", *logLevel)

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

	lgr.Info("IAM mode: %s", srv.IAMMode())
	if token := os.Getenv("EVENTARC_EMULATOR_TOKEN"); token != "" {
		lgr.Info("Bearer token injection: enabled")
	}

	// Wire up router and dispatcher
	rtr := router.NewRouter(srv.Storage(), lgr)
	dsp := dispatcher.NewDispatcher(nil, lgr)

	// Create publisher gRPC service
	pub := publisher.NewServer(rtr, dsp, srv.Storage(), lgr)

	// Create the shared gRPC server with all services registered
	grpcSrv := server.NewGRPCServer(srv, pub, lgr)

	// readyCh is closed by the gRPC goroutine once it is listening, so that the
	// main goroutine can print "Ready" only after gRPC is confirmed serving.
	readyCh := make(chan struct{})

	// Start gRPC server in background (externally exposed)
	go func() {
		lgr.Info("gRPC server listening at %v", lis.Addr())
		close(readyCh) // signal that gRPC is ready to accept connections
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
		lgr.Info("HTTP gateway listening at %s", httpAddr)
		if err := gw.Start(httpAddr); err != nil {
			log.Fatalf("Failed to serve HTTP: %v", err)
		}
	}()

	// Wait for gRPC to confirm it is serving before printing "Ready"
	<-readyCh
	lgr.Info("Ready to accept both gRPC and REST requests")
	lgr.Info("gRPC: localhost:%d", *grpcPort)
	lgr.Info("REST: http://localhost:%d/v1/projects/my-project/locations/us-central1/triggers", *httpPort)

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
