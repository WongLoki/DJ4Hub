package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeHTTPFromInterfaceUsesRequestedInterface(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := probeHTTPFromInterface(ctx, "lo0", "127.0.0.1", server.URL); err != nil {
		t.Fatalf("probeHTTPFromInterface() error = %v", err)
	}
}

func TestProbeHTTPFromInterfaceRejectsInvalidIPv4(t *testing.T) {
	err := probeHTTPFromInterface(context.Background(), "lo0", "not-an-ip", "http://127.0.0.1")
	if err == nil {
		t.Fatal("probeHTTPFromInterface() error = nil, want invalid IPv4 error")
	}
}

func TestProbeFirstTargetFallsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	target, err := probeFirstTarget(ctx, "lo0", "127.0.0.1", []string{"http://127.0.0.1:1", server.URL})
	if err != nil {
		t.Fatalf("probeFirstTarget() error = %v", err)
	}
	if target != server.URL {
		t.Fatalf("probeFirstTarget() target = %q, want %q", target, server.URL)
	}
}
