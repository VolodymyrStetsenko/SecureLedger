package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	if err := check(client, server.URL+"/readyz"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRejectsUnavailableStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	if err := check(client, server.URL); err == nil {
		t.Fatal("unavailable readiness status was accepted")
	}
}
