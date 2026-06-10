/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestMain(t *testing.T) {
	// Set the version flag to true
	os.Args = []string{"cmd", "-version"}

	// Capture stdout
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	// Replace exit function with mock function
	var exitCode int
	exit = func(code int) {
		exitCode = code
	}

	// Call main function
	main()

	// Restore stdout
	w.Close()
	os.Stdout = old
	exit = func(code int) {
		os.Exit(code)
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, but got %d", exitCode)
	}
}

func TestTrapClosedConnErr(t *testing.T) {
	tests := []struct {
		err         error
		expectedErr error
	}{
		{
			err:         net.ErrClosed,
			expectedErr: nil,
		},
		{
			err:         nil,
			expectedErr: nil,
		},
		{
			err:         fmt.Errorf("some error"),
			expectedErr: fmt.Errorf("some error"),
		},
	}

	for _, test := range tests {
		err := trapClosedConnErr(test.err)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("Expected error %v, but got %v", test.expectedErr, err)
		}
	}
}

func TestServeMetrics(t *testing.T) {
	// Open random test port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Start serveMetrics in background
	errCh := make(chan error, 1)
	go func() { errCh <- serveMetrics(l) }()

	// Build URL
	url := "http://" + l.Addr().String() + "/metrics"

	// Client timeout for each request
	client := &http.Client{Timeout: 500 * time.Millisecond}

	// Poll with 3-second deadlines until server is ready
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func(ctx context.Context) {
		defer close(done)
		for {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			// Execute probe, expecting status 200
			resp, err := client.Do(req)
			if err == nil {
				if resp.StatusCode == http.StatusOK {
					resp.Body.Close()
					return
				}
				resp.Body.Close()
			}
			// Abort probe if context deadline is expired or canceled
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}
	}(ctx)

	// Wait for readiness or fail on timeout
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("server not ready: %v", ctx.Err())
	}

	// Perform the request
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Get /metrics: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	// Check for HTTP status 200
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	// Validate response body is non-empty
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	// Validate response body is non-empty
	if len(body) == 0 {
		t.Fatalf("empty metrics body")
	}

	// Trigger graceful shutdown
	if err := l.Close(); err != nil {
		t.Fatalf("close listener %v", err)
	}

	// Fail if errCh exits non-graceful or is not closed after 2 sec
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serveMetrics error after close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("serveMetrics did not exit after listener close")
	}
}
