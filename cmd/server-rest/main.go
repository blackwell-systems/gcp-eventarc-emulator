// GCP Eventarc Emulator - REST API
//
// A REST/HTTP emulator of the Google Cloud Eventarc API for local testing.
// This server runs a gRPC backend on an internal port with an HTTP/REST
// gateway frontend. External clients use HTTP only.
//
// When to use: REST-only clients; gRPC backend runs internally.
//
// Usage:
//
//	gcp-eventarc-emulator-rest --http-port 8085 --grpc-port 9086
//
// Environment Variables:
//
//	EVENTARC_HTTP_PORT  - HTTP port to listen on (default: 8085)
//	GCP_MOCK_LOG_LEVEL  - Log level: debug, info, warn, error (default: info)
//	IAM_MODE            - IAM enforcement: off, permissive, strict (default: off)
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
	httpPort = flag.Int("http-port", getEnvInt("EVENTARC_HTTP_PORT", 8085), "HTTP port to listen on")
	grpcPort = flag.Int("grpc-port", 9086, "gRPC port to listen on (internal only)")
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
		fmt.Fprintf(os.Stderr, "GCP Eventarc Emulator v%s — REST-only server\n", version)
		fmt.Fprintf(os.Stderr, "When to use: REST-only clients; gRPC backend runs internally.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: server-rest [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  EVENTARC_EMULATOR_HOST   not applicable (gRPC port is internal)\n")
		fmt.Fprintf(os.Stderr, "  EVENTARC_HTTP_PORT       HTTP port (default: 8085)\n")
		fmt.Fprintf(os.Stderr, "  GCP_MOCK_LOG_LEVEL       Log level: debug, info, warn, error (default: info)\n")
		fmt.Fprintf(os.Stderr, "  IAM_MODE                 IAM enforcement: off, permissive, strict (default: off)\n")
		fmt.Fprintf(os.Stderr, "  EVENTARC_EMULATOR_TOKEN  Bearer token added to dispatched webhook requests\n")
		fmt.Fprintf(os.Stderr, "\nHealth endpoints (REST and dual only):\n")
		fmt.Fprintf(os.Stderr, "  GET /healthz  returns {\"status\":\"ok\"} HTTP 200\n")
		fmt.Fprintf(os.Stderr, "  GET /readyz   returns {\"status\":\"ok\"} HTTP 200\n")
	}

	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("GCP Eventarc Emulator v%s (server-rest)\n", version)
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

	lgr.Info("GCP Eventarc Emulator v%s (REST)", version)
	lgr.Info("Log level: %s", *logLevel)
	lgr.Info("Starting gRPC backend on internal port %d", *grpcPort)
	lgr.Info("Starting HTTP gateway on port %d", *httpPort)

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

	grpcReadyCh := make(chan struct{})

	// Start gRPC server in background (internal only — not exposed to clients)
	go func() {
		lgr.Info("gRPC backend listening at %v (internal)", lis.Addr())
		close(grpcReadyCh)
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
		lgr.Info("HTTP gateway listening at %s", httpAddr)
		if err := gw.Start(httpAddr); err != nil {
			log.Fatalf("Failed to serve HTTP: %v", err)
		}
	}()

	<-grpcReadyCh
	lgr.Info("Ready to accept REST requests")
	lgr.Info("REST: http://localhost:%d/v1/projects/my-project/locations/us-central1/triggers", *httpPort)

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
