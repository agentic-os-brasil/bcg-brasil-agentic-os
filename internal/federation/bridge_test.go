package federation

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPBridgePostsOnlyTypedBatchWithoutCredentials(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/federation/v1/batches" {
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" || request.Header.Get("X-GitHub-Token") != "" {
			t.Fatalf("bridge received a credential-bearing header: %#v", request.Header)
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", request.Header.Get("Content-Type"))
		}
		batch, err := Parse(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if batch.InstallationID != "0123456789abcdef" {
			t.Fatalf("batch = %#v", batch)
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	bridge, err := NewHTTPBridge(server.URL+"/federation/v1/batches", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.Submit(context.Background(), validBatch()); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPBridgeRejectsInsecureOrCredentialBearingEndpoint(t *testing.T) {
	for _, endpoint := range []string{
		"http://bridge.maestro.example/federation/v1/batches",
		"https://token@bridge.maestro.example/federation/v1/batches",
		"https://bridge.maestro.example/federation/v1/batches?token=no",
	} {
		if _, err := NewHTTPBridge(endpoint, nil); err == nil {
			t.Fatalf("endpoint %q was accepted", endpoint)
		}
	}
}

func TestHTTPBridgeDoesNotReflectRemoteResponseBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, "workspace-secret-CANARY")
	}))
	defer server.Close()
	bridge, err := NewHTTPBridge(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	err = bridge.Submit(context.Background(), validBatch())
	if err == nil || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("bridge error = %v", err)
	}
}
