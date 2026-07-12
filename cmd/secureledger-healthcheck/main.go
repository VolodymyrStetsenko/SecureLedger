package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	url := os.Getenv("SECURELEDGER_HEALTHCHECK_URL")
	if url == "" {
		url = "http://127.0.0.1:8080/readyz"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	if err := check(client, url); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func check(client *http.Client, url string) error {
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("readiness request: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness status: %s", response.Status)
	}
	return nil
}
