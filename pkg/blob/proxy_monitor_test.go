/*
Copyright 2026 The Kubernetes Authors.

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

package blob

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestNewBlobfuseProxyMonitor(t *testing.T) {
	endpoint := "unix:///tmp/test-proxy.sock"
	monitor := NewBlobfuseProxyMonitor(endpoint)

	if monitor == nil {
		t.Error("expected monitor to be created")
	}

	if monitor.endpoint != endpoint {
		t.Errorf("expected endpoint %s, got %s", endpoint, monitor.endpoint)
	}
}

func TestBlobfuseProxyMonitor_HealthCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	tempDir, err := os.MkdirTemp("", "proxy-monitor-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test-proxy.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to create Unix socket: %v", err)
	}
	defer listener.Close()

	// Test successful connection
	monitor := NewBlobfuseProxyMonitor("unix://" + socketPath)
	if !monitor.isProxyReachable() {
		t.Error("expected isProxyReachable to return true for valid socket")
	}
	// checkHealth should not panic
	monitor.checkHealth()

	// Test failed connection
	invalidMonitor := NewBlobfuseProxyMonitor("unix:///tmp/non-existent-proxy.sock")
	if invalidMonitor.isProxyReachable() {
		t.Error("expected isProxyReachable to return false for non-existent socket")
	}
	// checkHealth should not panic even for non-existent socket
	invalidMonitor.checkHealth()
}

func TestBlobfuseProxyMonitor_Start(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	monitor := NewBlobfuseProxyMonitor("unix:///tmp/test-proxy.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		monitor.Start(ctx)
		done <- true
	}()

	select {
	case <-done:
		// Expected - Start should return when context is cancelled
	case <-time.After(200 * time.Millisecond):
		t.Error("expected return within expected time after context cancelled")
	}
}
