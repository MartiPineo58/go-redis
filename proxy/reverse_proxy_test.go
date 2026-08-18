package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestRetryTransport_RetryWithBody(t *testing.T) {
	var attempts int32
	expectedPayload := `{"status": "test"}`

	// 1. Spin up a test HTTP server representing the upstream.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		currentAttempt := atomic.AddInt32(&attempts, 1)

		// Read the body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read body: %v", err)
		}
		defer r.Body.Close()

		// Assert that the body is received and matches the expected payload on all attempts
		if string(bodyBytes) != expectedPayload {
			t.Errorf("Attempt %d: expected body %q, got %q", currentAttempt, expectedPayload, string(bodyBytes))
		}

		if currentAttempt == 1 {
			// Fail the first request
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Succeed on the second request
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	// Create the RetryTransport
	transport := &RetryTransport{
		Transport:   http.DefaultTransport,
		MaxRetries:  2,
		MaxBodySize: 1024,
	}

	client := &http.Client{
		Transport: transport,
	}

	// Send a POST request with a non-empty body
	req, err := http.NewRequest("POST", server.URL, bytes.NewBufferString(expectedPayload))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}

	finalAttempts := atomic.LoadInt32(&attempts)
	if finalAttempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", finalAttempts)
	}
}

func TestRetryTransport_MaxBodySizeExceeded(t *testing.T) {
	transport := &RetryTransport{
		Transport:   http.DefaultTransport,
		MaxRetries:  2,
		MaxBodySize: 5,
	}

	client := &http.Client{
		Transport: transport,
	}

	req, err := http.NewRequest("POST", "http://localhost:8080", bytes.NewBufferString("123456"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	_, err = client.Do(req)
	if err == nil {
		t.Fatal("Expected error due to body size limit, got nil")
	}
}
