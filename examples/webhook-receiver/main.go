// Tiny HTTP server that logs incoming CloudEvents in binary content mode.
// Used by the demo to show events arriving at a destination.
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
)

func main() {
	port := "3000"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		fmt.Println()
		fmt.Println("═══════════════════════════════════════════════")
		fmt.Printf("  CloudEvent received  (%s %s)\n", r.Method, r.URL.Path)
		fmt.Println("═══════════════════════════════════════════════")

		// Print Ce-* headers (CloudEvents binary content mode)
		var ceHeaders []string
		for k := range r.Header {
			if strings.HasPrefix(strings.ToLower(k), "ce-") {
				ceHeaders = append(ceHeaders, k)
			}
		}
		sort.Strings(ceHeaders)

		if len(ceHeaders) > 0 {
			fmt.Println("  CloudEvent Headers:")
			for _, k := range ceHeaders {
				fmt.Printf("    %s: %s\n", k, r.Header.Get(k))
			}
		}

		fmt.Printf("  Content-Type: %s\n", r.Header.Get("Content-Type"))

		if len(body) > 0 {
			fmt.Println("  Body:")
			fmt.Printf("    %s\n", string(body))
		}
		fmt.Println("═══════════════════════════════════════════════")
		fmt.Println()

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"received"}`)
	})

	log.Printf("Webhook receiver listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
