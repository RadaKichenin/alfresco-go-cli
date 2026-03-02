package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConfigureValidation(t *testing.T) {
	if err := Configure(0, 1, time.Millisecond); err == nil {
		t.Fatal("expected error when timeout is zero")
	}
	if err := Configure(time.Second, -1, time.Millisecond); err == nil {
		t.Fatal("expected error when retries is negative")
	}
	if err := Configure(time.Second, 1, 0); err == nil {
		t.Fatal("expected error when retry wait is zero")
	}
	if err := Configure(time.Second, 1, time.Millisecond); err != nil {
		t.Fatalf("unexpected configure error: %v", err)
	}
}

func TestDoRequestWithRetryRetriesGETOn500(t *testing.T) {
	if err := Configure(2*time.Second, 2, time.Millisecond); err != nil {
		t.Fatalf("configure failed: %v", err)
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("temporary"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	err := doRequestWithRetry(
		http.MethodGet,
		client,
		func() (*http.Request, error) {
			return http.NewRequest(http.MethodGet, server.URL, nil)
		},
		func(resp *http.Response) error {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return &HTTPStatusError{
					StatusCode: resp.StatusCode,
					StatusText: http.StatusText(resp.StatusCode),
					Method:     http.MethodGet,
					URL:        server.URL,
				}
			}
			_, _ = io.ReadAll(resp.Body)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("expected eventual success, got error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempt count = %d, want 2", attempts)
	}
}

func TestDoRequestWithRetryDoesNotRetryPOST(t *testing.T) {
	if err := Configure(2*time.Second, 3, time.Millisecond); err != nil {
		t.Fatalf("configure failed: %v", err)
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("no retry for post"))
	}))
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	err := doRequestWithRetry(
		http.MethodPost,
		client,
		func() (*http.Request, error) {
			return http.NewRequest(http.MethodPost, server.URL, nil)
		},
		func(resp *http.Response) error {
			defer resp.Body.Close()
			return &HTTPStatusError{
				StatusCode: resp.StatusCode,
				StatusText: http.StatusText(resp.StatusCode),
				Method:     http.MethodPost,
				URL:        server.URL,
			}
		},
	)
	if err == nil {
		t.Fatal("expected error for POST 500")
	}
	if attempts != 1 {
		t.Fatalf("attempt count = %d, want 1", attempts)
	}
}

func TestBuildStatusErrorIncludesBodyAndTruncates(t *testing.T) {
	body := strings.Repeat("x", 5000)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	err := buildStatusError(resp, http.MethodGet, "https://example.test/resource")
	httpErr, ok := err.(*HTTPStatusError)
	if !ok {
		t.Fatalf("expected *HTTPStatusError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", httpErr.StatusCode, http.StatusBadRequest)
	}
	if !strings.Contains(httpErr.Body, "...(truncated)") {
		t.Fatalf("expected truncated marker in body, got: %q", httpErr.Body)
	}
	if len(httpErr.Body) <= 4096 {
		t.Fatalf("expected body to include truncation marker beyond 4096 chars, got len=%d", len(httpErr.Body))
	}
	msg := httpErr.Error()
	if !strings.Contains(msg, fmt.Sprintf("%d", http.StatusBadRequest)) {
		t.Fatalf("error message missing status code: %q", msg)
	}
}
