package proxy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"time"
)

// ErrBodyTooLarge is returned when the request body exceeds the configured limit.
var ErrBodyTooLarge = errors.New("request body too large")

// RetryTransport is an http.RoundTripper that retries failed requests.
type RetryTransport struct {
	Transport   http.RoundTripper
	MaxRetries  int
	MaxBodySize int64 // Configurable limit on the maximum body size to prevent OOM
}

// RoundTrip executes a single HTTP transaction, retrying on failure.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	var err error

	// 1. Read and Cache the Body if it exists
	if req.Body != nil && req.Body != http.NoBody {
		if t.MaxBodySize > 0 {
			// Read up to MaxBodySize + 1 to detect if it exceeds the limit
			bodyBytes, err = io.ReadAll(io.LimitReader(req.Body, t.MaxBodySize+1))
			if err != nil {
				req.Body.Close()
				return nil, err
			}
			if int64(len(bodyBytes)) > t.MaxBodySize {
				req.Body.Close()
				return nil, ErrBodyTooLarge
			}
		} else {
			bodyBytes, err = io.ReadAll(req.Body)
			if err != nil {
				req.Body.Close()
				return nil, err
			}
		}
		req.Body.Close()

		// 2. Define GetBody for Automatic Rewinding
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}

		// 3. Reset Body for the Initial Request
		req.Body, _ = req.GetBody()
	}

	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	var resp *http.Response
	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		// Clone the request for this attempt to ensure thread safety
		attemptReq := req.Clone(req.Context())
		if req.GetBody != nil {
			attemptReq.Body, _ = req.GetBody()
		}

		resp, err = transport.RoundTrip(attemptReq)
		if err == nil && (resp == nil || resp.StatusCode < 500) {
			return resp, nil
		}

		// If it's the last attempt, return the result
		if attempt == t.MaxRetries {
			break
		}

		// Wait a bit before retrying
		time.Sleep(50 * time.Millisecond)
	}

	return resp, err
}
