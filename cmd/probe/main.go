// Command probe provides a shell-free container health check.
package main

import (
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	path := "/health"
	if len(os.Args) == 2 && (os.Args[1] == "/health" || os.Args[1] == "/ready") {
		path = os.Args[1]
	} else if len(os.Args) != 1 {
		os.Exit(1)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		os.Exit(1)
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(n) + path)
	if err != nil {
		os.Exit(1)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
