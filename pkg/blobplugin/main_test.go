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
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
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
		{
			err:         fmt.Errorf("use of closed network connection"),
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		err := trapClosedConnErr(test.err)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("Expected error %v, but got %v", test.expectedErr, err)
		}
	}
}

func TestExportMetrics(t *testing.T) {
	// Test with empty metrics address
	*metricsAddress = ""
	exportMetrics() // Should return without error

	// Test with valid metrics address
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	*metricsAddress = listener.Addr().String()
	exportMetrics() // Should set up metrics server

	// Give some time for the goroutine to start
	time.Sleep(100 * time.Millisecond)
}

func TestServe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	testServeFunc := func(l net.Listener) error {
		return nil
	}

	ctx := context.Background()
	serve(ctx, listener, testServeFunc)

	// Give some time for the goroutine to start
	time.Sleep(100 * time.Millisecond)
}

func TestServeMetrics(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// Start the server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- serveMetrics(listener)
	}()

	// Give some time for the server to start
	time.Sleep(100 * time.Millisecond)

	// Make a request to the metrics endpoint
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/metrics", listener.Addr().String()))
	if err != nil {
		t.Logf("Expected to make request to metrics endpoint, but got error: %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status code 200, but got %d", resp.StatusCode)
		}
	}

	// Close the listener to stop the server
	listener.Close()

	// Wait for the server to stop
	select {
	case err := <-errCh:
		// The server should stop with a closed connection error, which is trapped
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			t.Errorf("Unexpected error from serveMetrics: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Server did not stop within timeout")
	}
}
