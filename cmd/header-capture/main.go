package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
)

func main() {
	port := "19999"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("\n=== %s %s ===\n", r.Method, r.URL.Path)

		// Sort headers for readability
		var keys []string
		for k := range r.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			for _, v := range r.Header[k] {
				display := v
				if len(display) > 120 {
					display = display[:120] + "..."
				}
				fmt.Printf("  %s: %s\n", k, display)
			}
		}

		// Read body size
		body, _ := io.ReadAll(r.Body)
		fmt.Printf("  [Body: %d bytes]\n", len(body))

		// Return a fake Anthropic error so the client doesn't hang
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"header capture done"}],"model":"claude-sonnet-4-20250514","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
	})

	fmt.Printf("Header capture server listening on :%s\n", port)
	fmt.Printf("Set ANTHROPIC_BASE_URL=http://localhost:%s to capture headers\n", port)
	fmt.Println(strings.Repeat("-", 60))
	http.ListenAndServe(":"+port, nil)
}
