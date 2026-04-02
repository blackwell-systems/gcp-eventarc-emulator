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
//	EVENTARC_EMULATOR_HOST  - Host and port for the gRPC server (e.g. localhost:9085)
//	EVENTARC_EVENTS_PORT    - Port for the HTTP /events convenience endpoint (default: 8085)
//	GCP_MOCK_LOG_LEVEL      - Log level: debug, info, warn, error (default: info)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/dispatcher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/publisher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/router"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/server"
)

var (
	port       = flag.Int("port", getEnvPort("EVENTARC_EMULATOR_HOST", 9085), "Port to listen on")
	eventsPort = flag.Int("events-port", getEnvInt("EVENTARC_EVENTS_PORT", 8085), "Port for the HTTP /events convenience endpoint")
	logLevel   = flag.String("log-level", getEnv("GCP_MOCK_LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	version    = "0.1.0"
)

func main() {
	flag.Parse()

	log.Printf("GCP Eventarc Emulator v%s", version)
	log.Printf("Starting on port %d with log level: %s", *port, *logLevel)

	// Create listener for gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
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
	log.Printf("Publisher service registered")

	// Create the shared gRPC server with all services registered:
	//   - eventarcpb.EventarcServer (Trigger/Provider CRUD)
	//   - longrunningpb.OperationsServer (LRO polling)
	//   - publishingpb.PublisherServer (event publishing)
	//   - grpc/reflection
	grpcServer := server.NewGRPCServer(srv, pub)

	log.Printf("Server listening at %v", lis.Addr())
	log.Printf("Ready to accept connections")

	// Start gRPC server
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Start HTTP convenience endpoint for curl-based testing
	// POST /events  — accepts a CloudEvent JSON body, matches triggers, dispatches
	go startHTTPEventsServer(*eventsPort, rtr, dsp)

	// Wait for interrupt signal to gracefully shut down
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	grpcServer.GracefulStop()
	log.Println("Server stopped")
}

// startHTTPEventsServer starts a lightweight HTTP server on the given port.
// POST /events accepts a CloudEvent JSON body, calls router.Match, and
// dispatches to matching triggers. This endpoint is for curl-based integration
// testing; real clients should use the gRPC Publisher service.
func startHTTPEventsServer(port int, rtr *router.Router, dsp *dispatcher.Dispatcher) {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var event cloudevents.Event
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, fmt.Sprintf("invalid CloudEvent JSON: %v", err), http.StatusBadRequest)
			return
		}

		// Derive parent from the "parent" query param or fall back to a default.
		parent := r.URL.Query().Get("parent")
		if parent == "" {
			// Fallback: use source as a rough proxy (best-effort for curl testing)
			parent = event.Source()
		}

		triggers, err := rtr.Match(r.Context(), parent, event)
		if err != nil {
			http.Error(w, fmt.Sprintf("router match: %v", err), http.StatusInternalServerError)
			return
		}

		dispatched := 0
		for _, t := range triggers {
			if _, err := dsp.Dispatch(r.Context(), t, event); err != nil {
				log.Printf("HTTP /events: dispatch error (trigger=%s): %v", t.GetName(), err)
				continue
			}
			dispatched++
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"matched":%d,"dispatched":%d}`, len(triggers), dispatched)
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("HTTP /events endpoint listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
		log.Printf("HTTP events server error: %v", err)
	}
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
